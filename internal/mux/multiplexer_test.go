package mux

import (
	"testing"

	"github.com/google/uuid"
	"github.com/sahmadiut/half-tunnel/internal/protocol"
	"github.com/sahmadiut/half-tunnel/internal/session"
)

func TestStreamBufferInOrder(t *testing.T) {
	buf := NewStreamBuffer(1024)

	// Write in order
	if err := buf.Write(0, []byte("Hello")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := buf.Write(1, []byte(" ")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := buf.Write(2, []byte("World")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data := buf.ReadAll()
	if string(data) != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", string(data))
	}
}

func TestStreamBufferOutOfOrder(t *testing.T) {
	buf := NewStreamBuffer(1024)

	// Write out of order
	if err := buf.Write(2, []byte("C")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := buf.Write(0, []byte("A")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := buf.Write(1, []byte("B")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data := buf.ReadAll()
	if string(data) != "ABC" {
		t.Errorf("Expected 'ABC', got '%s'", string(data))
	}
}

func TestStreamBufferMaxSize(t *testing.T) {
	buf := NewStreamBuffer(10)

	// Write up to max
	if err := buf.Write(0, []byte("12345")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := buf.Write(1, []byte("67890")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Should fail when exceeding max
	err := buf.Write(2, []byte("X"))
	if err != ErrBufferFull {
		t.Errorf("Expected ErrBufferFull, got %v", err)
	}
}

func TestStreamBufferDuplicateSeqNum(t *testing.T) {
	buf := NewStreamBuffer(1024)

	if err := buf.Write(0, []byte("First")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	// Duplicate should be ignored
	if err := buf.Write(0, []byte("Duplicate")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data := buf.ReadAll()
	if string(data) != "First" {
		t.Errorf("Expected 'First', got '%s'", string(data))
	}
}

func TestStreamBufferOutOfOrderSegments(t *testing.T) {
	buf := NewStreamBuffer(1024)

	// Write segments 3, 4, 5 before 0, 1, 2
	if err := buf.Write(3, []byte("D")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := buf.Write(4, []byte("E")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := buf.Write(5, []byte("F")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Check nothing in data yet (waiting for seq 0)
	data := buf.ReadAll()
	if len(data) != 0 {
		t.Errorf("Expected empty data, got '%s'", string(data))
	}

	// Now write 0, 1, 2 to trigger flush
	if err := buf.Write(0, []byte("A")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := buf.Write(1, []byte("B")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := buf.Write(2, []byte("C")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data = buf.ReadAll()
	if string(data) != "ABCDEF" {
		t.Errorf("Expected 'ABCDEF', got '%s'", string(data))
	}
}

func TestStreamBufferLen(t *testing.T) {
	buf := NewStreamBuffer(1024)

	if buf.Len() != 0 {
		t.Errorf("Expected length 0, got %d", buf.Len())
	}

	if err := buf.Write(0, []byte("12345")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if buf.Len() != 5 {
		t.Errorf("Expected length 5, got %d", buf.Len())
	}
}

func TestNewMultiplexer(t *testing.T) {
	sess := session.New()
	mux := NewMultiplexer(sess)

	if mux.session != sess {
		t.Error("Session not set correctly")
	}
}

func TestMultiplexerOpenStream(t *testing.T) {
	sess := session.New()
	mux := NewMultiplexer(sess)

	streamID1, err := mux.OpenStream()
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	if streamID1 != 1 {
		t.Errorf("Expected stream ID 1, got %d", streamID1)
	}

	streamID2, err := mux.OpenStream()
	if err != nil {
		t.Fatalf("OpenStream failed: %v", err)
	}
	if streamID2 != 2 {
		t.Errorf("Expected stream ID 2, got %d", streamID2)
	}
}

func TestMultiplexerCloseStream(t *testing.T) {
	sess := session.New()
	mux := NewMultiplexer(sess)

	streamID, _ := mux.OpenStream()
	
	err := mux.CloseStream(streamID)
	if err != nil {
		t.Fatalf("CloseStream failed: %v", err)
	}

	// Closing non-existent stream should fail
	err = mux.CloseStream(999)
	if err != ErrStreamNotFound {
		t.Errorf("Expected ErrStreamNotFound, got %v", err)
	}
}

func TestMultiplexerHandlePacket(t *testing.T) {
	sess := session.New()
	mux := NewMultiplexer(sess)

	pkt, _ := protocol.NewPacket(sess.ID, 1, protocol.FlagData, []byte("test data"))
	pkt.SeqNum = 0

	err := mux.HandlePacket(pkt)
	if err != nil {
		t.Fatalf("HandlePacket failed: %v", err)
	}

	data, err := mux.ReadStream(1)
	if err != nil {
		t.Fatalf("ReadStream failed: %v", err)
	}
	if string(data) != "test data" {
		t.Errorf("Expected 'test data', got '%s'", string(data))
	}
}

func TestMultiplexerSendPacket(t *testing.T) {
	sess := session.New()
	mux := NewMultiplexer(sess)

	var sentPacket *protocol.Packet
	mux.SetPacketHandler(func(pkt *protocol.Packet) error {
		sentPacket = pkt
		return nil
	})

	streamID, _ := mux.OpenStream()

	err := mux.SendPacket(streamID, protocol.FlagData, []byte("hello"))
	if err != nil {
		t.Fatalf("SendPacket failed: %v", err)
	}

	if sentPacket == nil {
		t.Fatal("Packet handler not called")
	}
	if sentPacket.StreamID != streamID {
		t.Errorf("Wrong stream ID: got %d, want %d", sentPacket.StreamID, streamID)
	}
	if string(sentPacket.Payload) != "hello" {
		t.Errorf("Wrong payload: got '%s', want 'hello'", string(sentPacket.Payload))
	}
}

func TestMultiplexerClose(t *testing.T) {
	sess := session.New()
	mux := NewMultiplexer(sess)

	_, _ = mux.OpenStream()
	_, _ = mux.OpenStream()

	err := mux.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations should fail after close
	_, err = mux.OpenStream()
	if err != ErrMuxClosed {
		t.Errorf("Expected ErrMuxClosed, got %v", err)
	}
}

func TestMultiplexerSendPacketNoHandler(t *testing.T) {
	sess := session.New()
	mux := NewMultiplexer(sess)

	streamID, _ := mux.OpenStream()

	err := mux.SendPacket(streamID, protocol.FlagData, []byte("hello"))
	if err == nil {
		t.Error("Expected error when no handler set")
	}
}

func TestMultiplexerSendPacketStreamNotFound(t *testing.T) {
	sess := session.New()
	mux := NewMultiplexer(sess)
	mux.SetPacketHandler(func(pkt *protocol.Packet) error {
		return nil
	})

	err := mux.SendPacket(999, protocol.FlagData, []byte("hello"))
	if err != ErrStreamNotFound {
		t.Errorf("Expected ErrStreamNotFound, got %v", err)
	}
}

func TestMultiplexerHandlePacketClosed(t *testing.T) {
	sess := session.New()
	mux := NewMultiplexer(sess)
	mux.Close()

	pkt, _ := protocol.NewPacket(uuid.New(), 1, protocol.FlagData, []byte("test"))
	err := mux.HandlePacket(pkt)
	if err != ErrMuxClosed {
		t.Errorf("Expected ErrMuxClosed, got %v", err)
	}
}
