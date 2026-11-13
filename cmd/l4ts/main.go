// Copyright 2018 fatedier, fatedier@gmail.com
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

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/eoasmxd/l4tunnel/config"
	"github.com/eoasmxd/l4tunnel/server"
	"github.com/eoasmxd/l4tunnel/utils"
)

func main() {
	var (
		cfgFile     string
		showVersion bool
	)
	flag.StringVar(&cfgFile, "c", "./l4ts.ini", "config file of l4ts")
	flag.BoolVar(&showVersion, "v", false, "version of l4ts")
	flag.Parse()

	if showVersion {
		fmt.Println(utils.Version())
		return
	}

	if err := runServer(cfgFile); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runServer(cfgFilePath string) error {
	content, err := config.LoadConfFromFile(cfgFilePath)
	if err != nil {
		return err
	}
	cfg, err := config.UnmarshalServerConfFromIni(content)
	if err != nil {
		return err
	}
	if err = cfg.Check(); err != nil {
		return err
	}

	utils.InitLog(cfg.LogLevel)
	svr, err := server.NewService(cfg)
	if err != nil {
		return err
	}
	utils.Info("start l4ts success")
	svr.Run()
	return nil
}
