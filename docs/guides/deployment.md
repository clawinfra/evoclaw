# Production Deployment Guide

This guide covers deploying EvoClaw in production environments.

## Architecture Options

### Single Machine

For small deployments (1-5 agents):

```
┌──────────────────────────────────┐
│  Single Server / Raspberry Pi    │
│  ┌───────────────────────────┐   │
│  │  Orchestrator (:8420)     │   │
│  │  + Mosquitto (:1883)      │   │
│  │  + Edge Agent(s)          │   │
│  └───────────────────────────┘   │
└──────────────────────────────────┘
```

### Distributed

For larger deployments:

```
┌─────────────────┐    ┌─────────────────┐
│  Cloud Server    │    │  Edge Devices    │
│  Orchestrator    │◄──►│  Agent 1 (Pi)    │
│  MQTT Broker     │    │  Agent 2 (Phone) │
│  Dashboard       │    │  Agent 3 (IoT)   │
└─────────────────┘    └─────────────────┘
```

## Docker Compose (Recommended)

```bash
# Start the full stack
docker compose up -d

# View logs
docker compose logs -f orchestrator

# Scale agents
docker compose up -d --scale edge-agent=3
```

### Production docker-compose.yml

```yaml
version: '3.8'

services:
  mosquitto:
    image: eclipse-mosquitto:2
    ports:
      - "1883:1883"
    volumes:
      - mosquitto-data:/mosquitto/data
      - ./mosquitto.conf:/mosquitto/config/mosquitto.conf
    restart: unless-stopped

  orchestrator:
    build:
      context: .
      dockerfile: Dockerfile.orchestrator
    ports:
      - "8420:8420"
    volumes:
      - evoclaw-data:/app/data
      - ./evoclaw.json:/app/evoclaw.json:ro
    depends_on:
      - mosquitto
    restart: unless-stopped
    environment:
      - EVOCLAW_LOG_LEVEL=info

  edge-agent:
    build:
      context: .
      dockerfile: Dockerfile.agent
    volumes:
      - ./edge-agent/agent.toml:/app/agent.toml:ro
      - ./keys:/app/keys:ro
    depends_on:
      - mosquitto
      - orchestrator
    restart: unless-stopped

volumes:
  mosquitto-data:
  evoclaw-data:
```

## Systemd Service

For bare-metal deployments:

### Orchestrator Service

```ini
# /etc/systemd/system/evoclaw.service
[Unit]
Description=EvoClaw Orchestrator
After=network.target mosquitto.service
Wants=mosquitto.service

[Service]
Type=simple
User=evoclaw
Group=evoclaw
WorkingDirectory=/opt/evoclaw
ExecStart=/opt/evoclaw/evoclaw --config /opt/evoclaw/evoclaw.json
Restart=always
RestartSec=5
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/evoclaw/data

[Install]
WantedBy=multi-user.target
```

### Edge Agent Service

```ini
# /etc/systemd/system/evoclaw-agent.service
[Unit]
Description=EvoClaw Edge Agent
After=network.target

[Service]
Type=simple
User=evoclaw
WorkingDirectory=/opt/evoclaw-agent
ExecStart=/opt/evoclaw-agent/evoclaw-agent --config /opt/evoclaw-agent/agent.toml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Setup

```bash
# Create user
sudo useradd -r -s /bin/false evoclaw

# Install
sudo mkdir -p /opt/evoclaw
sudo cp evoclaw /opt/evoclaw/
sudo cp evoclaw.json /opt/evoclaw/
sudo chown -R evoclaw:evoclaw /opt/evoclaw

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable evoclaw
sudo systemctl start evoclaw

# Check status
sudo systemctl status evoclaw
journalctl -u evoclaw -f
```

## Reverse Proxy (nginx)

For HTTPS and domain name:

```nginx
server {
    listen 443 ssl http2;
    server_name evoclaw.example.com;

    ssl_certificate /etc/letsencrypt/live/evoclaw.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/evoclaw.example.com/privkey.pem;

    # Dashboard and API
    location / {
        proxy_pass http://127.0.0.1:8420;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # SSE log streaming
    location /api/logs/stream {
        proxy_pass http://127.0.0.1:8420;
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
        proxy_buffering off;
        proxy_cache off;
    }
}
```

## Security Checklist

- [ ] **Bind to localhost** — Don't expose API to public internet without auth
- [ ] **Use TLS** — HTTPS for API, TLS for MQTT
- [ ] **Protect secrets** — API keys in environment variables, not config files
- [ ] **Restrict MQTT** — Username/password authentication
- [ ] **Firewall** — Only expose necessary ports (8420, 1883)
- [ ] **Updates** — Keep Go, Rust, and dependencies up to date
- [ ] **Backups** — Back up `data/` directory regularly
- [ ] **Monitoring** — Set up health checks and alerting

## Monitoring

### Health Check

```bash
# Simple health check
curl -f http://localhost:8420/api/status || echo "UNHEALTHY"
```

### Prometheus Metrics (Planned)

Future versions will expose `/metrics` in Prometheus format.

### Log Aggregation

Forward logs to your logging stack:

```bash
# journald → stdout → your log aggregator
journalctl -u evoclaw -f --output=json | your-log-shipper
```

## Backup & Restore

### Backup

```bash
# Backup all state
tar czf evoclaw-backup-$(date +%Y%m%d).tar.gz /opt/evoclaw/data/
```

### Restore

```bash
# Stop service
sudo systemctl stop evoclaw

# Restore
tar xzf evoclaw-backup-20260206.tar.gz -C /

# Start service
sudo systemctl start evoclaw
```

## Resource Requirements

| Component | CPU | RAM | Disk | Notes |
|-----------|-----|-----|------|-------|
| Orchestrator | 1 core | 128MB | 100MB | Lightweight Go binary |
| Edge Agent | 0.5 core | 32MB | 10MB | Minimal Rust binary |
| MQTT Broker | 0.5 core | 64MB | 50MB | Mosquitto |
| Dashboard | — | — | — | Served from orchestrator |

## See Also

- [Installation](../getting-started/installation.md)
- [Configuration](../getting-started/configuration.md)
- [Architecture](../architecture/overview.md)
