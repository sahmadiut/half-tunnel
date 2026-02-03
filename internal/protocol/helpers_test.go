package protocol

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewHandshakePacket(t *testing.T) {
	sessionID := uuid.New()
	pkt, err := NewHandshakePacket(sessionID)
	if err != nil {
		t.Fatalf("NewHandshakePacket failed: %v", err)
	}

	if pkt.SessionID != sessionID {
		t.Error("SessionID mismatch")
	}
	if pkt.StreamID != 0 {
		t.Errorf("StreamID should be 0 for handshake, got %d", pkt.StreamID)
	}
	if !pkt.IsHandshake() {
		t.Error("Packet should be a handshake")
	}
	if pkt.IsAck() {
		t.Error("Packet should not be an ack")
	}
	if !pkt.IsControlPacket() {
		t.Error("Handshake should be a control packet")
	}
	if pkt.PacketType() != "HANDSHAKE" {
		t.Errorf("PacketType should be HANDSHAKE, got %s", pkt.PacketType())
	}
}

func TestNewHandshakeAckPacket(t *testing.T) {
	sessionID := uuid.New()
	pkt, err := NewHandshakeAckPacket(sessionID)
	if err != nil {
		t.Fatalf("NewHandshakeAckPacket failed: %v", err)
	}

	if pkt.SessionID != sessionID {
		t.Error("SessionID mismatch")
	}
	if pkt.StreamID != 0 {
		t.Errorf("StreamID should be 0 for handshake ack, got %d", pkt.StreamID)
	}
	if !pkt.IsHandshake() {
		t.Error("Packet should be a handshake")
	}
	if !pkt.IsAck() {
		t.Error("Packet should be an ack")
	}
	if pkt.PacketType() != "HANDSHAKE_ACK" {
		t.Errorf("PacketType should be HANDSHAKE_ACK, got %s", pkt.PacketType())
	}
}

func TestNewDataPacket(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("test data")
	pkt, err := NewDataPacket(sessionID, 1, payload)
	if err != nil {
		t.Fatalf("NewDataPacket failed: %v", err)
	}

	if pkt.SessionID != sessionID {
		t.Error("SessionID mismatch")
	}
	if pkt.StreamID != 1 {
		t.Errorf("StreamID should be 1, got %d", pkt.StreamID)
	}
	if !pkt.IsData() {
		t.Error("Packet should be a data packet")
	}
	if string(pkt.Payload) != "test data" {
		t.Error("Payload mismatch")
	}
	if pkt.PacketType() != "DATA" {
		t.Errorf("PacketType should be DATA, got %s", pkt.PacketType())
	}
}

func TestNewDataAckPacket(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("response")
	ackNum := uint32(42)
	pkt, err := NewDataAckPacket(sessionID, 1, payload, ackNum)
	if err != nil {
		t.Fatalf("NewDataAckPacket failed: %v", err)
	}

	if !pkt.IsData() {
		t.Error("Packet should be a data packet")
	}
	if !pkt.IsAck() {
		t.Error("Packet should be an ack")
	}
	if pkt.AckNum != ackNum {
		t.Errorf("AckNum should be %d, got %d", ackNum, pkt.AckNum)
	}
	if pkt.PacketType() != "DATA_ACK" {
		t.Errorf("PacketType should be DATA_ACK, got %s", pkt.PacketType())
	}
}

func TestNewAckPacket(t *testing.T) {
	sessionID := uuid.New()
	ackNum := uint32(100)
	pkt, err := NewAckPacket(sessionID, 1, ackNum)
	if err != nil {
		t.Fatalf("NewAckPacket failed: %v", err)
	}

	if !pkt.IsAck() {
		t.Error("Packet should be an ack")
	}
	if pkt.IsData() {
		t.Error("Packet should not be a data packet")
	}
	if pkt.AckNum != ackNum {
		t.Errorf("AckNum should be %d, got %d", ackNum, pkt.AckNum)
	}
	if pkt.PacketType() != "ACK" {
		t.Errorf("PacketType should be ACK, got %s", pkt.PacketType())
	}
}

func TestNewKeepAlivePacket(t *testing.T) {
	sessionID := uuid.New()
	pkt, err := NewKeepAlivePacket(sessionID)
	if err != nil {
		t.Fatalf("NewKeepAlivePacket failed: %v", err)
	}

	if !pkt.IsKeepAlive() {
		t.Error("Packet should be a keep-alive")
	}
	if pkt.StreamID != 0 {
		t.Errorf("StreamID should be 0 for keep-alive, got %d", pkt.StreamID)
	}
	if !pkt.IsControlPacket() {
		t.Error("Keep-alive should be a control packet")
	}
	if pkt.PacketType() != "KEEPALIVE" {
		t.Errorf("PacketType should be KEEPALIVE, got %s", pkt.PacketType())
	}
}

func TestNewKeepAliveAckPacket(t *testing.T) {
	sessionID := uuid.New()
	pkt, err := NewKeepAliveAckPacket(sessionID)
	if err != nil {
		t.Fatalf("NewKeepAliveAckPacket failed: %v", err)
	}

	if !pkt.IsKeepAlive() {
		t.Error("Packet should be a keep-alive")
	}
	if !pkt.IsAck() {
		t.Error("Packet should be an ack")
	}
	if pkt.PacketType() != "KEEPALIVE_ACK" {
		t.Errorf("PacketType should be KEEPALIVE_ACK, got %s", pkt.PacketType())
	}
}

func TestNewFinPacket(t *testing.T) {
	sessionID := uuid.New()
	pkt, err := NewFinPacket(sessionID, 5)
	if err != nil {
		t.Fatalf("NewFinPacket failed: %v", err)
	}

	if !pkt.IsFin() {
		t.Error("Packet should be a FIN")
	}
	if pkt.StreamID != 5 {
		t.Errorf("StreamID should be 5, got %d", pkt.StreamID)
	}
	if pkt.PacketType() != "FIN" {
		t.Errorf("PacketType should be FIN, got %s", pkt.PacketType())
	}
}

func TestNewFinAckPacket(t *testing.T) {
	sessionID := uuid.New()
	ackNum := uint32(50)
	pkt, err := NewFinAckPacket(sessionID, 5, ackNum)
	if err != nil {
		t.Fatalf("NewFinAckPacket failed: %v", err)
	}

	if !pkt.IsFin() {
		t.Error("Packet should be a FIN")
	}
	if !pkt.IsAck() {
		t.Error("Packet should be an ack")
	}
	if pkt.AckNum != ackNum {
		t.Errorf("AckNum should be %d, got %d", ackNum, pkt.AckNum)
	}
	if pkt.PacketType() != "FIN_ACK" {
		t.Errorf("PacketType should be FIN_ACK, got %s", pkt.PacketType())
	}
}

func TestSetSequenceNumber(t *testing.T) {
	sessionID := uuid.New()
	pkt, _ := NewDataPacket(sessionID, 1, []byte("data"))

	result := pkt.SetSequenceNumber(42)
	if result != pkt {
		t.Error("SetSequenceNumber should return the packet for chaining")
	}
	if pkt.SeqNum != 42 {
		t.Errorf("SeqNum should be 42, got %d", pkt.SeqNum)
	}
}

func TestSetAckNumber(t *testing.T) {
	sessionID := uuid.New()
	pkt, _ := NewDataPacket(sessionID, 1, []byte("data"))

	result := pkt.SetAckNumber(99)
	if result != pkt {
		t.Error("SetAckNumber should return the packet for chaining")
	}
	if pkt.AckNum != 99 {
		t.Errorf("AckNum should be 99, got %d", pkt.AckNum)
	}
}

func TestWithAck(t *testing.T) {
	sessionID := uuid.New()
	pkt, _ := NewDataPacket(sessionID, 1, []byte("data"))

	if pkt.IsAck() {
		t.Error("Packet should not be an ack initially")
	}

	result := pkt.WithAck(100)
	if result != pkt {
		t.Error("WithAck should return the packet for chaining")
	}
	if !pkt.IsAck() {
		t.Error("Packet should be an ack after WithAck")
	}
	if pkt.AckNum != 100 {
		t.Errorf("AckNum should be 100, got %d", pkt.AckNum)
	}
}

func TestIsControlPacket(t *testing.T) {
	sessionID := uuid.New()

	// Control packet (StreamID 0)
	control, _ := NewHandshakePacket(sessionID)
	if !control.IsControlPacket() {
		t.Error("Handshake should be a control packet")
	}

	// Data packet (StreamID > 0)
	data, _ := NewDataPacket(sessionID, 1, []byte("data"))
	if data.IsControlPacket() {
		t.Error("Data packet with StreamID > 0 should not be a control packet")
	}
}

func TestPacketType(t *testing.T) {
	sessionID := uuid.New()

	tests := []struct {
		name     string
		packet   *Packet
		expected string
	}{
		{"handshake", func() *Packet { p, _ := NewHandshakePacket(sessionID); return p }(), "HANDSHAKE"},
		{"handshake_ack", func() *Packet { p, _ := NewHandshakeAckPacket(sessionID); return p }(), "HANDSHAKE_ACK"},
		{"data", func() *Packet { p, _ := NewDataPacket(sessionID, 1, nil); return p }(), "DATA"},
		{"data_ack", func() *Packet { p, _ := NewDataAckPacket(sessionID, 1, nil, 1); return p }(), "DATA_ACK"},
		{"ack", func() *Packet { p, _ := NewAckPacket(sessionID, 1, 1); return p }(), "ACK"},
		{"keepalive", func() *Packet { p, _ := NewKeepAlivePacket(sessionID); return p }(), "KEEPALIVE"},
		{"keepalive_ack", func() *Packet { p, _ := NewKeepAliveAckPacket(sessionID); return p }(), "KEEPALIVE_ACK"},
		{"fin", func() *Packet { p, _ := NewFinPacket(sessionID, 1); return p }(), "FIN"},
		{"fin_ack", func() *Packet { p, _ := NewFinAckPacket(sessionID, 1, 1); return p }(), "FIN_ACK"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.packet.PacketType() != tt.expected {
				t.Errorf("PacketType() = %s, want %s", tt.packet.PacketType(), tt.expected)
			}
		})
	}
}

func TestClone(t *testing.T) {
	sessionID := uuid.New()
	original, _ := NewDataPacket(sessionID, 1, []byte("original"))
	original.SeqNum = 100

	cloned := original.Clone()

	// Verify clone has same values
	if cloned.SessionID != original.SessionID {
		t.Error("SessionID mismatch")
	}
	if cloned.SeqNum != original.SeqNum {
		t.Error("SeqNum mismatch")
	}
	if string(cloned.Payload) != "original" {
		t.Error("Payload mismatch")
	}

	// Verify it's a deep copy
	cloned.Payload[0] = 'X'
	if original.Payload[0] == 'X' {
		t.Error("Modifying clone should not affect original")
	}
}

func TestPacketChaining(t *testing.T) {
	sessionID := uuid.New()
	pkt, _ := NewDataPacket(sessionID, 1, []byte("data"))

	// Test method chaining
	pkt.SetSequenceNumber(10).SetAckNumber(9).WithAck(9)

	if pkt.SeqNum != 10 {
		t.Errorf("SeqNum should be 10, got %d", pkt.SeqNum)
	}
	if pkt.AckNum != 9 {
		t.Errorf("AckNum should be 9, got %d", pkt.AckNum)
	}
	if !pkt.IsAck() {
		t.Error("Packet should be an ack")
	}
}

func TestUnknownPacketType(t *testing.T) {
	sessionID := uuid.New()
	// Create packet with no standard flags
	pkt, _ := NewPacket(sessionID, 0, FlagHMAC, nil)

	if pkt.PacketType() != "UNKNOWN" {
		t.Errorf("PacketType should be UNKNOWN for packet with only HMAC flag, got %s", pkt.PacketType())
	}
}
