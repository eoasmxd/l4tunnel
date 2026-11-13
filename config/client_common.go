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

// ClientCommonConf 客户端全局配置。
type ClientCommonConf struct {
	ServerAddr string
	ServerPort int
	LogLevel   string
	Token      string
	PoolCount  int
}

func GetDefaultClientConf() ClientCommonConf {
	return ClientCommonConf{
		ServerAddr: "0.0.0.0",
		ServerPort: 3007,
		LogLevel:   "info",
		Token:      "",
		PoolCount:  1,
	}
}

func UnmarshalClientConfFromIni(content string) (cfg ClientCommonConf, err error) {
	cfg = GetDefaultClientConf()

	conf, err := ParseIni(content)
	if err != nil {
		return ClientCommonConf{}, fmt.Errorf("parse ini conf file error: %v", err)
	}

	var (
		tmpStr string
		ok     bool
	)
	if tmpStr, ok = conf.Get("common", "server_addr"); ok {
		cfg.ServerAddr = tmpStr
	}

	if tmpStr, ok = conf.Get("common", "server_port"); ok {
		v, err := strconv.Atoi(tmpStr)
		if err != nil || v <= 0 || v > 65535 {
			return ClientCommonConf{}, fmt.Errorf("parse conf error: invalid server_port")
		}
		cfg.ServerPort = v
	}

	if tmpStr, ok = conf.Get("common", "log_level"); ok {
		cfg.LogLevel = tmpStr
	}

	if tmpStr, ok = conf.Get("common", "token"); ok {
		cfg.Token = tmpStr
	}

	if tmpStr, ok = conf.Get("common", "pool_count"); ok {
		if v, err := strconv.Atoi(tmpStr); err == nil && v >= 0 {
			cfg.PoolCount = v
		}
	}

	return cfg, nil
}
