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

// Tests for new Phase 3 features

func TestStreamRecordSendReceive(t *testing.T) {
	stream := NewStream(1)

	// Initial values
	state := stream.GetStreamState()
	if state.BytesSent != 0 || state.BytesRecv != 0 {
		t.Error("Expected initial bytes to be 0")
	}

	// Record send
	testData := []byte("hello world")
	stream.RecordSend(int64(len(testData)), testData)

	state = stream.GetStreamState()
	if state.BytesSent != int64(len(testData)) {
		t.Errorf("Expected BytesSent %d, got %d", len(testData), state.BytesSent)
	}
	if state.Checksum == 0 {
		t.Error("Expected non-zero checksum after send")
	}

	// Record receive
	recvData := []byte("response")
	stream.RecordReceive(int64(len(recvData)), recvData)

	state = stream.GetStreamState()
	if state.BytesRecv != int64(len(recvData)) {
		t.Errorf("Expected BytesRecv %d, got %d", len(recvData), state.BytesRecv)
	}
}

func TestStreamGetChecksum(t *testing.T) {
	stream := NewStream(1)

	// Initial checksum should be 0
	if stream.GetChecksum() != 0 {
		t.Error("Expected initial checksum to be 0")
	}

	// Send some data
	stream.RecordSend(5, []byte("hello"))
	checksum1 := stream.GetChecksum()

	// Send more data, checksum should change
	stream.RecordSend(5, []byte("world"))
	checksum2 := stream.GetChecksum()

	if checksum1 == checksum2 {
		t.Error("Expected checksum to change after more data")
	}
}

func TestStreamStateSnapshot(t *testing.T) {
	stream := NewStream(42)
	stream.SetState(StateActive)
	stream.RecordSend(100, []byte("test data"))
	stream.RecordReceive(50, []byte("resp"))

	state := stream.GetStreamState()

	if state.ID != 42 {
		t.Errorf("Expected ID 42, got %d", state.ID)
	}
	if state.State != StateActive {
		t.Errorf("Expected StateActive, got %v", state.State)
	}
	if state.BytesSent != 100 {
		t.Errorf("Expected BytesSent 100, got %d", state.BytesSent)
	}
	if state.BytesRecv != 50 {
		t.Errorf("Expected BytesRecv 50, got %d", state.BytesRecv)
	}
	if state.LastActivity.IsZero() {
		t.Error("Expected non-zero LastActivity")
	}
}

func TestSessionResumeStream(t *testing.T) {
	session := New()

	// Create a stream state to resume
	state := StreamState{
		ID:           10,
		State:        StateActive,
		SeqNum:       100,
		AckNum:       50,
		BytesSent:    1000,
		BytesRecv:    500,
		LastActivity: time.Now().Add(-time.Minute),
		Checksum:     12345,
	}

	// Resume the stream
	err := session.ResumeStream(state)
	if err != nil {
		t.Fatalf("ResumeStream failed: %v", err)
	}

	// Verify the stream was created with the correct state
	stream, exists := session.GetExistingStream(10)
	if !exists {
		t.Fatal("Expected stream to exist after resume")
	}

	if stream.GetState() != StateActive {
		t.Errorf("Expected StateActive, got %v", stream.GetState())
	}

	streamState := stream.GetStreamState()
	if streamState.SeqNum != 100 {
		t.Errorf("Expected SeqNum 100, got %d", streamState.SeqNum)
	}
	if streamState.BytesSent != 1000 {
		t.Errorf("Expected BytesSent 1000, got %d", streamState.BytesSent)
	}
}

func TestSessionResumeStreamAlreadyProgressed(t *testing.T) {
	session := New()

	// Create an existing stream with higher sequence numbers
	existingStream := session.GetStream(10)
	existingStream.mu.Lock()
	existingStream.SeqNum = 200
	existingStream.AckNum = 100
	existingStream.mu.Unlock()

	// Try to resume with lower sequence numbers
	state := StreamState{
		ID:     10,
		SeqNum: 50,
		AckNum: 25,
	}

	err := session.ResumeStream(state)
	if err != ErrStreamAlreadyResumed {
		t.Errorf("Expected ErrStreamAlreadyResumed, got %v", err)
	}
}

func TestSessionGetAllStreamStates(t *testing.T) {
	session := New()

	// Create some streams
	stream1 := session.GetStream(1)
	stream1.RecordSend(100, []byte("data"))

	stream2 := session.GetStream(2)
	stream2.SetState(StateActive)
	stream2.RecordReceive(50, []byte("resp"))

	session.GetStream(3)

	// Get all stream states
	states := session.GetAllStreamStates()
	if len(states) != 3 {
		t.Fatalf("Expected 3 stream states, got %d", len(states))
	}

	// Verify we have all the expected stream IDs
	foundIDs := make(map[uint32]bool)
	for _, s := range states {
		foundIDs[s.ID] = true
	}

	for _, id := range []uint32{1, 2, 3} {
		if !foundIDs[id] {
			t.Errorf("Expected to find stream ID %d", id)
		}
	}
}
