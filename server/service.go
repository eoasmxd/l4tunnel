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
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/eoasmxd/l4tunnel/config"
	"github.com/eoasmxd/l4tunnel/utils"
)

const connReadTimeout = 10 * time.Second

type Service struct {
	listener   net.Listener
	ctlManager *ControlManager
	pxyManager *ProxyManager
	tlsConfig  *tls.Config
	cfg        config.ServerCommonConf
}

func NewService(cfg config.ServerCommonConf) (*Service, error) {
	tlsConfig, err := utils.NewServerTLSConfig(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("create server tls config error: %v", err)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.BindAddr, cfg.BindPort))
	if err != nil {
		return nil, fmt.Errorf("create server listener error: %v", err)
	}

	svr := &Service{
		ctlManager: NewControlManager(),
		pxyManager: NewProxyManager(),
		tlsConfig:  tlsConfig,
		listener:   tls.NewListener(ln, tlsConfig),
		cfg:        cfg,
	}
	utils.Info("l4ts tcp listen on %s:%d", cfg.BindAddr, cfg.BindPort)
	return svr, nil
}

func (svr *Service) Run() {
	svr.HandleListener(svr.listener)
}

func (svr *Service) HandleListener(l net.Listener) {
	for {
		c, err := l.Accept()
		if err != nil {
			utils.Warn("Listener for incoming connections from client closed")
			return
		}

		go func(conn net.Conn) {
			conn.SetReadDeadline(time.Now().Add(connReadTimeout))
			req, err := http.ReadRequest(bufio.NewReader(conn))
			if err != nil {
				utils.WriteHttp404(conn)
				conn.Close()
				return
			}

			timestampStr := req.Header.Get("X-L4T-Timestamp")
			ts, _ := strconv.ParseInt(timestampStr, 10, 64)
			authKey := req.Header.Get("X-L4T-Auth")
			if authKey == "" || utils.GetAuthKey(svr.cfg.Token, ts) != authKey {
				utils.WriteHttp404(conn)
				conn.Close()
				return
			}

			connType := req.Header.Get("X-L4T-Type")
			runId := req.Header.Get("X-L4T-Run-Id")
			if connType == "login" && runId == "" {
				var err error
				runId, err = utils.RandId()
				if err != nil {
					conn.Close()
					return
				}
			}

			upgradeResp := fmt.Sprintf("HTTP/1.1 200 OK\r\nConnection: Upgrade\r\nX-L4T-Run-Id: %s\r\nX-L4T-Version: %s\r\n\r\n", runId, utils.Version())
			if _, err := io.WriteString(conn, upgradeResp); err != nil {
				conn.Close()
				return
			}
			conn.SetReadDeadline(time.Time{})

			if connType == "login" {
				poolCount, _ := strconv.Atoi(req.Header.Get("X-L4T-Pool-Count"))
				version := req.Header.Get("X-L4T-Version")
				if err := svr.RegisterControl(conn, runId, version, poolCount); err != nil {
					conn.Close()
				}
			} else if connType == "work" {
				svr.RegisterWorkConn(conn, runId)
			} else {
				utils.WriteHttp404(conn)
				conn.Close()
			}
		}(c)
	}
}

func (svr *Service) RegisterControl(ctlConn net.Conn, runId string, version string, poolCount int) error {
	xl := utils.NewLogger().AppendPrefix(runId)
	ctx := utils.NewLogContext(context.Background(), xl)
	xl.Info("client login info: ip [%s] version [%s]", ctlConn.RemoteAddr().String(), version)

	ctl := NewControl(ctx, svr.pxyManager, ctlConn, runId, poolCount, svr.cfg)

	if oldCtl := svr.ctlManager.Add(runId, ctl); oldCtl != nil {
		oldCtl.WaitClosed()
	}

	ctl.Start()

	go func() {
		ctl.WaitClosed()
		svr.ctlManager.Del(runId, ctl)
	}()
	return nil
}

func (svr *Service) RegisterWorkConn(workConn net.Conn, runId string) {
	xl := utils.NewLogger()
	ctl, exist := svr.ctlManager.GetById(runId)
	if !exist {
		xl.Warn("No client control found for run id [%s]", runId)
		workConn.Close()
		return
	}
	ctl.RegisterWorkConn(workConn)
}
