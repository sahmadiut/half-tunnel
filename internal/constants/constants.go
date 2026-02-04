// Package constants provides shared constants for the Half-Tunnel system.
package constants

// Buffer sizes for network I/O.
const (
	// SmallBufferSize is for interactive sessions with small payloads.
	SmallBufferSize = 16384 // 16KB

	// DefaultBufferSize is the default buffer size for reading and writing data.
	DefaultBufferSize = 32768 // 32KB

	// LargeBufferSize is for bulk data transfers.
	LargeBufferSize = 65536 // 64KB

	// MaxBufferSize is for high-throughput mode.
	MaxBufferSize = 131072 // 128KB

	// DefaultChannelBufferSize is the default buffer size for connection channels.
	DefaultChannelBufferSize = 5000

	// DefaultMaxMessageSize is the default maximum WebSocket message size.
	DefaultMaxMessageSize = 65536 // 64KB

	// DefaultStreamBufferSize is the default buffer size for stream reassembly.
	DefaultStreamBufferSize = 65536 // 64KB
)

// BufferMode represents the buffer size mode for connections.
type BufferMode string

const (
	// BufferModeSmall uses SmallBufferSize (16KB) for interactive sessions.
	BufferModeSmall BufferMode = "small"
	// BufferModeDefault uses DefaultBufferSize (32KB) for balanced performance.
	BufferModeDefault BufferMode = "default"
	// BufferModeLarge uses LargeBufferSize (64KB) for bulk transfers.
	BufferModeLarge BufferMode = "large"
	// BufferModeMax uses MaxBufferSize (128KB) for high-throughput mode.
	BufferModeMax BufferMode = "max"
)

// GetBufferSize returns the buffer size for a given mode.
func GetBufferSize(mode BufferMode) int {
	switch mode {
	case BufferModeSmall:
		return SmallBufferSize
	case BufferModeLarge:
		return LargeBufferSize
	case BufferModeMax:
		return MaxBufferSize
	default:
		return DefaultBufferSize
	}
}
