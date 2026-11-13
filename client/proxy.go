// Copyright 2016 fatedier, fatedier@gmail.com
// Copyright 2026 eoasmxd (Modified for L4Tunnel)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eoasmxd/l4tunnel/config"
	"github.com/eoasmxd/l4tunnel/msg"
	"github.com/eoasmxd/l4tunnel/utils"
)

const (
	ProxyStatusNew         = "new"
	ProxyStatusWaitStart   = "wait start"
	ProxyStatusStartErr    = "start error"
	ProxyStatusRunning     = "running"
	ProxyStatusCheckFailed = "check failed"
	ProxyStatusClosed      = "closed"
)

var (
	statusCheckInterval = 3 * time.Second
	waitResponseTimeout = 20 * time.Second
	startErrTimeout     = 30 * time.Second
)

// ProxyWrapper 管理客户端单条代理的生命周期、健康探测与工作连接转发。
type ProxyWrapper struct {
	Name   string
	Status string
	Err    string
	Cfg    *config.TcpProxyConf

	monitor *HealthCheckMonitor
	sendMsg func(msg.Message)

	health           uint32
	lastSendStartMsg time.Time
	lastStartErr     time.Time
	closeCh          chan struct{}
	healthNotifyCh   chan struct{}
	mu               sync.RWMutex

	xl  *utils.Logger
	ctx context.Context
}

func NewProxyWrapper(ctx context.Context, cfg *config.TcpProxyConf, sendMsg func(msg.Message)) *ProxyWrapper {
	xl := utils.LogFromContextSafe(ctx).Spawn().AppendPrefix(cfg.ProxyName)
	pw := &ProxyWrapper{
		Name:           cfg.ProxyName,
		Status:         ProxyStatusNew,
		Cfg:            cfg,
		closeCh:        make(chan struct{}),
		healthNotifyCh: make(chan struct{}, 1),
		sendMsg:        sendMsg,
		xl:             xl,
		ctx:            utils.NewLogContext(ctx, xl),
	}

	if cfg.HealthCheck {
		pw.health = 1
		pw.monitor = NewHealthCheckMonitor(pw.ctx, cfg.HealthCheckAddr,
			pw.statusNormalCallback, pw.statusFailedCallback)
		xl.Trace("enable health check monitor")
	}

	return pw
}

func (pw *ProxyWrapper) SetRunningStatus(respErr string) error {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	if pw.Status != ProxyStatusWaitStart {
		return fmt.Errorf("status not wait start, ignore start message")
	}

	if respErr != "" {
		pw.Status = ProxyStatusStartErr
		pw.Err = respErr
		pw.lastStartErr = time.Now()
		return errors.New(pw.Err)
	}

	pw.Status = ProxyStatusRunning
	pw.Err = ""
	return nil
}

func (pw *ProxyWrapper) Start() {
	go pw.checkWorker()
	if pw.monitor != nil {
		go pw.monitor.Start()
	}
}

func (pw *ProxyWrapper) Stop() {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	if pw.Status == ProxyStatusClosed {
		return
	}
	close(pw.closeCh)
	if pw.monitor != nil {
		pw.monitor.Stop()
	}
	pw.Status = ProxyStatusClosed
	pw.sendMsg(&msg.CloseProxy{
		ProxyName: pw.Name,
	})
}

func (pw *ProxyWrapper) checkWorker() {
	xl := pw.xl
	if pw.monitor != nil {
		time.Sleep(500 * time.Millisecond)
	}

	ticker := time.NewTicker(statusCheckInterval)
	defer ticker.Stop()

	for {
		now := time.Now()
		if atomic.LoadUint32(&pw.health) == 0 {
			pw.mu.Lock()
			if pw.Status == ProxyStatusNew ||
				pw.Status == ProxyStatusCheckFailed ||
				(pw.Status == ProxyStatusWaitStart && now.After(pw.lastSendStartMsg.Add(waitResponseTimeout))) ||
				(pw.Status == ProxyStatusStartErr && now.After(pw.lastStartErr.Add(startErrTimeout))) {

				xl.Trace("change status from [%s] to [%s]", pw.Status, ProxyStatusWaitStart)
				pw.Status = ProxyStatusWaitStart

				var newProxyMsg msg.NewProxy
				pw.Cfg.MarshalToMsg(&newProxyMsg)
				pw.lastSendStartMsg = now
				pw.sendMsg(&newProxyMsg)
			}
			pw.mu.Unlock()
		} else {
			pw.mu.Lock()
			if pw.Status == ProxyStatusRunning || pw.Status == ProxyStatusWaitStart {
				pw.sendMsg(&msg.CloseProxy{
					ProxyName: pw.Name,
				})
				xl.Trace("change status from [%s] to [%s]", pw.Status, ProxyStatusCheckFailed)
				pw.Status = ProxyStatusCheckFailed
			}
			pw.mu.Unlock()
		}

		select {
		case <-pw.closeCh:
			return
		case <-ticker.C:
		case <-pw.healthNotifyCh:
		}
	}
}

func (pw *ProxyWrapper) statusNormalCallback() {
	atomic.StoreUint32(&pw.health, 0)
	select {
	case pw.healthNotifyCh <- struct{}{}:
	default:
	}
	pw.xl.Info("health check success")
}

func (pw *ProxyWrapper) statusFailedCallback() {
	atomic.StoreUint32(&pw.health, 1)
	select {
	case pw.healthNotifyCh <- struct{}{}:
	default:
	}
	pw.xl.Info("health check failed")
}

func (pw *ProxyWrapper) InWorkConn(workConn net.Conn) {
	pw.xl.Debug("start a new work connection, localAddr: %s remoteAddr: %s", workConn.LocalAddr().String(), workConn.RemoteAddr().String())
	go HandleTcpWorkConnection(pw.ctx, pw.Cfg, workConn)
}

func HandleTcpWorkConnection(ctx context.Context, cfg *config.TcpProxyConf, workConn net.Conn) {
	xl := utils.LogFromContextSafe(ctx)

	localConn, err := utils.ConnectServer(fmt.Sprintf("%s:%d", cfg.LocalIp, cfg.LocalPort))
	if err != nil {
		workConn.Close()
		xl.Error("connect to local service [%s:%d] error: %v", cfg.LocalIp, cfg.LocalPort, err)
		return
	}

	xl.Debug("join connections, localConn(l[%s] r[%s]) workConn(l[%s] r[%s])", localConn.LocalAddr().String(),
		localConn.RemoteAddr().String(), workConn.LocalAddr().String(), workConn.RemoteAddr().String())

	utils.Join(localConn, workConn)
	xl.Debug("join connections closed")
}

// ProxyManager 管理客户端代理配置集合。
type ProxyManager struct {
	sendCh  chan msg.Message
	stopCh  <-chan struct{}
	proxies map[string]*ProxyWrapper

	mu  sync.RWMutex
	ctx context.Context
}

func NewProxyManager(ctx context.Context, msgSendCh chan msg.Message, stopCh <-chan struct{}) *ProxyManager {
	return &ProxyManager{
		sendCh:  msgSendCh,
		stopCh:  stopCh,
		proxies: make(map[string]*ProxyWrapper),
		ctx:     ctx,
	}
}

func (pm *ProxyManager) StartProxy(name string, serverRespErr string) error {
	pm.mu.RLock()
	pxy, ok := pm.proxies[name]
	pm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("proxy [%s] not found", name)
	}
	return pxy.SetRunningStatus(serverRespErr)
}

func (pm *ProxyManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, pxy := range pm.proxies {
		pxy.Stop()
	}
	pm.proxies = make(map[string]*ProxyWrapper)
}

func (pm *ProxyManager) HandleWorkConn(name string, workConn net.Conn) {
	pm.mu.RLock()
	pw, ok := pm.proxies[name]
	pm.mu.RUnlock()
	if ok {
		pw.InWorkConn(workConn)
	} else {
		workConn.Close()
	}
}

func (pm *ProxyManager) sendMsg(m msg.Message) {
	select {
	case pm.sendCh <- m:
	case <-pm.stopCh:
	}
}

// StartAll 初始化并启动全部代理。
func (pm *ProxyManager) StartAll(pxyCfgs map[string]*config.TcpProxyConf) {
	xl := utils.LogFromContextSafe(pm.ctx)
	pm.mu.Lock()
	defer pm.mu.Unlock()

	addPxyNames := make([]string, 0, len(pxyCfgs))
	for name, cfg := range pxyCfgs {
		if _, ok := pm.proxies[name]; !ok {
			pxy := NewProxyWrapper(pm.ctx, cfg, pm.sendMsg)
			pm.proxies[name] = pxy
			addPxyNames = append(addPxyNames, name)
			pxy.Start()
		}
	}
	if len(addPxyNames) > 0 {
		xl.Info("proxy added: %v", addPxyNames)
	}
}

