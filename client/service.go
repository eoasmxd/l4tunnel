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
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eoasmxd/l4tunnel/config"
	"github.com/eoasmxd/l4tunnel/utils"
)

type Service struct {
	runId   string
	ctl     *Control
	ctlMu   sync.RWMutex
	cfg     config.ClientCommonConf
	pxyCfgs map[string]*config.TcpProxyConf
	exit    uint32
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewService(cfg config.ClientCommonConf, pxyCfgs map[string]*config.TcpProxyConf) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		cfg:     cfg,
		pxyCfgs: pxyCfgs,
		ctx:     utils.NewLogContext(ctx, utils.NewLogger()),
		cancel:  cancel,
	}
}

func (svr *Service) Run() error {
	xl := utils.LogFromContextSafe(svr.ctx)

	for {
		conn, err := svr.login()
		if err != nil {
			xl.Warn("login to server failed: %v, will retry after 10s", err)
			time.Sleep(10 * time.Second)
			continue
		}

		ctl := NewControl(svr.ctx, svr.runId, conn, svr.cfg, svr.pxyCfgs)
		ctl.Run()
		svr.ctlMu.Lock()
		svr.ctl = ctl
		svr.ctlMu.Unlock()
		break
	}

	go svr.keepControllerWorking()

	<-svr.ctx.Done()
	return nil
}

func (svr *Service) keepControllerWorking() {
	xl := utils.LogFromContextSafe(svr.ctx)
	maxDelayTime := 20 * time.Second
	delayTime := time.Second

	for {
		svr.ctlMu.RLock()
		ctl := svr.ctl
		svr.ctlMu.RUnlock()
		if ctl != nil {
			<-ctl.ClosedDoneCh()
		}
		if atomic.LoadUint32(&svr.exit) != 0 {
			return
		}

		for {
			xl.Info("try to reconnect to server...")
			conn, err := svr.login()
			if err != nil {
				xl.Warn("reconnect to server error: %v", err)
				time.Sleep(delayTime)
				delayTime = delayTime * 2
				if delayTime > maxDelayTime {
					delayTime = maxDelayTime
				}
				continue
			}
			delayTime = time.Second

			ctl := NewControl(svr.ctx, svr.runId, conn, svr.cfg, svr.pxyCfgs)
			ctl.Run()
			svr.ctlMu.Lock()
			svr.ctl = ctl
			svr.ctlMu.Unlock()
			break
		}
	}
}

func (svr *Service) login() (conn net.Conn, err error) {
	xl := utils.LogFromContextSafe(svr.ctx)
	conn, err = utils.ConnectServerWithTLS(
		fmt.Sprintf("%s:%d", svr.cfg.ServerAddr, svr.cfg.ServerPort), utils.NewClientTLSConfig(svr.cfg.ServerAddr))
	if err != nil {
		return
	}

	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	now := time.Now().Unix()
	req, err := http.NewRequest("POST", "/l4t/connect", nil)
	if err != nil {
		return
	}
	req.Header.Set("X-L4T-Type", "login")
	req.Header.Set("X-L4T-Auth", utils.GetAuthKey(svr.cfg.Token, now))
	req.Header.Set("X-L4T-Timestamp", strconv.FormatInt(now, 10))
	req.Header.Set("X-L4T-Run-Id", svr.runId)
	req.Header.Set("X-L4T-Version", utils.Version())
	req.Header.Set("X-L4T-Pool-Count", strconv.Itoa(svr.cfg.PoolCount))

	if err = req.Write(conn); err != nil {
		return
	}

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if err == nil {
			err = fmt.Errorf("http handshake failed: %s", resp.Status)
		}
		return
	}
	conn.SetReadDeadline(time.Time{})

	svr.runId = resp.Header.Get("X-L4T-Run-Id")
	xl.ResetPrefixes()
	xl.AppendPrefix(svr.runId)

	xl.Info("login to server success, get run id [%s]", svr.runId)
	return
}

func (svr *Service) Close() {
	atomic.StoreUint32(&svr.exit, 1)
	svr.ctlMu.RLock()
	ctl := svr.ctl
	svr.ctlMu.RUnlock()
	if ctl != nil {
		ctl.Close()
	}
	svr.cancel()
}
