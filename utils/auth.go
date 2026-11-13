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

package utils

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"strconv"
)

// RandId 生成默认 8 字节长度的随机十六进制字符串作为标识。
func RandId() (id string, err error) {
	return RandIdWithLen(8)
}

// RandIdWithLen 生成指定字节长度的随机十六进制字符串。
func RandIdWithLen(idLen int) (id string, err error) {
	b := make([]byte, idLen)
	_, err = rand.Read(b)
	if err != nil {
		return
	}
	return hex.EncodeToString(b), nil
}

func GetAuthKey(token string, timestamp int64) (key string) {
	h := md5.Sum([]byte(token + strconv.FormatInt(timestamp, 10)))
	return hex.EncodeToString(h[:])
}

