// Copyright 2017 fatedier, fatedier@gmail.com
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

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/eoasmxd/l4tunnel/config"
	"github.com/eoasmxd/l4tunnel/msg"
	"github.com/eoasmxd/l4tunnel/utils"
)

const (
	serverHeartbeatTimeout = 90 * time.Second
	serverUserConnTimeout  = 10 * time.Second
	maxClientPoolCount     = 64
)

var errCtlClosed = errors.New("control is closed")

type ControlManager struct {
	ctlsByRunId map[string]*Control
	mu          sync.RWMutex
}

func NewControlManager() *ControlManager {
	return &ControlManager{
		ctlsByRunId: make(map[string]*Control),
	}
}

func (cm *ControlManager) Add(runId string, ctl *Control) (oldCtl *Control) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if oldCtl = cm.ctlsByRunId[runId]; oldCtl != nil {
		oldCtl.Replaced(ctl)
	}
	cm.ctlsByRunId[runId] = ctl
	return
}

func (cm *ControlManager) Del(runId string, ctl *Control) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if c, ok := cm.ctlsByRunId[runId]; ok && c == ctl {
		delete(cm.ctlsByRunId, runId)
	}
}

func (cm *ControlManager) GetById(runId string) (ctl *Control, ok bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	ctl, ok = cm.ctlsByRunId[runId]
	return
}

type Control struct {
	pxyManager   *ProxyManager
	conn         net.Conn
	sendCh       chan msg.Message
	readCh       chan msg.Message
	workConnCh   chan net.Conn
	proxies      map[string]*TcpProxy
	poolCount    int
	lastPing     time.Time
	runId        string
	stopOnce     sync.Once
	stopCh       chan struct{}
	closedDoneCh chan struct{}
	workerWg     sync.WaitGroup
	mu           sync.RWMutex
	serverCfg    config.ServerCommonConf

	xl  *utils.Logger
	ctx context.Context
}

func NewControl(ctx context.Context, pxyManager *ProxyManager,
	ctlConn net.Conn, runId string, poolCount int,
	serverCfg config.ServerCommonConf) *Control {

	if poolCount < 0 {
		poolCount = 0
	} else if poolCount > maxClientPoolCount {
		poolCount = maxClientPoolCount
	}
	return &Control{
		pxyManager:   pxyManager,
		conn:         ctlConn,
		sendCh:       make(chan msg.Message, 10),
		readCh:       make(chan msg.Message, 10),
		workConnCh:   make(chan net.Conn, poolCount+10),
		proxies:      make(map[string]*TcpProxy),
		poolCount:    poolCount,
		lastPing:     time.Now(),
		runId:        runId,
		stopCh:       make(chan struct{}),
		closedDoneCh: make(chan struct{}),
		serverCfg:    serverCfg,
		xl:           utils.LogFromContextSafe(ctx),
		ctx:          ctx,
	}
}

func (ctl *Control) Start() {
	ctl.workerWg.Add(3)
	go ctl.writer()
	for i := 0; i < ctl.poolCount; i++ {
		ctl.sendMsg(&msg.ReqWorkConn{})
	}
	go ctl.manager()
	go ctl.reader()
	go ctl.stopper()
}

func (ctl *Control) triggerStop() {
	ctl.stopOnce.Do(func() {
		close(ctl.stopCh)
	})
}

func (ctl *Control) sendMsg(m msg.Message) {
	select {
	case ctl.sendCh <- m:
	case <-ctl.stopCh:
	}
}

func (ctl *Control) RegisterWorkConn(conn net.Conn) error {
	select {
	case <-ctl.stopCh:
		conn.Close()
		return errCtlClosed
	default:
	}

	select {
	case ctl.workConnCh <- conn:
		return nil
	default:
		ctl.xl.Debug("workConnCh is full, discard this work connection")
		conn.Close()
		return fmt.Errorf("workConnCh is full")
	}
}

func (ctl *Control) GetWorkConn() (workConn net.Conn, err error) {
	select {
	case workConn = <-ctl.workConnCh:
		ctl.sendMsg(&msg.ReqWorkConn{})
		return workConn, nil
	case <-ctl.stopCh:
		return nil, errCtlClosed
	case <-time.After(serverUserConnTimeout):
		return nil, fmt.Errorf("get work connection timeout")
	}
}

func (ctl *Control) Replaced(newCtl *Control) {
	ctl.xl.Info("Replaced by client [%s]", newCtl.runId)
	ctl.triggerStop()
	ctl.conn.Close()
}

func (ctl *Control) writer() {
	xl := ctl.xl
	defer func() {
		if err := recover(); err != nil {
			xl.Error("panic error: %v", err)
			xl.Error(string(debug.Stack()))
		}
	}()
	defer ctl.workerWg.Done()

	for {
		select {
		case <-ctl.stopCh:
			xl.Info("control writer is closing")
			return
		case m := <-ctl.sendCh:
			if err := msg.WriteMsg(ctl.conn, m); err != nil {
				xl.Warn("write message to control connection error: %v", err)
				ctl.triggerStop()
				return
			}
		}
	}
}

func (ctl *Control) reader() {
	xl := ctl.xl
	defer func() {
		if err := recover(); err != nil {
			xl.Error("panic error: %v", err)
			xl.Error(string(debug.Stack()))
		}
	}()
	defer ctl.workerWg.Done()
	defer ctl.triggerStop()

	for {
		m, err := msg.ReadMsg(ctl.conn)
		if err != nil {
			if err == io.EOF {
				xl.Debug("control connection closed")
			} else {
				xl.Warn("read error: %v", err)
			}
			return
		}
		select {
		case ctl.readCh <- m:
		case <-ctl.stopCh:
			return
		}
	}
}

func (ctl *Control) stopper() {
	<-ctl.stopCh
	ctl.conn.Close()
	ctl.workerWg.Wait()

drain:
	for {
		select {
		case workConn := <-ctl.workConnCh:
			workConn.Close()
		default:
			break drain
		}
	}

	ctl.mu.Lock()
	for _, pxy := range ctl.proxies {
		pxy.Close()
		ctl.pxyManager.Del(pxy.GetName())
	}
	ctl.proxies = make(map[string]*TcpProxy)
	ctl.mu.Unlock()

	close(ctl.closedDoneCh)
	ctl.xl.Info("client exit success")
}

func (ctl *Control) WaitClosed() {
	<-ctl.closedDoneCh
}

func (ctl *Control) manager() {
	xl := ctl.xl
	defer func() {
		if err := recover(); err != nil {
			xl.Error("panic error: %v", err)
			xl.Error(string(debug.Stack()))
		}
	}()
	defer ctl.workerWg.Done()
	defer ctl.triggerStop()

	heartbeat := time.NewTicker(time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctl.stopCh:
			return
		case <-heartbeat.C:
			if time.Since(ctl.lastPing) > serverHeartbeatTimeout {
				xl.Warn("heartbeat timeout")
				return
			}
		case rawMsg := <-ctl.readCh:
			switch m := rawMsg.(type) {
			case *msg.NewProxy:
				err := ctl.RegisterProxy(m)
				resp := &msg.NewProxyResp{
					ProxyName: m.ProxyName,
				}
				if err != nil {
					resp.Error = err.Error()
					xl.Warn("new proxy [%s] error: %v", m.ProxyName, err)
				} else {
					xl.Info("new proxy [%s] success", m.ProxyName)
				}
				ctl.sendMsg(resp)
			case *msg.CloseProxy:
				ctl.CloseProxy(m)
				xl.Info("close proxy [%s] success", m.ProxyName)
			case *msg.Ping:
				ctl.lastPing = time.Now()
				xl.Debug("receive heartbeat")
				ctl.sendMsg(&msg.Pong{})
			}
		}
	}
}

func (ctl *Control) RegisterProxy(pxyMsg *msg.NewProxy) error {
	pxyConf, err := config.NewProxyConfFromMsg(pxyMsg)
	if err != nil {
		return err
	}

	pxy := NewTcpProxy(ctl.ctx, ctl.poolCount, ctl.GetWorkConn, pxyConf, ctl.serverCfg)
	if err = pxy.Run(); err != nil {
		return err
	}

	err = ctl.pxyManager.Add(pxyMsg.ProxyName, pxy)
	if err != nil {
		pxy.Close()
		return err
	}

	ctl.mu.Lock()
	ctl.proxies[pxy.GetName()] = pxy
	ctl.mu.Unlock()
	return nil
}

func (ctl *Control) CloseProxy(closeMsg *msg.CloseProxy) {
	ctl.mu.Lock()
	pxy, ok := ctl.proxies[closeMsg.ProxyName]
	if ok {
		pxy.Close()
		ctl.pxyManager.Del(pxy.GetName())
		delete(ctl.proxies, closeMsg.ProxyName)
	}
	ctl.mu.Unlock()
}
