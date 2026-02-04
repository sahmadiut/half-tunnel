package transport

import (
	"context"
	"testing"
	"time"

	"github.com/sahmadiut/half-tunnel/internal/constants"
)

func TestBufferPool(t *testing.T) {
	tests := []struct {
		name string
		size int
		want int
	}{
		{"default size", 0, constants.DefaultBufferSize},
		{"custom size", 1024, 1024},
		{"large size", 65536, 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewBufferPool(tt.size)
			buf := pool.Get()

			expectedSize := tt.want
			if tt.size <= 0 {
				expectedSize = constants.DefaultBufferSize
			}

			if len(buf) != expectedSize {
				t.Errorf("BufferPool.Get() buffer size = %d, want %d", len(buf), expectedSize)
			}

			// Test putting buffer back
			pool.Put(buf)

			// Get another buffer - should reuse
			buf2 := pool.Get()
			if len(buf2) != expectedSize {
				t.Errorf("BufferPool.Get() after Put() buffer size = %d, want %d", len(buf2), expectedSize)
			}
		})
	}
}

func TestBufferPoolPutInvalidSize(t *testing.T) {
	pool := NewBufferPool(1024)

	// Put a buffer that's too small - should not be reused
	smallBuf := make([]byte, 512)
	pool.Put(smallBuf)

	// Get should return a new buffer of the correct size
	buf := pool.Get()
	if len(buf) != 1024 {
		t.Errorf("BufferPool.Get() buffer size = %d, want 1024", len(buf))
	}
}

func TestGlobalBufferPools(t *testing.T) {
	tests := []struct {
		name     string
		pool     *BufferPool
		expected int
	}{
		{"DefaultBufferPool", DefaultBufferPool, constants.DefaultBufferSize},
		{"SmallBufferPool", SmallBufferPool, constants.SmallBufferSize},
		{"LargeBufferPool", LargeBufferPool, constants.LargeBufferSize},
		{"MaxBufferPool", MaxBufferPool, constants.MaxBufferSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := tt.pool.Get()
			if len(buf) != tt.expected {
				t.Errorf("%s.Get() buffer size = %d, want %d", tt.name, len(buf), tt.expected)
			}
			tt.pool.Put(buf)
		})
	}
}

func TestGetBufferPool(t *testing.T) {
	tests := []struct {
		mode     constants.BufferMode
		expected *BufferPool
	}{
		{constants.BufferModeSmall, SmallBufferPool},
		{constants.BufferModeDefault, DefaultBufferPool},
		{constants.BufferModeLarge, LargeBufferPool},
		{constants.BufferModeMax, MaxBufferPool},
		{"unknown", DefaultBufferPool},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			pool := GetBufferPool(tt.mode)
			if pool != tt.expected {
				t.Errorf("GetBufferPool(%s) returned wrong pool", tt.mode)
			}
		})
	}
}

func TestConnectionPoolClose(t *testing.T) {
	config := DefaultConfig("ws://localhost:8080")
	pool := NewConnectionPool(config, 5, time.Minute)

	// Close the pool
	err := pool.Close()
	if err != nil {
		t.Errorf("ConnectionPool.Close() error = %v", err)
	}

	// Get should return error after close
	ctx := context.Background()
	_, err = pool.Get(ctx)
	if err != ErrPoolClosed {
		t.Errorf("ConnectionPool.Get() after Close() error = %v, want ErrPoolClosed", err)
	}
}

func TestConnectionPoolDefaults(t *testing.T) {
	config := DefaultConfig("ws://localhost:8080")

	// Test with zero/negative values - should use defaults
	pool := NewConnectionPool(config, 0, 0)
	if pool.maxSize != 10 {
		t.Errorf("ConnectionPool.maxSize = %d, want 10", pool.maxSize)
	}
	if pool.idleTimeout != 5*time.Minute {
		t.Errorf("ConnectionPool.idleTimeout = %v, want 5m", pool.idleTimeout)
	}
}

func TestConnectionPoolSize(t *testing.T) {
	config := DefaultConfig("ws://localhost:8080")
	pool := NewConnectionPool(config, 5, time.Minute)
	defer pool.Close()

	// Initially empty
	if pool.Size() != 0 {
		t.Errorf("ConnectionPool.Size() = %d, want 0", pool.Size())
	}
}
