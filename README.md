# L4Tunnel (Pure Layer-4 TCP Reverse Proxy)

L4Tunnel is deeply customized and modernized from the classic `frp v0.29.1`, focusing strictly on the most essential and reliable **Layer-4 (TCP) reverse port tunneling**.

Implemented purely using the Go standard library (zero third-party dependencies), L4Tunnel preserves and enhances core capabilities including raw TCP port forwarding, pre-warmed connection pooling, mandatory standard TLS encryption, HTTP handshake authentication with honeypot-style defense (404 on unauthenticated probing), automatic reconnection, and client-side health checks.

---

## Use Cases

1. **HTTP/HTTPS Traffic**: Pure TCP port pass-through. Use Nginx or Caddy on the public server for domain routing and SSL termination.
2. **SSH Bastion**: Forward internal SSH (port 22), accessible via native SSH `ProxyJump`.
3. **Remote Desktop & Private Services**: Direct mapping for Windows RDP (3389), database ports, etc.

---

## Key Features

- **Pure Layer-4 TCP**: Only supports TCP port forwarding without multi-protocol configuration.
- **Standard TLS & Auth**: Mandatory standard TLS transport (supports custom cert/key or auto-generated in-memory certificate) with optional token password authentication.
- **Service Health Check**: Client-side Layer-4 TCP health checks with automatic recovery and deregistration.

---

## Quick Start

### 1. Server (l4ts)

`l4ts.ini`:
```ini
[common]
bind_addr = 0.0.0.0
bind_port = 3007
token = your_secure_token
```

Run:
```bash
./l4ts -c ./l4ts.ini
```

### 2. Client (l4tc)

`l4tc.ini`:
```ini
[common]
server_addr = x.x.x.x
server_port = 3007
token = your_secure_token

[ssh]
local_ip = 127.0.0.1
local_port = 22
remote_port = 6000

# Layer-4 TCP health check (optional, defaults: 10s interval, 3s timeout)
# health_check = true
```

Run:
```bash
./l4tc -c ./l4tc.ini
```

### 3. Docker Deployment

Separate lightweight images are provided for server and client (published to `ghcr.io` and Docker Hub). The default config directory inside containers is `/etc/l4tunnel/`:

#### Server (l4ts)
```bash
# Mount host config directory to container /etc/l4tunnel
docker run -d \
  --name l4ts \
  --restart always \
  --network host \
  -v /path/to/conf:/etc/l4tunnel \
  ghcr.io/eoasmxd/l4ts:latest
```

#### Client (l4tc)
```bash
docker run -d \
  --name l4tc \
  --restart always \
  --network host \
  -v /path/to/conf:/etc/l4tunnel \
  ghcr.io/eoasmxd/l4tc:latest
```

---

## Best Practices & Integration Guide

### 1. Multiplex Public Port 443 via Nginx SNI (Tunnel & Web on Port 443)

Because L4Tunnel's control and data channels run on standard TLS, **`l4ts` can directly share public port 443 with your regular HTTPS websites using Nginx's Layer-4 `stream` module and SNI preread**. No extra ports need to be exposed:

1. **Server `l4ts.ini`**: Listen on local loopback
   ```ini
   [common]
   bind_addr = 127.0.0.1
   bind_port = 3007
   token = your_secure_token
   ```
2. **Public Nginx Configuration (`stream` block in `nginx.conf`)**:
   ```nginx
   stream {
       map $ssl_preread_server_name $backend_name {
           # Dedicated domain for tunneling -> route to l4ts
           tunnel.yourdomain.com    l4ts_backend;
           # All other domains -> route to local Nginx Layer-7 HTTPS
           default                  web_backend;
       }

       upstream l4ts_backend {
           server 127.0.0.1:3007;
       }

       upstream web_backend {
           server 127.0.0.1:8443; # Local Nginx http server
       }

       server {
           listen 443;
           ssl_preread on; # Inspect SNI without terminating TLS
           proxy_pass $backend_name;
       }
   }
   ```
3. **Client `l4tc.ini`**: Connect directly via public port 443
   ```ini
   [common]
   server_addr = tunnel.yourdomain.com
   server_port = 443
   token = your_secure_token
   ```

### 2. Host Internal Web Services Behind Nginx (Multi-Domain & SSL Management)

No complex Layer-7 HTTP engine is needed inside L4Tunnel. The recommended approach is **mapping internal web services to the server's local loopback (`127.0.0.1`), managed centrally by Nginx**:

1. **Client `l4tc.ini`**: Map web service to a local loopback port on the server
   ```ini
   [my-web]
   local_ip = 127.0.0.1
   local_port = 8080
   remote_port = 18080
   ```
2. **Public Nginx Configuration**: Layer-7 `server` listens on internal port (routed from SNI)
   ```nginx
   server {
       listen 80;
       # Listen on 127.0.0.1:8443 when paired with SNI preread above; or 443 ssl if standalone
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
   *Advantage: Only public port 80/443 needs to be open. Multiple internal web services share port 443 via hostnames, while certificate auto-renewal and access logs are handled centrally by Nginx.*

### 3. SSH Bastion Host (Transparent Access to Internal Subnets)

Only one internal machine needs to expose port 22 via `l4tc` to act as a **bastion host**:

1. **Client `l4tc.ini`**:
   ```ini
   [bastion-ssh]
   local_ip = 127.0.0.1
   local_port = 22
   remote_port = 6000
   ```
2. **Local Machine `~/.ssh/config`**:
   ```ssh-config
   # The external bastion host
   Host bastion
       HostName x.x.x.x
       Port 6000
       User root

   # Jump through bastion to any 192.168.1.x machine
   Host 192.168.1.*
       ProxyJump bastion
       User root
   ```
3. **Usage**:
   Simply run `ssh root@192.168.1.100`. SSH will automatically and transparently jump through port 6000 without requiring L4Tunnel on every internal server.

---

## Build

Pure Go standard library implementation with zero third-party dependencies. You can build directly with standard `go` toolchain or using `make`:

### 1. Direct Go Build (Recommended, Cross-Platform)

```bash
# Build client
go build -o bin/l4tc cmd/l4tc/main.go

# Build server
go build -o bin/l4ts cmd/l4ts/main.go
```
*(On Windows, you can output as `bin/l4tc.exe` and `bin/l4ts.exe`)*

### 2. Using Makefile

```bash
# Local build
make

# Cross-compilation for all platforms
make -f ./Makefile.cross-compiles
```

---

## License

L4Tunnel is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for details.

This project is a customized, minimalist Layer-4 refactoring based on [frp](https://github.com/fatedier/frp) by fatedier.

