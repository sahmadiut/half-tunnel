# Deployment Guide

This guide covers deploying Half-Tunnel in various environments.

## Quick Start

### Using the Installer Script

The easiest way to install Half-Tunnel on Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/sahmadiut/half-tunnel/main/scripts/install.sh | bash
```

#### Installer Options

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `HALFTUNNEL_VERSION` | Specific version to install | Latest |
| `HALFTUNNEL_INSTALL_DIR` | Installation directory | `/usr/local/bin` |
| `HALFTUNNEL_NO_SUDO` | Set to `1` to skip sudo | `0` |

Example with options:
```bash
HALFTUNNEL_VERSION=v1.0.0 HALFTUNNEL_INSTALL_DIR=$HOME/.local/bin curl -fsSL https://raw.githubusercontent.com/sahmadiut/half-tunnel/main/scripts/install.sh | bash
```

### Manual Installation

1. Download the latest release from [GitHub Releases](https://github.com/sahmadiut/half-tunnel/releases)

2. Extract and install:
```bash
tar -xzf half-tunnel-vX.X.X-linux-amd64.tar.gz
sudo mv ht-client ht-server half-tunnel /usr/local/bin/
```

3. Verify installation:
```bash
half-tunnel --version
```

## Docker Deployment

### Using Docker Compose

1. Clone the repository:
```bash
git clone https://github.com/sahmadiut/half-tunnel.git
cd half-tunnel
```

2. Start the services:
```bash
cd deployments
docker-compose up -d
```

3. Check logs:
```bash
docker-compose logs -f
```

### Using Individual Containers

#### Server

```bash
docker run -d \
  --name half-tunnel-server \
  -p 8080:8080 \
  -p 8081:8081 \
  -v $(pwd)/server.yml:/app/config.yaml \
  ghcr.io/sahmadiut/half-tunnel-server:latest
```

#### Client

```bash
docker run -d \
  --name half-tunnel-client \
  -p 1080:1080 \
  -v $(pwd)/client.yml:/app/config.yaml \
  ghcr.io/sahmadiut/half-tunnel-client:latest
```

### Building Custom Images

```bash
# Build client image
docker build -t my-ht-client:latest -f deployments/Dockerfile.client .

# Build server image
docker build -t my-ht-server:latest -f deployments/Dockerfile.server .
```

## Systemd Service

Install services using the CLI. Client and server are installed separately, so you can deploy only what you need on each host.

### Server Service

```bash
sudo mkdir -p /etc/half-tunnel
sudo cp server.yml /etc/half-tunnel/server.yml
sudo /usr/local/bin/half-tunnel service install --type server
sudo systemctl enable half-tunnel-server
sudo systemctl start half-tunnel-server
sudo systemctl status half-tunnel-server
```

### Client Service

```bash
sudo mkdir -p /etc/half-tunnel
sudo cp client.yml /etc/half-tunnel/client.yml
sudo /usr/local/bin/half-tunnel service install --type client
sudo systemctl enable half-tunnel-client
sudo systemctl start half-tunnel-client
sudo systemctl status half-tunnel-client
```

## Production Deployment

### Architecture Overview

For maximum traffic analysis resistance, deploy upstream and downstream on separate infrastructure:

```
┌─────────────┐                                   ┌─────────────┐
│   Client    │                                   │  Internet   │
└──────┬──────┘                                   └──────▲──────┘
       │                                                 │
       │ Upstream (Domain A)      Downstream (Domain B)  │
       ▼                          ▼                      │
┌──────────────┐            ┌──────────────┐      ┌──────┴──────┐
│  Server A    │◀──────────▶│  Server B    │      │   Target    │
│  (Upstream)  │            │ (Downstream) │      │   Server    │
└──────────────┘            └──────────────┘      └─────────────┘
```

### TLS Configuration

Always use TLS in production:

#### Server Configuration

```yaml
server:
  upstream:
    host: "0.0.0.0"
    port: 8443
    tls:
      enabled: true
      cert_file: "/etc/half-tunnel/certs/server.crt"
      key_file: "/etc/half-tunnel/certs/server.key"
  downstream:
    host: "0.0.0.0"
    port: 8444
    tls:
      enabled: true
      cert_file: "/etc/half-tunnel/certs/server.crt"
      key_file: "/etc/half-tunnel/certs/server.key"
```

#### Generate Self-Signed Certificates (for testing)

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout server.key -out server.crt \
  -subj "/CN=half-tunnel"
```

### Firewall Configuration

```bash
# Allow upstream port
sudo ufw allow 8443/tcp

# Allow downstream port  
sudo ufw allow 8444/tcp

# For client: allow SOCKS5 from localhost only
sudo ufw allow from 127.0.0.1 to any port 1080
```

### Monitoring

#### Prometheus Metrics

Both client and server expose Prometheus metrics:

```yaml
observability:
  metrics:
    enabled: true
    port: 9090
    path: "/metrics"
```

Access metrics at `http://localhost:9090/metrics`

#### Health Checks

```yaml
observability:
  health:
    enabled: true
    port: 8080
    path: "/healthz"
```

Check health: `curl http://localhost:8080/healthz`

### Logging

Configure structured logging for production:

```yaml
logging:
  level: "info"           # Use "debug" for troubleshooting
  format: "json"          # Machine-parseable format
  output: "/var/log/half-tunnel/server.log"
```

#### Log Rotation

Create `/etc/logrotate.d/half-tunnel`:

```
/var/log/half-tunnel/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 root root
    postrotate
        systemctl reload half-tunnel-server 2>/dev/null || true
        systemctl reload half-tunnel-client 2>/dev/null || true
    endscript
}
```

## Troubleshooting

### Connection Issues

1. Check server is running:
```bash
curl http://server-ip:8080/healthz
```

2. Verify WebSocket connectivity:
```bash
wscat -c ws://server-ip:8080/upstream
```

3. Check firewall rules:
```bash
sudo ufw status
sudo iptables -L -n
```

### Performance Tuning

Increase file descriptor limits in `/etc/security/limits.conf`:

```
*    soft    nofile    65535
*    hard    nofile    65535
```

Kernel tuning in `/etc/sysctl.conf`:

```
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_keepalive_probes = 5
net.ipv4.tcp_keepalive_intvl = 15
```

Apply changes:
```bash
sudo sysctl -p
```
