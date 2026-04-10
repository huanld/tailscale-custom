# Hướng dẫn cài đặt Headscale Server

## Mục lục
1. [Tổng quan](#tổng-quan)
2. [Yêu cầu hệ thống](#yêu-cầu-hệ-thống)
3. [Cài đặt Headscale bằng Docker](#cài-đặt-headscale-bằng-docker)
4. [Cấu hình domain & reverse proxy](#cấu-hình-domain--reverse-proxy)
5. [Quản lý users & nodes](#quản-lý-users--nodes)
6. [API Reference cho Web Admin](#api-reference-cho-web-admin)

---

## Tổng quan

Headscale là bản self-hosted của control server Tailscale. Nó cho phép:
- Tạo mạng riêng ảo (VPN mesh) giữa các máy
- Không phụ thuộc vào server của Tailscale
- Toàn quyền quản lý users, nodes, ACL

**Kiến trúc:**
```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Client Windows  │────▶│  Headscale       │◀────│ Client Linux    │
│ (Tailscale-     │     │  (vpn.softs.     │     │ (Tailscale-     │
│  Custom)        │     │   business)      │     │  Custom)        │
└─────────────────┘     │  Port: 443       │     └─────────────────┘
                        │  gRPC + HTTPS    │
                        └──────────────────┘
                               ▲
                               │
                        ┌──────────────────┐
                        │  Web Admin App   │
                        │  (quản lý nodes) │
                        └──────────────────┘
```

---

## Yêu cầu hệ thống

- VPS Linux (Ubuntu 22.04+ hoặc Debian 12+)
- Docker + Docker Compose
- Domain trỏ về VPS (ví dụ: `vpn.softs.business`)
- Port 443 (HTTPS) mở

---

## Cài đặt Headscale bằng Docker

### 1. Tạo thư mục cấu hình

```bash
mkdir -p /opt/headscale/config /opt/headscale/data
```

### 2. Tạo file cấu hình `/opt/headscale/config/config.yaml`

```yaml
server_url: https://vpn.softs.business
listen_addr: 0.0.0.0:8080
metrics_listen_addr: 0.0.0.0:9090

# gRPC cho Tailscale client
grpc_listen_addr: 0.0.0.0:50443
grpc_allow_insecure: false

# Database
database:
  type: sqlite
  sqlite:
    path: /var/lib/headscale/db.sqlite

# Khoảng IP cấp cho các node
prefixes:
  v4: 100.64.0.0/10
  v6: fd7a:115c:a1e0::/48

# DERP (relay khi P2P không được)
derp:
  server:
    enabled: true
    region_id: 999
    region_code: custom
    region_name: "Custom DERP"
    stun_listen_addr: 0.0.0.0:3478

  urls: []
  paths: []
  auto_update_enabled: true
  update_frequency: 24h

# DNS - KHÔNG bật MagicDNS để tránh xung đột
dns:
  magic_dns: false
  base_domain: vpn.local
  nameservers:
    global: []

# Thời gian hiệu lực node key
node_key_expiry: 0  # không hết hạn

# Bật API (quan trọng cho Web Admin)
# API key tạo bằng: headscale apikeys create
```

### 3. Tạo `docker-compose.yml`

```yaml
version: "3.9"
services:
  headscale:
    image: headscale/headscale:latest
    container_name: headscale
    restart: always
    volumes:
      - /opt/headscale/config:/etc/headscale
      - /opt/headscale/data:/var/lib/headscale
    ports:
      - "8080:8080"       # HTTP API
      - "9090:9090"       # Metrics
      - "3478:3478/udp"   # STUN
    command: serve
    environment:
      - TZ=Asia/Ho_Chi_Minh
```

### 4. Reverse proxy (Nginx)

```nginx
server {
    listen 443 ssl http2;
    server_name vpn.softs.business;

    ssl_certificate     /etc/ssl/certs/vpn.softs.business.crt;
    ssl_certificate_key /etc/ssl/private/vpn.softs.business.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }
}
```

### 5. Khởi động

```bash
cd /opt/headscale
docker compose up -d

# Tạo user đầu tiên
docker exec headscale headscale users create default

# Tạo API key (dùng cho Web Admin)
docker exec headscale headscale apikeys create --expiration 365d
# Lưu lại key này!
```

---

## Quản lý users & nodes

### Tạo user
```bash
docker exec headscale headscale users create <username>
```

### Đăng ký node (sau khi client login)
```bash
docker exec headscale headscale nodes register --key <NODE_KEY> --user <username>
```

### Liệt kê nodes
```bash
docker exec headscale headscale nodes list
```

### Xóa node
```bash
docker exec headscale headscale nodes delete --identifier <node_id>
```

### Tạo pre-auth key (để client tự đăng ký, không cần approve thủ công)
```bash
docker exec headscale headscale preauthkeys create --user <username> --reusable --expiration 24h
```

Client dùng pre-auth key:
```bash
tailscale up --login-server https://vpn.softs.business --authkey <PREAUTH_KEY>
```

---

## API Reference cho Web Admin

Headscale có REST API đầy đủ. Web Admin app sẽ dùng các endpoint này:

### Authentication
Mọi request cần header:
```
Authorization: Bearer <API_KEY>
```

### Endpoints chính

| Method | Endpoint | Mô tả |
|--------|----------|-------|
| `GET` | `/api/v1/user` | Danh sách users |
| `POST` | `/api/v1/user` | Tạo user mới |
| `DELETE` | `/api/v1/user/{name}` | Xóa user |
| `GET` | `/api/v1/node` | Danh sách tất cả nodes |
| `GET` | `/api/v1/node/{nodeId}` | Chi tiết 1 node |
| `DELETE` | `/api/v1/node/{nodeId}` | Xóa node |
| `POST` | `/api/v1/node/{nodeId}/expire` | Expire node |
| `POST` | `/api/v1/node/register` | Đăng ký node mới |
| `GET` | `/api/v1/node/{nodeId}/routes` | Routes của node |
| `POST` | `/api/v1/routes/{routeId}/enable` | Bật route |
| `POST` | `/api/v1/routes/{routeId}/disable` | Tắt route |
| `GET` | `/api/v1/preauthkey` | Danh sách pre-auth keys |
| `POST` | `/api/v1/preauthkey` | Tạo pre-auth key |
| `POST` | `/api/v1/preauthkey/expire` | Expire key |
| `GET` | `/api/v1/apikey` | Danh sách API keys |
| `POST` | `/api/v1/apikey` | Tạo API key |

### Ví dụ: Lấy danh sách nodes
```bash
curl -s -H "Authorization: Bearer <API_KEY>" \
  https://vpn.softs.business/api/v1/node | jq
```

Response:
```json
{
  "nodes": [
    {
      "id": "1",
      "machineKey": "mkey:...",
      "nodeKey": "nodekey:...",
      "name": "thinkpad",
      "user": { "id": "1", "name": "huanld" },
      "ipAddresses": ["100.64.0.1", "fd7a:115c:a1e0::1"],
      "online": true,
      "lastSeen": "2026-04-10T16:52:30Z",
      "createdAt": "2026-04-10T16:55:04Z",
      "registerMethod": "REGISTER_METHOD_CLI"
    }
  ]
}
```

### Ví dụ: Approve node mới (thay vì chạy lệnh thủ công)
```bash
# Lấy danh sách nodes chờ approve
curl -s -H "Authorization: Bearer <API_KEY>" \
  https://vpn.softs.business/api/v1/node

# Register node bằng key
curl -X POST -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"key": "<NODE_KEY>", "user": "huanld"}' \
  https://vpn.softs.business/api/v1/node/register
```

### Web Admin cần hiển thị:
1. **Dashboard**: Tổng nodes, online/offline, users
2. **Nodes list**: Tên, IP, user, trạng thái online, last seen
3. **Approve**: Nút approve cho nodes mới (gọi register API)
4. **Users**: CRUD users
5. **Pre-auth Keys**: Tạo/quản lý keys để client tự đăng ký
