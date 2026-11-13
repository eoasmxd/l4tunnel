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
	"bufio"
	"fmt"
	"io/ioutil"
	"strings"
)

// LoadConfFromFile 读取配置文件原始内容。
func LoadConfFromFile(path string) (string, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type iniSections map[string]map[string]string

func (s iniSections) Get(section, key string) (string, bool) {
	sec, ok := s[section]
	if !ok {
		return "", false
	}
	v, ok := sec[key]
	return v, ok
}

// ParseIni 使用标准库解析简单的 INI 配置文件。
func ParseIni(content string) (iniSections, error) {
	sections := make(iniSections)
	current := ""

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			if current == "" {
				return nil, fmt.Errorf("empty section name at line %d", lineNum)
			}
			if _, ok := sections[current]; !ok {
				sections[current] = make(map[string]string)
			}
			continue
		}

		if current == "" {
			return nil, fmt.Errorf("key outside section at line %d", lineNum)
		}

		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			return nil, fmt.Errorf("invalid ini syntax at line %d", lineNum)
		}
		key := strings.TrimSpace(line[:idx])
		val := stripInlineComment(strings.TrimSpace(line[idx+1:]))
		sections[current][key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return sections, nil
}

func stripInlineComment(s string) string {
	for i := 0; i < len(s); i++ {
		if (s[i] == '#' || s[i] == ';') && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}
