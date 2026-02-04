// Package transport provides WebSocket connection managers for the Half-Tunnel system.
package transport

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sahmadiut/half-tunnel/internal/constants"
)

// Connection pool errors.
var (
	ErrPoolExhausted = errors.New("connection pool exhausted")
	ErrPoolClosed    = errors.New("connection pool closed")
)

// ConnectionPool manages a pool of reusable connections.
type ConnectionPool struct {
	config      *Config
	maxSize     int
	idleTimeout time.Duration
	connections chan *pooledConnection
	mu          sync.RWMutex
	closed      bool
}

// pooledConnection wraps a connection with pool metadata.
type pooledConnection struct {
	conn       *Connection
	lastUsed   time.Time
	createTime time.Time
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(config *Config, maxSize int, idleTimeout time.Duration) *ConnectionPool {
	if maxSize <= 0 {
		maxSize = 10
	}
	if idleTimeout <= 0 {
		idleTimeout = 5 * time.Minute
	}

	return &ConnectionPool{
		config:      config,
		maxSize:     maxSize,
		idleTimeout: idleTimeout,
		connections: make(chan *pooledConnection, maxSize),
	}
}

// Get retrieves a connection from the pool or creates a new one.
func (p *ConnectionPool) Get(ctx context.Context) (*Connection, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrPoolClosed
	}
	p.mu.RUnlock()

	// Try to get an existing connection, limit iterations to avoid inefficiency
	maxIterations := p.maxSize
	for i := 0; i < maxIterations; i++ {
		select {
		case pc := <-p.connections:
			// Check if the connection is still valid
			if pc.conn.IsClosed() || time.Since(pc.lastUsed) > p.idleTimeout {
				pc.conn.Close()
				continue
			}
			return pc.conn, nil
		default:
			// No available connections, create a new one
			return Dial(ctx, p.config)
		}
	}

	// All connections were stale, create a new one
	return Dial(ctx, p.config)
}

// Put returns a connection to the pool.
func (p *ConnectionPool) Put(conn *Connection) {
	if conn == nil || conn.IsClosed() {
		return
	}

	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		conn.Close()
		return
	}
	p.mu.RUnlock()

	pc := &pooledConnection{
		conn:     conn,
		lastUsed: time.Now(),
	}

	select {
	case p.connections <- pc:
		// Connection returned to pool
	default:
		// Pool is full, close the connection
		conn.Close()
	}
}

// Close closes the pool and all its connections.
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	close(p.connections)
	for pc := range p.connections {
		pc.conn.Close()
	}
	return nil
}

// Size returns the current number of connections in the pool.
func (p *ConnectionPool) Size() int {
	return len(p.connections)
}

// BufferPool provides a pool of reusable byte buffers to reduce allocations.
type BufferPool struct {
	pool sync.Pool
	size int
}

// NewBufferPool creates a new buffer pool with the specified buffer size.
func NewBufferPool(size int) *BufferPool {
	if size <= 0 {
		size = constants.DefaultBufferSize
	}
	return &BufferPool{
		size: size,
		pool: sync.Pool{
			New: func() interface{} {
				buf := make([]byte, size)
				return &buf
			},
		},
	}
}

// Get retrieves a buffer from the pool.
func (p *BufferPool) Get() []byte {
	buf := p.pool.Get().(*[]byte)
	return *buf
}

// Put returns a buffer to the pool.
func (p *BufferPool) Put(buf []byte) {
	if cap(buf) >= p.size {
		buf = buf[:p.size]
		p.pool.Put(&buf)
	}
}

// DefaultBufferPool is a global buffer pool using the default buffer size.
var DefaultBufferPool = NewBufferPool(constants.DefaultBufferSize)

// SmallBufferPool is a global buffer pool using the small buffer size.
var SmallBufferPool = NewBufferPool(constants.SmallBufferSize)

// LargeBufferPool is a global buffer pool using the large buffer size.
var LargeBufferPool = NewBufferPool(constants.LargeBufferSize)

// MaxBufferPool is a global buffer pool using the max buffer size.
var MaxBufferPool = NewBufferPool(constants.MaxBufferSize)

// GetBufferPool returns the appropriate buffer pool for the given buffer mode.
func GetBufferPool(mode constants.BufferMode) *BufferPool {
	switch mode {
	case constants.BufferModeSmall:
		return SmallBufferPool
	case constants.BufferModeLarge:
		return LargeBufferPool
	case constants.BufferModeMax:
		return MaxBufferPool
	default:
		return DefaultBufferPool
	}
}
