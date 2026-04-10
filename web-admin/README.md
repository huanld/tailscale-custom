# Headscale Web Admin

Web-based admin panel for managing Headscale VPN nodes, users, and pre-auth keys.

## Features
- **Dashboard**: Total nodes, online/offline count
- **Nodes**: List, delete, expire nodes
- **Users**: Create, delete users
- **Pre-auth Keys**: Create reusable/ephemeral keys for auto-registration
- **Auto-refresh**: Node status updates every 30s
- **Simple auth**: Password-protected admin panel

## Quick Start

```bash
# 1. Copy env file
cp .env.example .env

# 2. Edit .env with your Headscale API key
#    Generate key: docker exec headscale headscale apikeys create --expiration 365d

# 3. Start
docker compose up -d
```

Admin panel: `http://your-server:9080`

## Standalone (without Headscale in same compose)

```bash
docker build -t headscale-admin .
docker run -d --name headscale-admin \
  -p 9080:9080 \
  -e HEADSCALE_URL=http://your-headscale:8080 \
  -e HEADSCALE_API_KEY=your_key \
  -e ADMIN_PASSWORD=your_password \
  headscale-admin
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `HEADSCALE_URL` | Yes | `http://localhost:8080` | Headscale API URL |
| `HEADSCALE_API_KEY` | Yes | - | Headscale API key |
| `ADMIN_PASSWORD` | No | - | Web UI password (empty = no auth) |
| `LISTEN_ADDR` | No | `:9080` | Listen address |
