# L4Tunnel (纯四层 TCP 端口反向代理工具)

本项目基于经典的 `frp v0.29.1` 进行深度裁剪与现代化改造，专注于最核心、最可靠的**纯四层（Layer 4）TCP 端口反向隧道**。

项目仅依赖 Go 标准库（零第三方依赖），保留并强化了四层 TCP 端口转发、预建连接池、标准 TLS 传输加密、HTTP 握手认证与伪装防御、断线重连以及本地服务健康检查机制，致力于提供一个极简、易审查、安全轻量的四层穿透工具。

---

## 适用场景

1. **HTTP/HTTPS 穿透**：通过纯 TCP 端口一对一穿透，公网服务器外层统一挂载 Nginx/Caddy 等专业反代网关完成域名解析与证书管理。
2. **SSH 运维穿透**：映射跳板机 22 端口，外部可通过原生 SSH `ProxyJump` 命令无缝穿透访问内网网段。
3. **远程桌面与私有服务**：Windows RDP（3389）、MySQL、Redis 等原生 TCP 服务的单端口反向映射。

---

## 核心保留特性

- **纯四层 TCP 隧道**：仅支持四层 TCP 端口映射，不提供多协议选择，彻底杜绝应用层解析漏洞与性能开销。
- **标准 TLS 传输与密码校验**：全链路强制使用标准 TLS 加密（服务端支持自配证书或在内存中自动生成自签证书，客户端免证书校验），并支持 `token` 接入密码校验。
- **服务健康检查（Health Check）**：支持在客户端侧对本地真实服务进行四层 TCP 连通性周期探测，服务异常时自动向服务端摘除，恢复后自动重新注册。

---

## 快速使用

### 1. 服务端配置与启动 (l4ts)

编写 `l4ts.ini`：

```ini
[common]
bind_addr = 0.0.0.0
bind_port = 3007

# 认证凭据
token = your_secure_token
```

启动服务端：
```bash
./l4ts -c ./l4ts.ini
```

### 2. 客户端配置与启动 (l4tc)

编写 `l4tc.ini`：

```ini
[common]
server_addr = x.x.x.x
server_port = 3007
token = your_secure_token

# 示例：映射本地 SSH 22 端口至公网 6000 端口
[ssh]
local_ip = 127.0.0.1
local_port = 22
remote_port = 6000

# 示例：映射本地 Web 服务并配置健康检查
[web]
local_ip = 127.0.0.1
local_port = 8080
remote_port = 8080
health_check = true
```

启动客户端：
```bash
./l4tc -c ./l4tc.ini
```

### 3. Docker 部署

服务端与客户端均提供了独立的轻量容器镜像（发布至 `ghcr.io` 与 Docker Hub），容器内统一约定配置文件路径为 `/etc/l4tunnel/`：

#### 服务端 (l4ts)
```bash
# 将宿主机配置目录挂载至容器 /etc/l4tunnel
docker run -d \
  --name l4ts \
  --restart always \
  --network host \
  -v /path/to/conf:/etc/l4tunnel \
  ghcr.io/eoasmxd/l4ts:latest
```

#### 客户端 (l4tc)
```bash
docker run -d \
  --name l4tc \
  --restart always \
  --network host \
  -v /path/to/conf:/etc/l4tunnel \
  ghcr.io/eoasmxd/l4tc:latest
```

---

## 最佳实践与集成指南

### 1. 基于 Nginx SNI 复用公网 443 端口（服务端口与网站合二为一）

因为 `l4tunnel` 控制与传输链路全量运行在标准 TLS 之上，因此 **`l4ts` 服务端口本身也可以通过 Nginx 四层 `stream` 模块的 SNI 分流，直接与常规 HTTPS 网站复用公网 443 端口**。公网无需单独额外放行控制端口：

1. **服务端 `l4ts.ini`**：监听本地回环地址
   ```ini
   [common]
   bind_addr = 127.0.0.1
   bind_port = 3007
   token = your_secure_token
   ```
2. **公网 Nginx 配置（`nginx.conf` 顶层 `stream` 块）**：
   ```nginx
   stream {
       map $ssl_preread_server_name $backend_name {
           # 穿透服务专用域名，透传给 l4ts 服务端
           tunnel.yourdomain.com    l4ts_backend;
           # 其它所有域名，分流给本地 Nginx 的七层 HTTPS 模块处理
           default                  web_backend;
       }

       upstream l4ts_backend {
           server 127.0.0.1:3007;
       }

       upstream web_backend {
           server 127.0.0.1:8443; # 本地 Nginx http server 监听该端口
       }

       server {
           listen 443;
           ssl_preread on; # 启用 SNI 预读，无须解密 TLS 即可透明路由
           proxy_pass $backend_name;
       }
   }
   ```
3. **客户端 `l4tc.ini`**：直接连接公网 443 端口
   ```ini
   [common]
   server_addr = tunnel.yourdomain.com
   server_port = 443
   token = your_secure_token
   ```

### 2. 内网 Web 穿透由 Nginx 反代托管（多站点/SSL 统一管理）

无需在穿透工具中内置复杂的七层 HTTP 引擎。推荐模式是**将内网 Web 服务映射到公网服务器本地回环地址（`127.0.0.1`），再由公网 Nginx 统一对外托管**：

1. **客户端 `l4tc.ini`**：将内网 Web 服务映射到服务端回环端口
   ```ini
   [my-web]
   local_ip = 127.0.0.1
   local_port = 8080
   remote_port = 18080
   ```
2. **公网 Nginx 配置**：七层 `server` 块监听内部端口（由上述 SNI 分流转入）并转发至本地映射端口
   ```nginx
   server {
       listen 80;
       # 若配合上述第 1 点的 SNI 分流，此处监听本地 8443；独立部署时则可直接监听 443 ssl
       listen 127.0.0.1:8443 ssl http2;
       server_name app.yourdomain.com;

       ssl_certificate /path/to/fullchain.pem;
       ssl_certificate_key /path/to/privkey.pem;

       location / {
           proxy_pass http://127.0.0.1:18080;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header X-Forwarded-Proto $scheme;
       }
   }
   ```
   *优势：公网只开放统一的 80/443，多个内网 Web 站点通过域名自然复用端口，且证书续期和访问日志由 Nginx 专业处理。*

### 3. 结合 SSH 作为跳板机（直通内网全网段）

只需在内网一台机器上运行 `l4tc` 映射其 22 端口，外部即可通过该机器作为**跳板机（Bastion Host）**直连内网所有服务器：

1. **客户端 `l4tc.ini`**：映射跳板机 SSH 端口
   ```ini
   [bastion-ssh]
   local_ip = 127.0.0.1
   local_port = 22
   remote_port = 6000
   ```
2. **外部管理机 `~/.ssh/config` 配置**：
   ```ssh-config
   # 定义外网访问的跳板机
   Host bastion
       HostName x.x.x.x
       Port 6000
       User root

   # 通过跳板机直连内网 192.168.1.x 网段的任何机器
   Host 192.168.1.*
       ProxyJump bastion
       User root
   ```
3. **使用体验**：
   终端直接执行 `ssh root@192.168.1.100`，SSH 会自动通过公网 6000 端口完成透明跳板转发，内网其他数十台服务器无需逐一配置穿透客户端。

---

## 构建与编译

本项目采用纯 Go 标准库实现，零第三方依赖，可直接使用原生 `go` 命令编译或使用 `make`：

### 1. 原生 Go 编译（推荐，全平台通用）

```bash
# 编译客户端
go build -o bin/l4tc cmd/l4tc/main.go

# 编译服务端
go build -o bin/l4ts cmd/l4ts/main.go
```
*(Windows 环境下可指定为 `bin/l4tc.exe` 与 `bin/l4ts.exe`)*

### 2. 使用 Makefile 编译

```bash
# 本地编译
make

# 交叉编译全平台二进制
make -f ./Makefile.cross-compiles
```

---

## 开源协议

本项目采用 [Apache License 2.0](LICENSE) 协议开源。
部分核心架构与设计衍生自 [fatedier/frp](https://github.com/fatedier/frp)（遵从 Apache License 2.0），详细版权及原作者声明参见 [NOTICE](NOTICE) 文件。
