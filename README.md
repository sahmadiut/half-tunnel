# Half-Tunnel

A production-grade Go tunneling system that obscures traffic analysis by splitting upstream (Domain A) and downstream (Domain B) paths over separate WebSocket connections.

## Overview

Half-Tunnel uses UUID-based session correlation and multiplexing to reassemble bidirectional traffic on the exit server. This architecture provides enhanced privacy by separating request and response paths across different domains.


```
┌──────┐   ┌─────────────┐     Upstream (Domain A)       ┌─────────────┐     Outbound
│ User │──▶│   Client    │ ─────────────────────────────▶│   Server    │ ─────────────▶ Internet
└──────┘   │  (Entry)    │                               │   (Exit)    │
		   └─────────────┘ ◀─────────────────────────────└─────────────┘
			 ▲               Downstream (Domain B)           ▲
			 └───────────────────────────────────────────────┘
```

## Features

- **Split-Path Architecture**: Separates upstream and downstream traffic across different domains
- **UUID-Based Sessions**: Cryptographically random session IDs for correlation
- **Stream Multiplexing**: Multiple logical connections within a single session
- **Binary Protocol**: Efficient wire format with optional HMAC authentication
- **Reconnection Support**: Automatic reconnection with exponential backoff
- **SOCKS5 Proxy**: Local SOCKS5 interface for easy client integration

## Quick Start

### Installation

#### Quick Install (Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/sahmadiut/half-tunnel/main/scripts/install.sh | bash
```

#### Manual Installation

```bash
# Clone the repository
git clone https://github.com/sahmadiut/half-tunnel.git
cd half-tunnel

# Build binaries
make build

# Or install with Go
go install ./cmd/client
go install ./cmd/server
```

### Running the Server

```bash
# Using config file
./bin/ht-server -config configs/config.example.yaml

# Or with environment variables
HT_SERVER_UPSTREAM_ADDR=:8080 HT_SERVER_DOWNSTREAM_ADDR=:8081 ./bin/ht-server
```

### Running the Client

```bash
# Using config file
./bin/ht-client -config configs/config.example.yaml

# Or with environment variables
HT_CLIENT_UPSTREAM_URL=ws://localhost:8080/upstream \
HT_CLIENT_DOWNSTREAM_URL=ws://localhost:8081/downstream \
./bin/ht-client
```

### Using with SOCKS5

Configure your application to use the SOCKS5 proxy at `127.0.0.1:1080`:

```bash
# Example with curl
curl --socks5 127.0.0.1:1080 https://example.com

# Example with proxychains
proxychains4 curl https://example.com
```

## Configuration

Configuration can be provided via:
1. YAML config file (`-config` flag)
2. Environment variables (prefix: `HT_`)
3. Default values

See [configs/config.example.yaml](configs/config.example.yaml) for all available options.

## Project Structure

```
half-tunnel/
├── cmd/
│   ├── client/          # Entry client binary
│   ├── server/          # Exit server binary
│   ├── ht/              # Service manager CLI
│   └── half-tunnel/     # CLI tool
├── internal/
│   ├── protocol/        # Packet format, serialization
│   ├── transport/       # WebSocket managers
│   ├── session/         # UUID-based session tracking
│   ├── mux/             # Multiplexer for logical connections
│   ├── service/         # Systemd service management
│   └── config/          # Configuration loading
├── pkg/
│   ├── crypto/          # Encryption utilities
│   └── logger/          # Structured logging wrapper
├── configs/             # Sample configurations
├── deployments/         # Docker files
├── scripts/             # Build and install scripts
├── test/                # Integration and E2E tests
└── docs/                # Documentation
```

## Service Management

Half-Tunnel includes a service manager (`ht`) for easy systemd service management:

### Quick Commands

```bash
# Client service
ht c install --config /etc/half-tunnel/client.yml   # Install client service
ht c start                                           # Start client
ht c stop                                            # Stop client
ht c restart                                         # Restart client
ht c logs                                            # View logs (follow mode)
ht c logs -n 50 --no-follow                         # View last 50 lines

# Server service  
ht s install --config /etc/half-tunnel/server.yml   # Install server service
ht s start                                           # Start server
ht s stop                                            # Stop server
ht s restart                                         # Restart server
ht s logs                                            # View logs (follow mode)

# Enable auto-start on boot
ht c enable
ht s enable
```

### Hot Reload

Both client and server support hot reload of configuration files:

```bash
# Enable hot reload when starting
ht-client -config /etc/half-tunnel/client.yml -hot-reload
ht-server -config /etc/half-tunnel/server.yml -hot-reload

# Or send SIGHUP to reload config
kill -HUP $(pgrep ht-client)
kill -HUP $(pgrep ht-server)
```

When running as a systemd service, the service will automatically restart with the new configuration.

## Documentation

- [Protocol Specification](docs/PROTOCOL.md) - Wire format and protocol details
- [Deployment Guide](docs/DEPLOYMENT.md) - Production deployment instructions
- [Contributing](CONTRIBUTING.md) - Contribution guidelines
- [Security Policy](SECURITY.md) - Security information and reporting

## Protocol

The Half-Tunnel protocol uses a binary packet format:

```
┌─────────────────────────────────────────────────────────────┐
│ Magic (2B) │ Version (1B) │ Flags (1B) │ SessionID (16B)    │
├─────────────────────────────────────────────────────────────┤
│ StreamID (4B) │ SeqNum (4B) │ AckNum (4B) │ PayloadLen (2B) │
├─────────────────────────────────────────────────────────────┤
│ Payload (0-65535 bytes)                                     │
├─────────────────────────────────────────────────────────────┤
│ HMAC (32B, optional)                                        │
└─────────────────────────────────────────────────────────────┘
```

See [docs/PROTOCOL.md](docs/PROTOCOL.md) for the complete protocol specification.

## Development

### Prerequisites

- Go 1.21+
- golangci-lint (for linting)
- Docker (optional, for containerized deployment)

### Building

```bash
# Build all
make build

# Run tests
make test

# Run linter
make lint

# Build Docker images
make docker
```

### Testing

```bash
# Run unit tests
make test

# Run with coverage
make test-coverage
```

## Security Considerations

- Always use TLS in production (`tls.enabled: true`)
- Use strong, unique session keys
- Consider enabling HMAC authentication for packet integrity
- Deploy upstream and downstream servers on separate infrastructure for maximum traffic analysis resistance

## License

MIT License - see [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
