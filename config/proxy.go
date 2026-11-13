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

package config

import (
	"fmt"
	"strconv"

	"github.com/eoasmxd/l4tunnel/msg"
)

// TcpProxyConf 四层 TCP 端口映射配置。
type TcpProxyConf struct {
	ProxyName       string
	LocalIp         string
	LocalPort       int
	RemotePort      int
	HealthCheck     bool
	HealthCheckAddr string
}

func NewProxyConfFromMsg(pMsg *msg.NewProxy) (*TcpProxyConf, error) {
	if pMsg.RemotePort <= 0 || pMsg.RemotePort > 65535 {
		return nil, fmt.Errorf("invalid remote_port: %d", pMsg.RemotePort)
	}
	return &TcpProxyConf{
		ProxyName:  pMsg.ProxyName,
		RemotePort: pMsg.RemotePort,
	}, nil
}

func NewProxyConfFromIni(name string, section map[string]string) (*TcpProxyConf, error) {
	cfg := &TcpProxyConf{
		ProxyName: name,
	}

	if cfg.LocalIp = section["local_ip"]; cfg.LocalIp == "" {
		cfg.LocalIp = "127.0.0.1"
	}

	if tmpStr, ok := section["local_port"]; ok {
		v, err := strconv.Atoi(tmpStr)
		if err != nil || v <= 0 || v > 65535 {
			return nil, fmt.Errorf("Parse conf error: proxy [%s] local_port error", name)
		}
		cfg.LocalPort = v
	} else {
		return nil, fmt.Errorf("Parse conf error: proxy [%s] local_port not found", name)
	}

	if tmpStr, ok := section["remote_port"]; ok {
		v, err := strconv.Atoi(tmpStr)
		if err != nil || v <= 0 || v > 65535 {
			return nil, fmt.Errorf("Parse conf error: proxy [%s] remote_port error", name)
		}
		cfg.RemotePort = v
	} else {
		return nil, fmt.Errorf("Parse conf error: proxy [%s] remote_port not found", name)
	}

	if tmpStr, ok := section["health_check"]; ok && tmpStr == "true" {
		cfg.HealthCheck = true
		cfg.HealthCheckAddr = fmt.Sprintf("%s:%d", cfg.LocalIp, cfg.LocalPort)
	}

	return cfg, nil
}

func (cfg *TcpProxyConf) MarshalToMsg(pMsg *msg.NewProxy) {
	pMsg.ProxyName = cfg.ProxyName
	pMsg.RemotePort = cfg.RemotePort
}

func LoadAllConfFromIni(content string) (proxyConfs map[string]*TcpProxyConf, err error) {
	conf, errRet := ParseIni(content)
	if errRet != nil {
		return nil, errRet
	}

	proxyConfs = make(map[string]*TcpProxyConf)
	for name, section := range conf {
		if name == "common" {
			continue
		}
		cfg, errRet := NewProxyConfFromIni(name, section)
		if errRet != nil {
			return nil, errRet
		}
		proxyConfs[name] = cfg
	}
	return proxyConfs, nil
}
