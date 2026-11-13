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

package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"sync"
	"time"
)

const defaultDialTimeout = 10 * time.Second

func ConnectServer(addr string) (c net.Conn, err error) {
	return net.DialTimeout("tcp", addr, defaultDialTimeout)
}

func ConnectServerWithTLS(addr string, tlsConfig *tls.Config) (c net.Conn, err error) {
	rawConn, err := ConnectServer(addr)
	if err != nil {
		return nil, err
	}

	if tlsConfig == nil {
		host, _, _ := net.SplitHostPort(addr)
		tlsConfig = NewClientTLSConfig(host)
	}

	tlsConn := tls.Client(rawConn, tlsConfig)
	tlsConn.SetDeadline(time.Now().Add(defaultDialTimeout))
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, err
	}
	tlsConn.SetDeadline(time.Time{})
	return tlsConn, nil
}

// Join 在两个连接之间双向转发数据，任一端结束时同时关闭两端的连接。
func Join(c1, c2 io.ReadWriteCloser) {
	var wg sync.WaitGroup
	pipe := func(dst, src io.ReadWriteCloser) {
		defer wg.Done()
		defer dst.Close()
		defer src.Close()
		_, _ = io.Copy(dst, src)
	}

	wg.Add(2)
	go pipe(c1, c2)
	go pipe(c2, c1)
	wg.Wait()
}

// WriteHttp404 向连接写入标准的 HTTP 404 响应。
func WriteHttp404(c io.Writer) {
	const resp404 = "HTTP/1.1 404 Not Found\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: 13\r\nConnection: close\r\n\r\n404 Not Found"
	_, _ = io.WriteString(c, resp404)
}

// NewClientTLSConfig 创建客户端 TLS 配置，若指定地址为域名则自动配置 SNI。
func NewClientTLSConfig(serverAddr string) *tls.Config {
	cfg := &tls.Config{
		InsecureSkipVerify: true,
	}
	if net.ParseIP(serverAddr) == nil && serverAddr != "" {
		cfg.ServerName = serverAddr
	}
	return cfg
}

// NewServerTLSConfig 加载指定的 TLS 证书，未指定时在内存中自动生成自签证书。
func NewServerTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}, nil
}

