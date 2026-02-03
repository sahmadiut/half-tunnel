package session

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewSession(t *testing.T) {
	s := New()

	if s.ID == uuid.Nil {
		t.Error("Session ID should not be nil")
	}

	if s.StreamCount() != 0 {
		t.Errorf("New session should have 0 streams, got %d", s.StreamCount())
	}

	if s.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestNewSessionWithID(t *testing.T) {
	id := uuid.New()
	s := NewWithID(id)

	if s.ID != id {
		t.Errorf("Session ID mismatch: got %v, want %v", s.ID, id)
	}
}

func TestSessionStreams(t *testing.T) {
	s := New()

	// Get (creates) stream
	stream1 := s.GetStream(1)
	if stream1.ID != 1 {
		t.Errorf("Stream ID mismatch: got %v, want 1", stream1.ID)
	}

	if s.StreamCount() != 1 {
		t.Errorf("Expected 1 stream, got %d", s.StreamCount())
	}

	// Get existing stream
	stream1Again := s.GetStream(1)
	if stream1 != stream1Again {
		t.Error("Should return same stream instance")
	}

	// Get another stream
	stream2 := s.GetStream(2)
	if s.StreamCount() != 2 {
		t.Errorf("Expected 2 streams, got %d", s.StreamCount())
	}

	// Remove stream
	s.RemoveStream(1)
	if s.StreamCount() != 1 {
		t.Errorf("Expected 1 stream after removal, got %d", s.StreamCount())
	}

	_, exists := s.GetExistingStream(1)
	if exists {
		t.Error("Stream 1 should not exist after removal")
	}

	_, exists = s.GetExistingStream(2)
	if !exists {
		t.Error("Stream 2 should still exist")
	}

	_ = stream2 // use variable
}

func TestStreamState(t *testing.T) {
	stream := NewStream(1)

	if stream.GetState() != StateOpen {
		t.Errorf("Initial state should be OPEN, got %v", stream.GetState())
	}

	stream.SetState(StateActive)
	if stream.GetState() != StateActive {
		t.Errorf("State should be ACTIVE, got %v", stream.GetState())
	}

	stream.SetState(StateHalfClosed)
	if stream.GetState() != StateHalfClosed {
		t.Errorf("State should be HALF_CLOSED, got %v", stream.GetState())
	}

	stream.SetState(StateClosed)
	if stream.GetState() != StateClosed {
		t.Errorf("State should be CLOSED, got %v", stream.GetState())
	}
}

func TestStreamSequenceNumbers(t *testing.T) {
	stream := NewStream(1)

	seq1 := stream.NextSeqNum()
	if seq1 != 0 {
		t.Errorf("First SeqNum should be 0, got %d", seq1)
	}

	seq2 := stream.NextSeqNum()
	if seq2 != 1 {
		t.Errorf("Second SeqNum should be 1, got %d", seq2)
	}

	stream.UpdateAckNum(10)
	// Access through lock
	stream.mu.RLock()
	ackNum := stream.AckNum
	stream.mu.RUnlock()
	if ackNum != 10 {
		t.Errorf("AckNum should be 10, got %d", ackNum)
	}

	// Should not decrease
	stream.UpdateAckNum(5)
	stream.mu.RLock()
	ackNum = stream.AckNum
	stream.mu.RUnlock()
	if ackNum != 10 {
		t.Errorf("AckNum should still be 10, got %d", ackNum)
	}
}

func TestSessionExpiration(t *testing.T) {
	s := New()

	// Not expired immediately
	if s.IsExpired(time.Minute) {
		t.Error("Session should not be expired immediately")
	}

	// Expired with very short timeout
	if !s.IsExpired(time.Nanosecond) {
		t.Error("Session should be expired with nanosecond timeout")
	}
}

func TestSessionTouch(t *testing.T) {
	s := New()
	originalTime := s.UpdatedAt

	time.Sleep(10 * time.Millisecond)
	s.Touch()

	if !s.UpdatedAt.After(originalTime) {
		t.Error("Touch should update UpdatedAt")
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateOpen, "OPEN"},
		{StateActive, "ACTIVE"},
		{StateHalfClosed, "HALF_CLOSED"},
		{StateClosed, "CLOSED"},
		{State(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("State %d String() = %s, want %s", tt.state, tt.state.String(), tt.expected)
		}
	}
}
