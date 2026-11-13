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

package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/eoasmxd/l4tunnel/config"
	"github.com/eoasmxd/l4tunnel/msg"
	"github.com/eoasmxd/l4tunnel/utils"
)

const (
	heartbeatInterval = 30 * time.Second
	heartbeatTimeout  = 90 * time.Second
)

type Control struct {
	runId     string
	pxyCfgs   map[string]*config.TcpProxyConf
	pm        *ProxyManager
	conn      net.Conn
	sendCh    chan msg.Message
	readCh    chan msg.Message
	closedCh  chan struct{}
	stopCh    chan struct{}
	closedWg  sync.WaitGroup
	doneCh    chan struct{}
	lastPong  time.Time
	clientCfg config.ClientCommonConf

	xl  *utils.Logger
	ctx context.Context
}

func NewControl(ctx context.Context, runId string, conn net.Conn,
	clientCfg config.ClientCommonConf,
	pxyCfgs map[string]*config.TcpProxyConf) *Control {

	stopCh := make(chan struct{})
	ctl := &Control{
		runId:     runId,
		conn:      conn,
		pxyCfgs:   pxyCfgs,
		sendCh:    make(chan msg.Message, 100),
		readCh:    make(chan msg.Message, 100),
		closedCh:  make(chan struct{}),
		stopCh:    stopCh,
		doneCh:    make(chan struct{}),
		clientCfg: clientCfg,
		xl:        utils.LogFromContextSafe(ctx),
		ctx:       ctx,
	}
	ctl.pm = NewProxyManager(ctl.ctx, ctl.sendCh, stopCh)
	return ctl
}

func (ctl *Control) Run() {
	go ctl.worker()
	ctl.pm.StartAll(ctl.pxyCfgs)
}

func (ctl *Control) HandleReqWorkConn(inMsg *msg.ReqWorkConn) {
	xl := ctl.xl
	workConn, err := ctl.connectServer()
	if err != nil {
		return
	}

	now := time.Now().Unix()
	req, err := http.NewRequest("POST", "/l4t/connect", nil)
	if err != nil {
		workConn.Close()
		return
	}
	req.Header.Set("X-L4T-Type", "work")
	req.Header.Set("X-L4T-Auth", utils.GetAuthKey(ctl.clientCfg.Token, now))
	req.Header.Set("X-L4T-Timestamp", strconv.FormatInt(now, 10))
	req.Header.Set("X-L4T-Run-Id", ctl.runId)

	if err = req.Write(workConn); err != nil {
		workConn.Close()
		return
	}
	if resp, err := http.ReadResponse(bufio.NewReader(workConn), req); err != nil || resp.StatusCode != http.StatusOK {
		workConn.Close()
		return
	}

	doneCh := make(chan struct{})
	go func() {
		select {
		case <-ctl.stopCh:
			workConn.Close()
		case <-doneCh:
		}
	}()

	var startMsg msg.StartWorkConn
	if err = msg.ReadMsgInto(workConn, &startMsg); err != nil {
		close(doneCh)
		xl.Error("work connection closed before response StartWorkConn message: %v", err)
		workConn.Close()
		return
	}
	close(doneCh)

	ctl.pm.HandleWorkConn(startMsg.ProxyName, workConn)
}

func (ctl *Control) HandleNewProxyResp(inMsg *msg.NewProxyResp) {
	xl := ctl.xl
	err := ctl.pm.StartProxy(inMsg.ProxyName, inMsg.Error)
	if err != nil {
		xl.Warn("[%s] start error: %v", inMsg.ProxyName, err)
	} else {
		xl.Info("[%s] start proxy success", inMsg.ProxyName)
	}
}

func (ctl *Control) Close() {
	ctl.pm.Close()
	ctl.conn.Close()
}

func (ctl *Control) ClosedDoneCh() <-chan struct{} {
	return ctl.doneCh
}

func (ctl *Control) connectServer() (conn net.Conn, err error) {
	conn, err = utils.ConnectServerWithTLS(
		fmt.Sprintf("%s:%d", ctl.clientCfg.ServerAddr, ctl.clientCfg.ServerPort), utils.NewClientTLSConfig(ctl.clientCfg.ServerAddr))
	if err != nil {
		ctl.xl.Warn("start new connection to server error: %v", err)
		return
	}
	return
}

func (ctl *Control) sendMsg(m msg.Message) {
	select {
	case ctl.sendCh <- m:
	case <-ctl.stopCh:
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
	defer ctl.closedWg.Done()
	defer close(ctl.closedCh)

	for {
		m, err := msg.ReadMsg(ctl.conn)
		if err != nil {
			if err == io.EOF {
				xl.Debug("read from control connection EOF")
			} else {
				xl.Warn("read error: %v", err)
				ctl.conn.Close()
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

func (ctl *Control) writer() {
	defer ctl.closedWg.Done()
	for {
		select {
		case <-ctl.stopCh:
			ctl.xl.Info("control writer is closing")
			return
		case m := <-ctl.sendCh:
			if err := msg.WriteMsg(ctl.conn, m); err != nil {
				ctl.xl.Warn("write message to control connection error: %v", err)
				ctl.conn.Close()
				return
			}
		}
	}
}

func (ctl *Control) msgHandler() {
	xl := ctl.xl
	defer func() {
		if err := recover(); err != nil {
			xl.Error("panic error: %v", err)
			xl.Error(string(debug.Stack()))
		}
	}()
	defer ctl.closedWg.Done()

	hbSend := time.NewTicker(heartbeatInterval)
	defer hbSend.Stop()
	hbCheck := time.NewTicker(time.Second)
	defer hbCheck.Stop()

	ctl.lastPong = time.Now()

	for {
		select {
		case <-ctl.stopCh:
			return
		case <-hbSend.C:
			xl.Debug("send heartbeat to server")
			ctl.sendMsg(&msg.Ping{})
		case <-hbCheck.C:
			if time.Since(ctl.lastPong) > heartbeatTimeout {
				xl.Warn("heartbeat timeout")
				ctl.conn.Close()
				return
			}
		case rawMsg := <-ctl.readCh:
			switch m := rawMsg.(type) {
			case *msg.ReqWorkConn:
				go ctl.HandleReqWorkConn(m)
			case *msg.NewProxyResp:
				ctl.HandleNewProxyResp(m)
			case *msg.Pong:
				ctl.lastPong = time.Now()
				xl.Debug("receive heartbeat from server")
			}
		}
	}
}

func (ctl *Control) worker() {
	ctl.closedWg.Add(3)
	go ctl.msgHandler()
	go ctl.reader()
	go ctl.writer()

	<-ctl.closedCh
	close(ctl.stopCh)
	ctl.conn.Close()
	ctl.closedWg.Wait()

	ctl.pm.Close()
	close(ctl.doneCh)
}
