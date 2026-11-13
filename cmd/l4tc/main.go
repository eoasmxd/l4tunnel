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

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/eoasmxd/l4tunnel/client"
	"github.com/eoasmxd/l4tunnel/config"
	"github.com/eoasmxd/l4tunnel/utils"
)

func main() {
	var (
		cfgFile     string
		showVersion bool
	)
	flag.StringVar(&cfgFile, "c", "./l4tc.ini", "config file of l4tc")
	flag.BoolVar(&showVersion, "v", false, "version of l4tc")
	flag.Parse()

	if showVersion {
		fmt.Println(utils.Version())
		return
	}

	if err := runClient(cfgFile); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runClient(cfgFilePath string) error {
	content, err := config.LoadConfFromFile(cfgFilePath)
	if err != nil {
		return err
	}

	cfg, err := config.UnmarshalClientConfFromIni(content)
	if err != nil {
		return err
	}

	pxyCfgs, err := config.LoadAllConfFromIni(content)
	if err != nil {
		return err
	}

	utils.InitLog(cfg.LogLevel)
	svr := client.NewService(cfg, pxyCfgs)
	return svr.Run()
}
