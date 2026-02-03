// Package mux provides multiplexing for logical connections within a session.
package mux

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/sahmadiut/half-tunnel/internal/protocol"
	"github.com/sahmadiut/half-tunnel/internal/session"
)

// Errors
var (
	ErrStreamNotFound = errors.New("stream not found")
	ErrStreamClosed   = errors.New("stream is closed")
	ErrMuxClosed      = errors.New("multiplexer is closed")
)

// Multiplexer routes packets to the correct stream within a session.
type Multiplexer struct {
	session       *session.Session
	nextStreamID  uint32
	streamBuffers map[uint32]*StreamBuffer
	closed        bool
	mu            sync.RWMutex

	// Callbacks for handling packets
	onPacket func(*protocol.Packet) error
}

// NewMultiplexer creates a new multiplexer for the given session.
func NewMultiplexer(s *session.Session) *Multiplexer {
	return &Multiplexer{
		session:       s,
		nextStreamID:  1,
		streamBuffers: make(map[uint32]*StreamBuffer),
	}
}

// SetPacketHandler sets the callback for outgoing packets.
func (m *Multiplexer) SetPacketHandler(handler func(*protocol.Packet) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPacket = handler
}

// OpenStream creates a new stream and returns its ID.
func (m *Multiplexer) OpenStream() (uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, ErrMuxClosed
	}

	streamID := atomic.AddUint32(&m.nextStreamID, 1) - 1
	m.session.GetStream(streamID)
	m.streamBuffers[streamID] = NewStreamBuffer(1024) // 1KB default buffer

	return streamID, nil
}

// CloseStream closes a stream.
func (m *Multiplexer) CloseStream(streamID uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stream, exists := m.session.GetExistingStream(streamID)
	if !exists {
		return ErrStreamNotFound
	}

	stream.SetState(session.StateClosed)
	delete(m.streamBuffers, streamID)
	m.session.RemoveStream(streamID)

	return nil
}

// HandlePacket routes an incoming packet to the correct stream.
func (m *Multiplexer) HandlePacket(pkt *protocol.Packet) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrMuxClosed
	}
	m.mu.RUnlock()

	m.mu.Lock()
	stream := m.session.GetStream(pkt.StreamID)

	// Get or create buffer
	buf, exists := m.streamBuffers[pkt.StreamID]
	if !exists {
		buf = NewStreamBuffer(1024)
		m.streamBuffers[pkt.StreamID] = buf
	}
	m.mu.Unlock()

	// Update stream state based on packet flags
	if pkt.IsHandshake() && stream.GetState() == session.StateOpen {
		stream.SetState(session.StateActive)
	} else if pkt.IsFin() {
		if stream.GetState() == session.StateActive {
			stream.SetState(session.StateHalfClosed)
		} else if stream.GetState() == session.StateHalfClosed {
			stream.SetState(session.StateClosed)
		}
	}

	// Update acknowledgment number
	if pkt.IsAck() {
		stream.UpdateAckNum(pkt.AckNum)
	}

	// Buffer the payload if present
	if pkt.IsData() && len(pkt.Payload) > 0 {
		if err := buf.Write(pkt.SeqNum, pkt.Payload); err != nil {
			return err
		}
	}

	return nil
}

// SendPacket creates and sends a packet for a stream.
func (m *Multiplexer) SendPacket(streamID uint32, flags protocol.Flag, payload []byte) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrMuxClosed
	}
	handler := m.onPacket
	m.mu.RUnlock()

	if handler == nil {
		return errors.New("no packet handler set")
	}

	stream, exists := m.session.GetExistingStream(streamID)
	if !exists {
		return ErrStreamNotFound
	}

	if stream.GetState() == session.StateClosed {
		return ErrStreamClosed
	}

	pkt, err := protocol.NewPacket(m.session.ID, streamID, flags, payload)
	if err != nil {
		return err
	}

	pkt.SeqNum = stream.NextSeqNum()

	return handler(pkt)
}

// ReadStream reads available data from a stream's buffer.
func (m *Multiplexer) ReadStream(streamID uint32) ([]byte, error) {
	m.mu.RLock()
	buf, exists := m.streamBuffers[streamID]
	m.mu.RUnlock()

	if !exists {
		return nil, ErrStreamNotFound
	}

	return buf.ReadAll(), nil
}

// Close closes the multiplexer and all streams.
func (m *Multiplexer) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	m.streamBuffers = make(map[uint32]*StreamBuffer)

	return nil
}

// StreamBuffer provides a simple buffer for stream data.
// In production, this would be a ring buffer for out-of-order packet reassembly.
type StreamBuffer struct {
	data     []byte
	maxSize  int
	mu       sync.Mutex
	segments map[uint32][]byte // SeqNum -> data for out-of-order handling
}

// NewStreamBuffer creates a new stream buffer with the given max size.
func NewStreamBuffer(maxSize int) *StreamBuffer {
	return &StreamBuffer{
		maxSize:  maxSize,
		segments: make(map[uint32][]byte),
	}
}

// Write adds data to the buffer at the given sequence number.
func (b *StreamBuffer) Write(seqNum uint32, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Simple implementation: just append for now
	// Production would handle out-of-order segments
	b.data = append(b.data, data...)

	return nil
}

// ReadAll returns all buffered data and clears the buffer.
func (b *StreamBuffer) ReadAll() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	data := b.data
	b.data = nil
	return data
}

// Len returns the current size of buffered data.
func (b *StreamBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.data)
}
