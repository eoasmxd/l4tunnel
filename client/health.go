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
	"context"
	"net"
	"time"

	"github.com/eoasmxd/l4tunnel/utils"
)

const (
	defaultHealthCheckInterval = 10 * time.Second
	defaultHealthCheckTimeout  = 3 * time.Second
	defaultHealthCheckMaxFail  = 1
)

// HealthCheckMonitor 负责周期性探测本地服务四层连通性，并在状态变更时触发回调。
type HealthCheckMonitor struct {
	addr string

	failedTimes    uint64
	statusOK       bool
	statusNormalFn func()
	statusFailedFn func()

	ctx    context.Context
	cancel context.CancelFunc
}

func NewHealthCheckMonitor(ctx context.Context, addr string,
	statusNormalFn func(), statusFailedFn func()) *HealthCheckMonitor {

	newctx, cancel := context.WithCancel(ctx)
	return &HealthCheckMonitor{
		addr:           addr,
		statusOK:       false,
		statusNormalFn: statusNormalFn,
		statusFailedFn: statusFailedFn,
		ctx:            newctx,
		cancel:         cancel,
	}
}

func (monitor *HealthCheckMonitor) Start() {
	go monitor.checkWorker()
}

func (monitor *HealthCheckMonitor) Stop() {
	monitor.cancel()
}

func (monitor *HealthCheckMonitor) checkWorker() {
	xl := utils.LogFromContextSafe(monitor.ctx)
	for {
		doCtx, cancel := context.WithDeadline(monitor.ctx, time.Now().Add(defaultHealthCheckTimeout))
		err := monitor.doTcpCheck(doCtx)

		select {
		case <-monitor.ctx.Done():
			cancel()
			return
		default:
			cancel()
		}

		if err == nil {
			xl.Trace("do one health check success")
			monitor.failedTimes = 0
			if !monitor.statusOK && monitor.statusNormalFn != nil {
				xl.Info("health check status change to success")
				monitor.statusOK = true
				monitor.statusNormalFn()
			}
		} else {
			xl.Warn("do one health check failed: %v", err)
			monitor.failedTimes++
			if monitor.statusOK && int(monitor.failedTimes) >= defaultHealthCheckMaxFail && monitor.statusFailedFn != nil {
				xl.Warn("health check status change to failed")
				monitor.statusOK = false
				monitor.statusFailedFn()
			}
		}

		select {
		case <-monitor.ctx.Done():
			return
		case <-time.After(defaultHealthCheckInterval):
		}
	}
}

func (monitor *HealthCheckMonitor) doTcpCheck(ctx context.Context) error {
	if monitor.addr == "" {
		return nil
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", monitor.addr)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

