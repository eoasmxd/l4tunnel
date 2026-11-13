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
)

// ServerCommonConf 服务端全局配置。
type ServerCommonConf struct {
	BindAddr      string
	BindPort      int
	ProxyBindAddr string
	LogLevel      string
	Token         string
	TLSCertFile   string
	TLSKeyFile    string
}

func GetDefaultServerConf() ServerCommonConf {
	return ServerCommonConf{
		BindAddr:      "0.0.0.0",
		BindPort:      3007,
		ProxyBindAddr: "0.0.0.0",
		LogLevel:      "info",
		Token:         "",
	}
}

func UnmarshalServerConfFromIni(content string) (cfg ServerCommonConf, err error) {
	cfg = GetDefaultServerConf()

	conf, err := ParseIni(content)
	if err != nil {
		return ServerCommonConf{}, fmt.Errorf("parse ini conf file error: %v", err)
	}

	var (
		tmpStr string
		ok     bool
	)
	if tmpStr, ok = conf.Get("common", "bind_addr"); ok {
		cfg.BindAddr = tmpStr
	}

	if tmpStr, ok = conf.Get("common", "bind_port"); ok {
		v, err := strconv.Atoi(tmpStr)
		if err != nil || v <= 0 || v > 65535 {
			return ServerCommonConf{}, fmt.Errorf("parse conf error: invalid bind_port")
		}
		cfg.BindPort = v
	}

	if tmpStr, ok = conf.Get("common", "proxy_bind_addr"); ok {
		cfg.ProxyBindAddr = tmpStr
	} else {
		cfg.ProxyBindAddr = cfg.BindAddr
	}

	if tmpStr, ok = conf.Get("common", "log_level"); ok {
		cfg.LogLevel = tmpStr
	}

	cfg.Token, _ = conf.Get("common", "token")
	cfg.TLSCertFile, _ = conf.Get("common", "tls_cert_file")
	cfg.TLSKeyFile, _ = conf.Get("common", "tls_key_file")
	return cfg, nil
}

func (cfg *ServerCommonConf) Check() (err error) {
	if (cfg.TLSCertFile != "" && cfg.TLSKeyFile == "") || (cfg.TLSCertFile == "" && cfg.TLSKeyFile != "") {
		return fmt.Errorf("Parse conf error: tls_cert_file and tls_key_file must be set together")
	}
	return nil
}
