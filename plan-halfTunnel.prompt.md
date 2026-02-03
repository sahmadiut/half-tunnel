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
| Tests pass                  | `make test` with 70%+ coverage                         |
| Lint clean                  | `golangci-lint run` with no errors                     |
| Protocol correctness        | Fuzz tests + integration tests                         |
| Split-path works            | E2E test: traffic observed on both Domain A and B only |
| Reconnection works          | Chaos test: kill upstream, verify session resume       |
| Docker images functional    | `docker-compose up` → client connects successfully     |
| Release automation          | Tag push → binaries + images published                 |

---

## Decisions

- **SOCKS5 over TUN for Phase 1**: Simpler cross-platform, avoids OS-level permissions; TUN can be Phase 2 enhancement
- **Binary protocol over JSON**: Lower overhead for high-throughput tunneling
- **UUID v4 for SessionID**: Cryptographically random, no coordination needed
- **Separate WebSocket listeners for Domain A/B on server**: Allows deployment on physically separate infrastructure for maximum traffic analysis resistance
- **Protobuf optional**: Start with hand-rolled binary format for minimal dependencies; add protobuf if cross-language clients needed
