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
	"fmt"
	"net"
	"sync"

	"github.com/eoasmxd/l4tunnel/config"
	"github.com/eoasmxd/l4tunnel/msg"
	"github.com/eoasmxd/l4tunnel/utils"
)

type GetWorkConnFn func() (net.Conn, error)

// TcpProxy 负责服务端单个 TCP 映射端口的监听与转发。
type TcpProxy struct {
	name          string
	listener      net.Listener
	poolCount     int
	getWorkConnFn GetWorkConnFn
	serverCfg     config.ServerCommonConf
	cfg           *config.TcpProxyConf

	xl  *utils.Logger
	ctx context.Context
}

func NewTcpProxy(ctx context.Context, poolCount int, getWorkConnFn GetWorkConnFn,
	cfg *config.TcpProxyConf, serverCfg config.ServerCommonConf) *TcpProxy {

	xl := utils.LogFromContextSafe(ctx).Spawn().AppendPrefix(cfg.ProxyName)
	return &TcpProxy{
		name:          cfg.ProxyName,
		poolCount:     poolCount,
		getWorkConnFn: getWorkConnFn,
		serverCfg:     serverCfg,
		cfg:           cfg,
		xl:            xl,
		ctx:           utils.NewLogContext(ctx, xl),
	}
}

func (pxy *TcpProxy) GetName() string {
	return pxy.name
}

func (pxy *TcpProxy) Run() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", pxy.serverCfg.ProxyBindAddr, pxy.cfg.RemotePort))
	if err != nil {
		return err
	}
	pxy.listener = listener
	pxy.xl.Info("tcp proxy listen port [%d]", pxy.cfg.RemotePort)

	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				pxy.xl.Info("listener is closed")
				return
			}
			pxy.xl.Debug("get a user connection [%s]", c.RemoteAddr().String())
			go pxy.handleUserTcpConnection(c)
		}
	}()

	return nil
}

func (pxy *TcpProxy) Close() {
	pxy.xl.Info("proxy closing")
	if pxy.listener != nil {
		pxy.listener.Close()
	}
}

func (pxy *TcpProxy) getWorkConnFromPool() (workConn net.Conn, err error) {
	for i := 0; i < pxy.poolCount+1; i++ {
		if workConn, err = pxy.getWorkConnFn(); err != nil {
			pxy.xl.Warn("failed to get work connection: %v", err)
			return nil, err
		}
		pxy.xl.Info("get a new work connection: [%s]", workConn.RemoteAddr().String())

		err = msg.WriteMsg(workConn, &msg.StartWorkConn{
			ProxyName: pxy.name,
		})
		if err != nil {
			pxy.xl.Warn("failed to send message to work connection from pool: %v, times: %d", err, i)
			workConn.Close()
			continue
		}
		return workConn, nil
	}

	pxy.xl.Error("try to get work connection failed in the end")
	return nil, err
}

func (pxy *TcpProxy) handleUserTcpConnection(userConn net.Conn) {
	defer userConn.Close()

	workConn, err := pxy.getWorkConnFromPool()
	if err != nil {
		return
	}
	defer workConn.Close()

	utils.Join(workConn, userConn)
	pxy.xl.Debug("join connections closed")
}

// ProxyManager 维护服务端活跃代理名称映射，避免跨客户端重名冲突。
type ProxyManager struct {
	pxys map[string]*TcpProxy
	mu   sync.RWMutex
}

func NewProxyManager() *ProxyManager {
	return &ProxyManager{
		pxys: make(map[string]*TcpProxy),
	}
}

func (pm *ProxyManager) Add(name string, pxy *TcpProxy) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if _, ok := pm.pxys[name]; ok {
		return fmt.Errorf("proxy name [%s] is already in use", name)
	}
	pm.pxys[name] = pxy
	return nil
}

func (pm *ProxyManager) Del(name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.pxys, name)
}

