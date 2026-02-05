// Package session provides UUID-based session tracking for the Half-Tunnel system.
package session

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Errors for session operations
var (
	// ErrStreamAlreadyResumed indicates the stream has already progressed beyond the saved state.
	ErrStreamAlreadyResumed = errors.New("stream already resumed or progressed beyond saved state")
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
	ID           uint32
	State        State
	SeqNum       uint32 // Next sequence number to send
	AckNum       uint32 // Next expected sequence number
	BytesSent    int64  // Total bytes sent on this stream
	BytesRecv    int64  // Total bytes received on this stream
	LastActivity time.Time
	Checksum     uint32 // Rolling checksum for data integrity verification
	CreatedAt    time.Time
	UpdatedAt    time.Time
	mu           sync.RWMutex
}

// StreamState represents the serializable state of a stream for persistence and resumption.
type StreamState struct {
	ID           uint32
	State        State
	SeqNum       uint32
	AckNum       uint32
	BytesSent    int64
	BytesRecv    int64
	LastActivity time.Time
	Checksum     uint32
}

// NewStream creates a new stream with the given ID.
func NewStream(id uint32) *Stream {
	now := time.Now()
	return &Stream{
		ID:           id,
		State:        StateOpen,
		SeqNum:       0,
		AckNum:       0,
		BytesSent:    0,
		BytesRecv:    0,
		LastActivity: now,
		Checksum:     0,
		CreatedAt:    now,
		UpdatedAt:    now,
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

// RecordSend records bytes sent on this stream and updates the checksum.
func (s *Stream) RecordSend(bytes int64, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesSent += bytes
	now := time.Now()
	s.LastActivity = now
	s.UpdatedAt = now
	// Update rolling checksum with sent data
	s.Checksum = updateChecksum(s.Checksum, data)
}

// RecordReceive records bytes received on this stream and updates the checksum.
func (s *Stream) RecordReceive(bytes int64, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesRecv += bytes
	now := time.Now()
	s.LastActivity = now
	s.UpdatedAt = now
	// Update rolling checksum with received data
	s.Checksum = updateChecksum(s.Checksum, data)
}

// GetStreamState returns a snapshot of the stream state for persistence.
func (s *Stream) GetStreamState() StreamState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StreamState{
		ID:           s.ID,
		State:        s.State,
		SeqNum:       s.SeqNum,
		AckNum:       s.AckNum,
		BytesSent:    s.BytesSent,
		BytesRecv:    s.BytesRecv,
		LastActivity: s.LastActivity,
		Checksum:     s.Checksum,
	}
}

// GetChecksum returns the current checksum value.
func (s *Stream) GetChecksum() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Checksum
}

// updateChecksum updates a rolling CRC32 checksum with new data.
func updateChecksum(current uint32, data []byte) uint32 {
	if len(data) == 0 {
		return current
	}
	// Simple rolling checksum using CRC32-like algorithm
	for _, b := range data {
		current = (current << 8) ^ uint32(b) ^ (current >> 24)
	}
	return current
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

// ResumeStream restores a stream from a saved state, allowing stream resumption after reconnection.
// If the stream already exists and has progressed beyond the saved state, the resumption is skipped.
func (s *Session) ResumeStream(state StreamState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.streams[state.ID]
	if exists {
		// Check if existing stream has progressed beyond saved state
		existing.mu.RLock()
		if existing.SeqNum > state.SeqNum || existing.AckNum > state.AckNum {
			existing.mu.RUnlock()
			return ErrStreamAlreadyResumed
		}
		existing.mu.RUnlock()
	}

	// Create or update stream from saved state
	now := time.Now()
	stream := &Stream{
		ID:           state.ID,
		State:        state.State,
		SeqNum:       state.SeqNum,
		AckNum:       state.AckNum,
		BytesSent:    state.BytesSent,
		BytesRecv:    state.BytesRecv,
		LastActivity: state.LastActivity,
		Checksum:     state.Checksum,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	// If the stream existed, preserve the original creation time
	if exists {
		stream.CreatedAt = existing.CreatedAt
	}
	s.streams[state.ID] = stream
	s.UpdatedAt = now
	return nil
}

// GetAllStreamStates returns the state of all active streams for persistence.
func (s *Session) GetAllStreamStates() []StreamState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make([]StreamState, 0, len(s.streams))
	for _, stream := range s.streams {
		states = append(states, stream.GetStreamState())
	}
	return states
}
