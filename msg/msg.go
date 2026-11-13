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

package msg

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

const (
	TypeNewProxy      = 'p'
	TypeNewProxyResp  = '2'
	TypeCloseProxy    = 'c'
	TypeReqWorkConn   = 'r'
	TypeStartWorkConn = 's'
	TypePing          = 'h'
	TypePong          = '4'
)

const maxMsgLength = 10 * 1024 * 1024

type Message interface{}

type NewProxy struct {
	ProxyName  string
	RemotePort int
}

type NewProxyResp struct {
	ProxyName string
	Error     string
}

type CloseProxy struct {
	ProxyName string
}

type ReqWorkConn struct{}

type StartWorkConn struct {
	ProxyName string
}

type Ping struct{}

type Pong struct{}

var (
	typeByteMap = make(map[reflect.Type]byte)
	byteTypeMap = make(map[byte]reflect.Type)
)

func init() {
	register := func(b byte, m interface{}) {
		t := reflect.TypeOf(m)
		byteTypeMap[b] = t
		typeByteMap[t] = b
	}
	register(TypeNewProxy, NewProxy{})
	register(TypeNewProxyResp, NewProxyResp{})
	register(TypeCloseProxy, CloseProxy{})
	register(TypeReqWorkConn, ReqWorkConn{})
	register(TypeStartWorkConn, StartWorkConn{})
	register(TypePing, Ping{})
	register(TypePong, Pong{})
}

func readRawMsg(c io.Reader) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(c, header[:]); err != nil {
		return 0, nil, err
	}

	length := binary.BigEndian.Uint32(header[1:5])
	if length > maxMsgLength {
		return 0, nil, fmt.Errorf("message length %d exceeds max limit", length)
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(c, payload); err != nil {
			return 0, nil, err
		}
	}
	return header[0], payload, nil
}

// ReadMsg 从连接读取一条协议消息并反序列化为对应结构体指针。
func ReadMsg(c io.Reader) (Message, error) {
	typeByte, payload, err := readRawMsg(c)
	if err != nil {
		return nil, err
	}

	t, ok := byteTypeMap[typeByte]
	if !ok {
		return nil, fmt.Errorf("unknown message type: %q", typeByte)
	}

	v := reflect.New(t).Interface()
	if err := json.Unmarshal(payload, v); err != nil {
		return nil, err
	}
	return v, nil
}

// ReadMsgInto 从连接读取消息并填充到指定的结构体指针中。
func ReadMsgInto(c io.Reader, m Message) error {
	typeByte, payload, err := readRawMsg(c)
	if err != nil {
		return err
	}

	v := reflect.ValueOf(m)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return errors.New("m must be a non-nil pointer")
	}

	expectedType, ok := byteTypeMap[typeByte]
	if !ok || expectedType != v.Elem().Type() {
		return fmt.Errorf("message type mismatch: got %q, expect %s", typeByte, v.Elem().Type().Name())
	}

	return json.Unmarshal(payload, m)
}

// WriteMsg 将消息按 [1B类型 + 4B大端长度 + JSON] 帧格式写入连接。
func WriteMsg(c io.Writer, m interface{}) error {
	v := reflect.ValueOf(m)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	typeByte, ok := typeByteMap[v.Type()]
	if !ok {
		return errors.New("unknown message type")
	}

	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}

	frame := make([]byte, 5+len(payload))
	frame[0] = typeByte
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)

	_, err = c.Write(frame)
	return err
}
