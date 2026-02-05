#!/bin/bash
# build.sh - Build Half-Tunnel binaries

set -e

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "none")}
BUILD_DATE=${BUILD_DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}

LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}"

echo "Building Half-Tunnel ${VERSION} (${COMMIT})"

# Build client
echo "Building client..."
go build -ldflags "${LDFLAGS}" -o bin/ht-client ./cmd/client

# Build server
echo "Building server..."
go build -ldflags "${LDFLAGS}" -o bin/ht-server ./cmd/server


# Build ht (Service Manager)
echo "Building ht..."
go build -ldflags "${LDFLAGS}" -o bin/ht ./cmd/ht

# Build half-tunnel (CLI)
echo "Building half-tunnel..."
go build -ldflags "${LDFLAGS}" -o bin/half-tunnel ./cmd/half-tunnel

echo "Build complete!"
ls -la bin/
