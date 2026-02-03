// Package constants provides shared constants for the Half-Tunnel system.
package constants

// Buffer sizes for network I/O.
const (
	// DefaultBufferSize is the default buffer size for reading and writing data.
	DefaultBufferSize = 32768 // 32KB

	// DefaultChannelBufferSize is the default buffer size for connection channels.
	DefaultChannelBufferSize = 100

	// DefaultMaxMessageSize is the default maximum WebSocket message size.
	DefaultMaxMessageSize = 65536 // 64KB
)
