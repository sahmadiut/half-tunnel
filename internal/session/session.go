// Package session provides UUID-based session tracking for the Half-Tunnel system.
package session

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// State represents the current state of a stream.
type State int

const (
	StateOpen       State = iota // Initial state
	StateActive                  // Data is flowing
	StateHalfClosed              // One direction closed
	StateClosed                  // Fully closed
)

// String returns a string representation of the state.
func (s State) String() string {
	switch s {
	case StateOpen:
		return "OPEN"
	case StateActive:
		return "ACTIVE"
	case StateHalfClosed:
		return "HALF_CLOSED"
	case StateClosed:
		return "CLOSED"
	default:
		return "UNKNOWN"
	}
}

// Stream represents a logical connection within a session.
type Stream struct {
	ID        uint32
	State     State
	SeqNum    uint32 // Next sequence number to send
	AckNum    uint32 // Next expected sequence number
	BytesSent int64  // Total bytes sent through this stream
	BytesRecv int64  // Total bytes received through this stream
	CreatedAt time.Time
	UpdatedAt time.Time
	mu        sync.RWMutex
}

// StreamState holds the state of a stream for persistence and resumption.
// This is used for stream recovery after reconnection.
type StreamState struct {
	ID           uint32
	State        State
	BytesSent    int64
	BytesRecv    int64
	LastActivity time.Time
	Checksum     uint32 // For data integrity verification
}

// NewStream creates a new stream with the given ID.
func NewStream(id uint32) *Stream {
	now := time.Now()
	return &Stream{
		ID:        id,
		State:     StateOpen,
		SeqNum:    0,
		AckNum:    0,
		BytesSent: 0,
		BytesRecv: 0,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// GetState returns the current state of the stream.
func (s *Stream) GetState() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// SetState sets the state of the stream.
func (s *Stream) SetState(state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
	s.UpdatedAt = time.Now()
}

// NextSeqNum returns and increments the sequence number.
func (s *Stream) NextSeqNum() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	seq := s.SeqNum
	s.SeqNum++
	s.UpdatedAt = time.Now()
	return seq
}

// UpdateAckNum updates the acknowledgment number.
func (s *Stream) UpdateAckNum(ack uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ack > s.AckNum {
		s.AckNum = ack
		s.UpdatedAt = time.Now()
	}
}

// AddBytesSent adds bytes to the sent counter.
func (s *Stream) AddBytesSent(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesSent += n
	s.UpdatedAt = time.Now()
}

// AddBytesRecv adds bytes to the received counter.
func (s *Stream) AddBytesRecv(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesRecv += n
	s.UpdatedAt = time.Now()
}

// Session represents a client session with upstream and downstream state.
type Session struct {
	ID        uuid.UUID
	streams   map[uint32]*Stream
	CreatedAt time.Time
	UpdatedAt time.Time
	mu        sync.RWMutex
}

// New creates a new session with a random UUID.
func New() *Session {
	now := time.Now()
	return &Session{
		ID:        uuid.New(),
		streams:   make(map[uint32]*Stream),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// NewWithID creates a new session with the given UUID.
func NewWithID(id uuid.UUID) *Session {
	now := time.Now()
	return &Session{
		ID:        id,
		streams:   make(map[uint32]*Stream),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// GetStream returns the stream with the given ID, creating it if necessary.
func (s *Session) GetStream(streamID uint32) *Stream {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream, exists := s.streams[streamID]
	if !exists {
		stream = NewStream(streamID)
		s.streams[streamID] = stream
	}
	s.UpdatedAt = time.Now()
	return stream
}

// GetExistingStream returns the stream with the given ID if it exists.
func (s *Session) GetExistingStream(streamID uint32) (*Stream, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stream, exists := s.streams[streamID]
	return stream, exists
}

// RemoveStream removes a stream from the session.
func (s *Session) RemoveStream(streamID uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streams, streamID)
	s.UpdatedAt = time.Now()
}

// StreamCount returns the number of active streams.
func (s *Session) StreamCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.streams)
}

// IsExpired returns true if the session has been idle for longer than the timeout.
func (s *Session) IsExpired(timeout time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.UpdatedAt) > timeout
}

// Touch updates the session's last activity time.
func (s *Session) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UpdatedAt = time.Now()
}

// ResumeStream resumes a stream with the given state after reconnection.
// This allows stream recovery after connection failures.
func (s *Session) ResumeStream(id uint32, state StreamState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream := NewStream(id)
	stream.State = state.State
	stream.BytesSent = state.BytesSent
	stream.BytesRecv = state.BytesRecv
	s.streams[id] = stream
	s.UpdatedAt = time.Now()
	return nil
}

// GetStreamState returns the current state of a stream for persistence.
func (s *Session) GetStreamState(streamID uint32) (StreamState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream, exists := s.streams[streamID]
	if !exists {
		return StreamState{}, false
	}

	stream.mu.RLock()
	defer stream.mu.RUnlock()

	return StreamState{
		ID:           stream.ID,
		State:        stream.State,
		BytesSent:    stream.BytesSent,
		BytesRecv:    stream.BytesRecv,
		LastActivity: stream.UpdatedAt,
	}, true
}
