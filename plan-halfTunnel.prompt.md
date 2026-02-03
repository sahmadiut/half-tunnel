# Plan: Half-Tunnel Split-Path VPN System

A production-grade Go tunneling system that obscures traffic analysis by splitting upstream (Domain A) and downstream (Domain B) paths over separate WebSocket connections. The architecture uses UUID-based session correlation and multiplexing to reassemble bidirectional traffic on the exit server.

---

## Phase 1: Project Foundation & Structure

**Objective:** Establish a standard Go project layout with core configuration and interfaces.

**Directory Structure:**
```
half-tunnel/
├── cmd/
│   ├── client/          # Entry client binary
│   │   └── main.go
│   └── server/          # Exit server binary
│       └── main.go
├── internal/
│   ├── protocol/        # Packet format, serialization
│   ├── transport/       # WebSocket managers
│   ├── session/         # UUID-based session tracking
│   ├── mux/             # Multiplexer for logical connections
│   └── config/          # Configuration loading
├── pkg/
│   ├── crypto/          # Encryption utilities (reusable)
│   └── logger/          # Structured logging wrapper
├── api/                 # Protobuf or API definitions
├── configs/             # Sample YAML/TOML configs
├── scripts/             # Build/deploy helper scripts
├── deployments/         # Dockerfiles, k8s manifests
├── test/                # Integration/e2e test fixtures
├── docs/                # Architecture diagrams, protocol spec
├── .github/workflows/   # CI/CD pipelines
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

**Steps:**
1. Initialize Go module (`go mod init github.com/<org>/half-tunnel`)
2. Create directory skeleton with placeholder files
3. Define core interfaces: `Packet`, `Session`, `Transport`, `Multiplexer`
4. Implement config loader (Viper or Koanf) for YAML-based settings
5. Set up structured logger (zerolog or zap)
6. Create `Makefile` with targets: `build`, `test`, `lint`, `docker`

---

## Phase 1.5: Configuration Files (YAML)

**Objective:** Define YAML configuration files for both client and server. Port mappings are defined only on the client side - the server acts as a transparent proxy for any destination requested by authenticated clients.

### Server Configuration (`configs/server.yml`)

```yaml
# Half-Tunnel Server Configuration
server:
  # Server identification
  name: "exit-server-01"
  
  # Upstream listener (Domain A) - receives client requests
  upstream:
    host: "0.0.0.0"
    port: 8443
    path: "/ws/upstream"
    tls:
      enabled: true
      cert_file: "/etc/half-tunnel/certs/server.crt"
      key_file: "/etc/half-tunnel/certs/server.key"
  
  # Downstream listener (Domain B) - sends responses to client
  downstream:
    host: "0.0.0.0"
    port: 8444
    path: "/ws/downstream"
    tls:
      enabled: true
      cert_file: "/etc/half-tunnel/certs/server.crt"
      key_file: "/etc/half-tunnel/certs/server.key"

# Access control (server doesn't define ports - client requests any destination)
access:
  # Allowed destination networks (empty = allow all)
  allowed_networks:
    - "0.0.0.0/0"           # Allow all IPv4
    - "::/0"                # Allow all IPv6
  # Blocked destinations (takes priority over allowed)
  blocked_networks:
    - "10.0.0.0/8"          # Block private networks (optional)
    - "172.16.0.0/12"
    - "192.168.0.0/16"
  # Max connections per session
  max_streams_per_session: 100

# Tunnel settings
tunnel:
  # Session management
  session:
    timeout: "5m"           # Idle session timeout
    max_sessions: 1000      # Maximum concurrent sessions
    
  # Connection settings
  connection:
    read_buffer_size: 32768
    write_buffer_size: 32768
    keepalive_interval: "30s"
    max_message_size: 65536
    
  # Encryption
  encryption:
    enabled: true
    algorithm: "aes-256-gcm"  # Options: aes-256-gcm, chacha20-poly1305

# Logging
logging:
  level: "info"             # debug, info, warn, error
  format: "json"            # json, text
  output: "/var/log/half-tunnel/server.log"

# Metrics & Health
observability:
  metrics:
    enabled: true
    port: 9090
    path: "/metrics"
  health:
    enabled: true
    port: 8080
    path: "/healthz"
```

### Client Configuration (`configs/client.yml`)

```yaml
# Half-Tunnel Client Configuration
client:
  # Client identification
  name: "entry-client-01"
  
  # Upstream connection (Domain A) - sends requests to server
  upstream:
    url: "wss://domain-a.example.com:8443/ws/upstream"
    tls:
      enabled: true
      skip_verify: false
      ca_file: "/etc/half-tunnel/certs/ca.crt"
      
  # Downstream connection (Domain B) - receives responses from server
  downstream:
    url: "wss://domain-b.example.com:8444/ws/downstream"
    tls:
      enabled: true
      skip_verify: false
      ca_file: "/etc/half-tunnel/certs/ca.crt"

# Port forwarding rules (all port definitions are on client side)
# Client tells server which destination to connect to
port_forwards:
  - name: "web-proxy"
    listen_host: "127.0.0.1"    # Local address to listen on
    listen_port: 8080            # Local port to listen on
    remote_host: "example.com"   # Destination host (server connects to this)
    remote_port: 80              # Destination port
    protocol: "tcp"
    
  - name: "https-proxy"
    listen_host: "127.0.0.1"
    listen_port: 8443
    remote_host: "secure.example.com"
    remote_port: 443
    protocol: "tcp"
    
  - name: "ssh-tunnel"
    listen_host: "127.0.0.1"
    listen_port: 2222
    remote_host: "ssh.internal.company.com"
    remote_port: 22
    protocol: "tcp"
    
  - name: "database"
    listen_host: "127.0.0.1"
    listen_port: 5432
    remote_host: "db.internal.company.com"
    remote_port: 5432
    protocol: "tcp"

# SOCKS5 Proxy (for dynamic port forwarding - any destination)
socks5:
  enabled: true
  listen_host: "127.0.0.1"
  listen_port: 1080
  auth:
    enabled: false
    username: ""
    password: ""

# Tunnel settings
tunnel:
  # Reconnection strategy
  reconnect:
    enabled: true
    initial_delay: "1s"
    max_delay: "60s"
    multiplier: 2.0
    jitter: 0.1
    
  # Connection settings
  connection:
    read_buffer_size: 32768
    write_buffer_size: 32768
    keepalive_interval: "30s"
    dial_timeout: "10s"
    
  # Encryption (must match server)
  encryption:
    enabled: true
    algorithm: "aes-256-gcm"

# DNS settings (for full VPN mode)
dns:
  enabled: false
  listen_host: "127.0.0.1"
  listen_port: 5353
  upstream_servers:
    - "8.8.8.8:53"
    - "1.1.1.1:53"

# Logging
logging:
  level: "info"
  format: "json"
  output: "/var/log/half-tunnel/client.log"

# Local metrics
observability:
  metrics:
    enabled: true
    port: 9091
    path: "/metrics"
```

---

### Configuration Generator (CLI Commands)

**Objective:** The application should be able to generate configuration files for users interactively or via CLI flags.

**CLI Commands:**

```bash
# Generate server config interactively
half-tunnel config generate --type server --output server.yml

# Generate client config interactively
half-tunnel config generate --type client --output client.yml

# Generate client config with flags (non-interactive)
half-tunnel config generate --type client \
  --upstream-url "wss://domain-a.example.com:8443/ws/upstream" \
  --downstream-url "wss://domain-b.example.com:8444/ws/downstream" \
  --port-forward "8080:example.com:80" \
  --port-forward "2222:ssh.server.com:22" \
  --socks5-port 1080 \
  --output client.yml

# Generate server config with flags
half-tunnel config generate --type server \
  --upstream-port 8443 \
  --downstream-port 8444 \
  --tls-cert /path/to/cert.pem \
  --tls-key /path/to/key.pem \
  --output server.yml

# Validate existing config
half-tunnel config validate --config client.yml

# Show sample config
half-tunnel config sample --type client
half-tunnel config sample --type server
```

**Interactive Mode Example:**
```
$ half-tunnel config generate --type client

🔧 Half-Tunnel Client Configuration Generator

? Enter a name for this client: my-laptop
? Upstream server URL: wss://up.example.com:8443/ws/upstream
? Downstream server URL: wss://down.example.com:8444/ws/downstream
? Enable TLS verification? Yes
? CA certificate path (optional): 

📡 Port Forwarding Rules
? Add a port forward? Yes
? Local listen address [127.0.0.1]: 
? Local port: 8080
? Remote host: api.internal.com
? Remote port: 80
? Add another port forward? Yes
? Local listen address [127.0.0.1]: 
? Local port: 3306
? Remote host: db.internal.com
? Remote port: 3306
? Add another port forward? No

🧦 SOCKS5 Proxy
? Enable SOCKS5 proxy? Yes
? SOCKS5 listen port [1080]: 

✅ Configuration saved to: client.yml
```

**Implementation (`internal/config/generator.go`):**

```go
// ConfigGenerator handles interactive and flag-based config generation
type ConfigGenerator struct {
    reader   io.Reader
    writer   io.Writer
    isInteractive bool
}

// GenerateClientConfig creates a new client configuration
func (g *ConfigGenerator) GenerateClientConfig(opts GenerateOptions) (*ClientConfig, error)

// GenerateServerConfig creates a new server configuration  
func (g *ConfigGenerator) GenerateServerConfig(opts GenerateOptions) (*ServerConfig, error)

// ParsePortForward parses "localPort:remoteHost:remotePort" format
func ParsePortForward(spec string) (*PortForward, error)

// ValidateConfig validates a config file
func ValidateConfig(path string, configType string) error
```

**Steps for Config Generator:**
1. Create `cmd/half-tunnel/config.go` with config subcommands
2. Implement `internal/config/generator.go` for config generation logic
3. Use `survey` or `promptui` library for interactive prompts
4. Support both interactive and non-interactive (flag-based) modes
5. Implement config validation with detailed error messages
6. Add `config sample` command to print example configs

---

### Configuration Loading (`internal/config/`)

**Config Structures:**
```go
// ServerConfig represents server configuration
type ServerConfig struct {
    Server        ServerSettings   `yaml:"server"`
    Access        AccessConfig     `yaml:"access"`
    Tunnel        TunnelConfig     `yaml:"tunnel"`
    Logging       LogConfig        `yaml:"logging"`
    Observability ObservConfig     `yaml:"observability"`
}

// AccessConfig defines server-side access control (no port definitions)
type AccessConfig struct {
    AllowedNetworks      []string `yaml:"allowed_networks"`
    BlockedNetworks      []string `yaml:"blocked_networks"`
    MaxStreamsPerSession int      `yaml:"max_streams_per_session"`
}

// ClientConfig represents client configuration
type ClientConfig struct {
    Client        ClientSettings   `yaml:"client"`
    PortForwards  []PortForward    `yaml:"port_forwards"`
    SOCKS5        SOCKS5Config     `yaml:"socks5"`
    Tunnel        TunnelConfig     `yaml:"tunnel"`
    DNS           DNSConfig        `yaml:"dns"`
    Logging       LogConfig        `yaml:"logging"`
    Observability ObservConfig     `yaml:"observability"`
}

// PortForward defines client-side port forwarding (client decides destination)
type PortForward struct {
    Name       string `yaml:"name"`
    ListenHost string `yaml:"listen_host"`
    ListenPort int    `yaml:"listen_port"`
    RemoteHost string `yaml:"remote_host"`   // Destination host (server connects to)
    RemotePort int    `yaml:"remote_port"`   // Destination port
    Protocol   string `yaml:"protocol"`
}
```

**Steps:**
1. Create `internal/config/server.go` with `LoadServerConfig()` function
2. Create `internal/config/client.go` with `LoadClientConfig()` function
3. Implement validation logic for required fields and port ranges
4. Support config file path via CLI flag (`-c` / `--config`)
5. Support environment variable overrides (e.g., `HALFTUNNEL_SERVER_UPSTREAM_PORT`)
6. Create sample configs in `configs/` directory

---

## Phase 2: Protocol Design & Packet Encapsulation

**Objective:** Define the wire format for split-path communication with UUID correlation.

**Packet Structure (binary format):**
```
┌─────────────────────────────────────────────────────────────┐
│ Magic (2B) │ Version (1B) │ Flags (1B) │ SessionID (16B UUID)│
├─────────────────────────────────────────────────────────────┤
│ StreamID (4B) │ SeqNum (4B) │ AckNum (4B) │ PayloadLen (2B) │
├─────────────────────────────────────────────────────────────┤
│ Payload (0-65535 bytes, encrypted)                          │
├─────────────────────────────────────────────────────────────┤
│ HMAC (32B, optional based on flags)                         │
└─────────────────────────────────────────────────────────────┘
```

**Key Design Decisions:**
- `SessionID`: UUID v4 correlates upstream/downstream paths for a single client
- `StreamID`: Multiplexes logical connections (TCP streams) within one session
- `SeqNum/AckNum`: Enables ordering and optional reliable delivery over unreliable split paths
- Flags: Control packet type (DATA, ACK, FIN, KEEPALIVE, HANDSHAKE)

**Steps:**
1. Define `internal/protocol/packet.go` with `Packet` struct and `Marshal/Unmarshal`
2. Implement protobuf schema (optional) in `api/` for cross-language interop
3. Design handshake: client sends HANDSHAKE upstream → server responds downstream with session confirmation
4. Define encryption layer: NaCl box or AES-GCM with per-session keys
5. Document protocol spec in `docs/PROTOCOL.md`

---

## Phase 3: Core Component Development

**Objective:** Implement Entry Client and Exit Server with split-path transport.

### 3A: Transport Layer (`internal/transport/`)

**Steps:**
1. Implement `UpstreamConn`: WebSocket client dial to Domain A, write-only
2. Implement `DownstreamConn`: WebSocket client dial to Domain B (or server listener), read-only
3. Create `TransportManager` interface unifying both paths
4. Add TLS configuration with custom CA pinning options
5. Implement connection health monitoring (ping/pong, deadlines)

### 3B: Session & Multiplexer (`internal/session/`, `internal/mux/`)

**Steps:**
1. `SessionStore`: Thread-safe map of `SessionID → Session` with TTL eviction
2. `Session`: Holds both upstream/downstream state, per-stream buffers
3. `Multiplexer`: Routes incoming packets to correct stream, creates new streams on demand
4. Implement stream lifecycle: OPEN → ACTIVE → HALF_CLOSED → CLOSED
5. Use ring buffers for out-of-order packet reassembly (bounded memory)

### 3C: Entry Client (`cmd/client/`)

**Steps:**
1. SOCKS5 proxy listener (simpler) OR TUN device (advanced) for traffic capture
2. On new TCP connection → allocate `StreamID`, create upstream packets
3. Send packets via `UpstreamConn` to Domain A
4. Receive responses from `DownstreamConn` (Domain B) → demux → deliver to local socket
5. Handle DNS interception (optional, for full VPN mode)

### 3D: Exit Server (`cmd/server/`)

**Steps:**
1. WebSocket listener on Domain A (upstream receiver)
2. WebSocket listener on Domain B (downstream sender)
3. On upstream packet: lookup/create session → forward payload to target destination
4. On response from destination: encapsulate → send via Domain B downstream
5. Implement NAT table for tracking outbound → inbound mapping

---

## Phase 4: Resilience & Production Hardening

**Objective:** Handle failures gracefully with reconnection, concurrency safety, and observability.

**Reconnection Strategy:**
- Exponential backoff with jitter (1s → 2s → 4s → ... max 60s)
- Session persistence: client caches `SessionID`, resumes on reconnect
- Server-side session timeout: 5-minute idle before eviction
- Implement RECONNECT packet type to restore state

**Concurrency Patterns:**
1. One goroutine per WebSocket read loop (upstream/downstream)
2. Channel-based packet dispatch to multiplexer
3. `sync.RWMutex` on session store, per-stream mutexes for buffers
4. Use `context.Context` for cancellation propagation
5. Worker pool for destination dialing (limit concurrent outbound connections)

**Error Handling:**
- Define error types: `ErrSessionExpired`, `ErrStreamClosed`, `ErrUpstreamUnavailable`
- Graceful shutdown: drain queues, send FIN packets, wait with timeout
- Circuit breaker for downstream domain failures

**Observability:**
1. Prometheus metrics: `packets_sent`, `packets_received`, `active_sessions`, `stream_latency_ms`
2. OpenTelemetry tracing for packet flow
3. Structured logging with correlation IDs
4. Health endpoints: `/healthz`, `/readyz`

**Steps:**
1. Implement retry wrapper in `internal/transport/`
2. Add `context` plumbing throughout codebase
3. Create `internal/metrics/` with Prometheus collectors
4. Add circuit breaker library (e.g., `sony/gobreaker`)
5. Write chaos tests: kill connections mid-stream, verify recovery

---

## Phase 5: DevOps, Testing & Delivery

**Objective:** Prepare for open-source release with CI/CD, containers, and comprehensive tests.

### 5A: Dockerization (`deployments/`)

**Steps:**
1. Multi-stage Dockerfile for client:
   - Stage 1: `golang:1.22-alpine` build
   - Stage 2: `alpine:3.19` runtime with ca-certificates
2. Same pattern for server
3. `docker-compose.yml`: client, server-upstream (Domain A), server-downstream (Domain B)
4. Add healthchecks in Compose

### 5B: GitHub Actions (`.github/workflows/`)

**Pipelines:**
1. `ci.yml`: On PR/push
   - Lint (`golangci-lint`)
   - Unit tests with coverage
   - Build binaries
   - Security scan (`govulncheck`, `trivy`)
2. `release.yml`: On tag push
   - Build multi-arch binaries (linux/amd64, linux/arm64, darwin/amd64, windows/amd64)
   - Build & push Docker images to GHCR
   - Generate changelog, create GitHub Release

### 5C: Testing Strategy

| Level       | Location          | Scope                                    |
|-------------|-------------------|------------------------------------------|
| Unit        | `*_test.go`       | Packet marshal/unmarshal, session logic  |
| Integration | `test/integration/`| Client ↔ Server over localhost WebSockets|
| E2E         | `test/e2e/`       | Full split-path with mock Domain A/B     |
| Fuzz        | `*_fuzz_test.go`  | Packet parsing, malformed input handling |

**Steps:**
1. Aim for 70%+ unit test coverage on `internal/`
2. Use `testcontainers-go` for integration tests with real WebSocket servers
3. Create E2E scenario: client sends HTTP request → exit server fetches → response returns via split path
4. Add `go test -fuzz` targets for protocol parsing

### 5D: Documentation

**Steps:**
1. `README.md`: Overview, quickstart, architecture diagram
2. `docs/PROTOCOL.md`: Wire format specification
3. `docs/DEPLOYMENT.md`: Docker, bare-metal, cloud deployment guides
4. `CONTRIBUTING.md`: Code style, PR process
5. `SECURITY.md`: Responsible disclosure policy

---

## Verification

| Checkpoint                  | Validation Method                                      |
|-----------------------------|--------------------------------------------------------|
| Project builds              | `make build` succeeds on CI                            |
| Config files valid          | `make validate-config` parses sample YAML configs      |
| Tests pass                  | `make test` with 70%+ coverage                         |
| Lint clean                  | `golangci-lint run` with no errors                     |
| Protocol correctness        | Fuzz tests + integration tests                         |
| Port mappings work          | Test each configured port forwards correctly           |
| Split-path works            | E2E test: traffic observed on both Domain A and B only |
| Reconnection works          | Chaos test: kill upstream, verify session resume       |
| Docker images functional    | `docker-compose up` → client connects successfully     |
| Release automation          | Tag push → binaries + images published                 |

---

## Decisions

- **YAML for configuration**: Human-readable, widely supported, good for port mappings and nested structures
- **Separate config files for client/server**: Clearer separation of concerns, easier to deploy and manage
- **Port definitions only on client**: Client specifies destination (remote_host:remote_port); server acts as transparent proxy. Simpler architecture, no need to sync port configs between client and server
- **Config generator CLI**: Users can generate configs interactively or via flags, reducing configuration errors
- **SOCKS5 over TUN for Phase 1**: Simpler cross-platform, avoids OS-level permissions; TUN can be Phase 2 enhancement
- **Binary protocol over JSON**: Lower overhead for high-throughput tunneling
- **UUID v4 for SessionID**: Cryptographically random, no coordination needed
- **Separate WebSocket listeners for Domain A/B on server**: Allows deployment on physically separate infrastructure for maximum traffic analysis resistance
- **Protobuf optional**: Start with hand-rolled binary format for minimal dependencies; add protobuf if cross-language clients needed
