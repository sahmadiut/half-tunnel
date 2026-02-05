package protocol

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
)

func TestNewPacket(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("test payload")

	pkt, err := NewPacket(sessionID, 1, FlagData, payload)
	if err != nil {
		t.Fatalf("NewPacket failed: %v", err)
	}

	if pkt.Magic[0] != MagicByte1 || pkt.Magic[1] != MagicByte2 {
		t.Errorf("Invalid magic bytes: got %v, want [%v, %v]", pkt.Magic, MagicByte1, MagicByte2)
	}

	if pkt.Version != Version {
		t.Errorf("Invalid version: got %v, want %v", pkt.Version, Version)
	}

	if pkt.SessionID != sessionID {
		t.Errorf("Invalid session ID: got %v, want %v", pkt.SessionID, sessionID)
	}

	if pkt.StreamID != 1 {
		t.Errorf("Invalid stream ID: got %v, want 1", pkt.StreamID)
	}

	if pkt.Flags != FlagData {
		t.Errorf("Invalid flags: got %v, want %v", pkt.Flags, FlagData)
	}

	if !bytes.Equal(pkt.Payload, payload) {
		t.Errorf("Invalid payload: got %v, want %v", pkt.Payload, payload)
	}
}

func TestNewPacketPayloadTooLarge(t *testing.T) {
	sessionID := uuid.New()
	payload := make([]byte, MaxPayloadSize+1)

	_, err := NewPacket(sessionID, 1, FlagData, payload)
	if err != ErrPayloadTooLarge {
		t.Errorf("Expected ErrPayloadTooLarge, got %v", err)
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("Hello, World!")

	original, err := NewPacket(sessionID, 42, FlagData|FlagAck, payload)
	if err != nil {
		t.Fatalf("NewPacket failed: %v", err)
	}
	original.SeqNum = 100
	original.AckNum = 99

	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	expectedSize := HeaderSize + len(payload)
	if len(data) != expectedSize {
		t.Errorf("Invalid marshal size: got %v, want %v", len(data), expectedSize)
	}

	restored, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Magic != original.Magic {
		t.Errorf("Magic mismatch: got %v, want %v", restored.Magic, original.Magic)
	}
	if restored.Version != original.Version {
		t.Errorf("Version mismatch: got %v, want %v", restored.Version, original.Version)
	}
	if restored.Flags != original.Flags {
		t.Errorf("Flags mismatch: got %v, want %v", restored.Flags, original.Flags)
	}
	if restored.SessionID != original.SessionID {
		t.Errorf("SessionID mismatch: got %v, want %v", restored.SessionID, original.SessionID)
	}
	if restored.StreamID != original.StreamID {
		t.Errorf("StreamID mismatch: got %v, want %v", restored.StreamID, original.StreamID)
	}
	if restored.SeqNum != original.SeqNum {
		t.Errorf("SeqNum mismatch: got %v, want %v", restored.SeqNum, original.SeqNum)
	}
	if restored.AckNum != original.AckNum {
		t.Errorf("AckNum mismatch: got %v, want %v", restored.AckNum, original.AckNum)
	}
	if restored.PayloadLen != original.PayloadLen {
		t.Errorf("PayloadLen mismatch: got %v, want %v", restored.PayloadLen, original.PayloadLen)
	}
	if !bytes.Equal(restored.Payload, original.Payload) {
		t.Errorf("Payload mismatch: got %v, want %v", restored.Payload, original.Payload)
	}
}

func TestMarshalUnmarshalWithHMAC(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("Authenticated data")

	original, err := NewPacket(sessionID, 1, FlagData|FlagHMAC, payload)
	if err != nil {
		t.Fatalf("NewPacket failed: %v", err)
	}
	original.HMAC = make([]byte, HMACSize)
	for i := range original.HMAC {
		original.HMAC[i] = byte(i)
	}

	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	expectedSize := HeaderSize + len(payload) + HMACSize
	if len(data) != expectedSize {
		t.Errorf("Invalid marshal size: got %v, want %v", len(data), expectedSize)
	}

	restored, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !restored.HasHMAC() {
		t.Error("Expected HasHMAC to be true")
	}
	if !bytes.Equal(restored.HMAC, original.HMAC) {
		t.Errorf("HMAC mismatch: got %v, want %v", restored.HMAC, original.HMAC)
	}
}

func TestUnmarshalErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "insufficient data",
			data:    make([]byte, HeaderSize-1),
			wantErr: ErrInsufficientData,
		},
		{
			name: "invalid magic",
			data: func() []byte {
				d := make([]byte, HeaderSize)
				d[0] = 0xFF
				d[1] = 0xFF
				return d
			}(),
			wantErr: ErrInvalidMagic,
		},
		{
			name: "invalid version",
			data: func() []byte {
				d := make([]byte, HeaderSize)
				d[0] = MagicByte1
				d[1] = MagicByte2
				d[2] = 0xFF
				return d
			}(),
			wantErr: ErrInvalidVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Unmarshal(tt.data)
			if err != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPacketFlags(t *testing.T) {
	sessionID := uuid.New()

	tests := []struct {
		flag      Flag
		isData    bool
		isAck     bool
		isFin     bool
		isKA      bool
		isHS      bool
		isReconn  bool
		hasHMAC   bool
	}{
		{FlagData, true, false, false, false, false, false, false},
		{FlagAck, false, true, false, false, false, false, false},
		{FlagFin, false, false, true, false, false, false, false},
		{FlagKeepAlive, false, false, false, true, false, false, false},
		{FlagHandshake, false, false, false, false, true, false, false},
		{FlagReconnect, false, false, false, false, false, true, false},
		{FlagHMAC, false, false, false, false, false, false, true},
		{FlagData | FlagAck, true, true, false, false, false, false, false},
		{FlagData | FlagHMAC, true, false, false, false, false, false, true},
		{FlagReconnect | FlagHandshake, false, false, false, false, true, true, false},
	}

	for _, tt := range tests {
		pkt, _ := NewPacket(sessionID, 1, tt.flag, nil)
		if pkt.IsData() != tt.isData {
			t.Errorf("Flag %v: IsData() = %v, want %v", tt.flag, pkt.IsData(), tt.isData)
		}
		if pkt.IsAck() != tt.isAck {
			t.Errorf("Flag %v: IsAck() = %v, want %v", tt.flag, pkt.IsAck(), tt.isAck)
		}
		if pkt.IsFin() != tt.isFin {
			t.Errorf("Flag %v: IsFin() = %v, want %v", tt.flag, pkt.IsFin(), tt.isFin)
		}
		if pkt.IsKeepAlive() != tt.isKA {
			t.Errorf("Flag %v: IsKeepAlive() = %v, want %v", tt.flag, pkt.IsKeepAlive(), tt.isKA)
		}
		if pkt.IsHandshake() != tt.isHS {
			t.Errorf("Flag %v: IsHandshake() = %v, want %v", tt.flag, pkt.IsHandshake(), tt.isHS)
		}
		if pkt.IsReconnect() != tt.isReconn {
			t.Errorf("Flag %v: IsReconnect() = %v, want %v", tt.flag, pkt.IsReconnect(), tt.isReconn)
		}
		if pkt.HasHMAC() != tt.hasHMAC {
			t.Errorf("Flag %v: HasHMAC() = %v, want %v", tt.flag, pkt.HasHMAC(), tt.hasHMAC)
		}
	}
}

func TestReadPacket(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("test packet read")

	original, err := NewPacket(sessionID, 5, FlagData, payload)
	if err != nil {
		t.Fatalf("NewPacket failed: %v", err)
	}
	original.SeqNum = 42

	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	reader := bytes.NewReader(data)
	restored, err := ReadPacket(reader)
	if err != nil {
		t.Fatalf("ReadPacket failed: %v", err)
	}

	if restored.SessionID != original.SessionID {
		t.Errorf("SessionID mismatch")
	}
	if restored.StreamID != original.StreamID {
		t.Errorf("StreamID mismatch")
	}
	if restored.SeqNum != original.SeqNum {
		t.Errorf("SeqNum mismatch: got %v, want %v", restored.SeqNum, original.SeqNum)
	}
	if !bytes.Equal(restored.Payload, original.Payload) {
		t.Errorf("Payload mismatch")
	}
}

func TestEmptyPayload(t *testing.T) {
	sessionID := uuid.New()

	pkt, err := NewPacket(sessionID, 1, FlagKeepAlive, nil)
	if err != nil {
		t.Fatalf("NewPacket failed: %v", err)
	}

	data, err := pkt.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if len(data) != HeaderSize {
		t.Errorf("Expected size %d, got %d", HeaderSize, len(data))
	}

	restored, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.PayloadLen != 0 {
		t.Errorf("Expected PayloadLen 0, got %d", restored.PayloadLen)
	}
	if len(restored.Payload) != 0 {
		t.Errorf("Expected empty payload, got %v", restored.Payload)
	}
}

func BenchmarkMarshal(b *testing.B) {
	sessionID := uuid.New()
	payload := make([]byte, 1024)
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pkt.Marshal()
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	sessionID := uuid.New()
	payload := make([]byte, 1024)
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)
	data, _ := pkt.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Unmarshal(data)
	}
}

// Tests for Phase 3 checksum functionality

func TestPacket_CalculateChecksum(t *testing.T) {
	sessionID := uuid.New()

	// Test with payload
	payload := []byte("hello world")
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)

	checksum := pkt.CalculateChecksum()
	if checksum == 0 {
		t.Error("expected non-zero checksum for non-empty payload")
	}

	// Same payload should produce same checksum
	checksum2 := pkt.CalculateChecksum()
	if checksum != checksum2 {
		t.Error("expected same checksum for same payload")
	}

	// Different payload should produce different checksum
	pkt2, _ := NewPacket(sessionID, 1, FlagData, []byte("different"))
	checksum3 := pkt2.CalculateChecksum()
	if checksum == checksum3 {
		t.Error("expected different checksum for different payload")
	}
}

func TestPacket_CalculateChecksumEmpty(t *testing.T) {
	sessionID := uuid.New()

	// Empty payload should return 0
	pkt, _ := NewPacket(sessionID, 1, FlagKeepAlive, nil)
	checksum := pkt.CalculateChecksum()
	if checksum != 0 {
		t.Errorf("expected checksum 0 for empty payload, got %d", checksum)
	}
}

func TestPacket_VerifyChecksum(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("test data for checksum")

	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)
	checksum := pkt.CalculateChecksum()

	// Verify correct checksum
	if !pkt.VerifyChecksum(checksum) {
		t.Error("expected checksum verification to pass")
	}

	// Verify wrong checksum
	if pkt.VerifyChecksum(checksum + 1) {
		t.Error("expected checksum verification to fail with wrong checksum")
	}
}

func TestPacket_CalculateHeaderChecksum(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("test payload")

	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)
	pkt.SeqNum = 100

	headerChecksum := pkt.CalculateHeaderChecksum()
	if headerChecksum == 0 {
		t.Error("expected non-zero header checksum")
	}

	// Same packet should produce same checksum
	headerChecksum2 := pkt.CalculateHeaderChecksum()
	if headerChecksum != headerChecksum2 {
		t.Error("expected same header checksum for same packet")
	}

	// Different stream ID should produce different checksum
	pkt2, _ := NewPacket(sessionID, 2, FlagData, payload) // Different stream ID
	pkt2.SeqNum = 100
	headerChecksum3 := pkt2.CalculateHeaderChecksum()
	if headerChecksum == headerChecksum3 {
		t.Error("expected different header checksum for different stream ID")
	}

	// Different seq num should produce different checksum
	pkt3, _ := NewPacket(sessionID, 1, FlagData, payload)
	pkt3.SeqNum = 200 // Different sequence number
	headerChecksum4 := pkt3.CalculateHeaderChecksum()
	if headerChecksum == headerChecksum4 {
		t.Error("expected different header checksum for different sequence number")
	}
}

func TestPacket_VerifyHeaderChecksum(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("test data")

	pkt, _ := NewPacket(sessionID, 5, FlagData, payload)
	pkt.SeqNum = 42

	headerChecksum := pkt.CalculateHeaderChecksum()

	// Verify correct checksum
	if !pkt.VerifyHeaderChecksum(headerChecksum) {
		t.Error("expected header checksum verification to pass")
	}

	// Verify wrong checksum
	if pkt.VerifyHeaderChecksum(headerChecksum ^ 0x12345678) {
		t.Error("expected header checksum verification to fail with wrong checksum")
	}
}

func TestPacket_ChecksumAfterModification(t *testing.T) {
	sessionID := uuid.New()
	payload := []byte("original")

	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)
	originalChecksum := pkt.CalculateChecksum()

	// Modify payload
	pkt.Payload = []byte("modified")
	pkt.PayloadLen = uint16(len(pkt.Payload))

	modifiedChecksum := pkt.CalculateChecksum()

	if originalChecksum == modifiedChecksum {
		t.Error("expected checksum to change after payload modification")
	}

	// Verify original checksum no longer works
	if pkt.VerifyChecksum(originalChecksum) {
		t.Error("expected original checksum to fail after modification")
	}
}

func BenchmarkCalculateChecksum(b *testing.B) {
	sessionID := uuid.New()
	payload := make([]byte, 1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pkt.CalculateChecksum()
	}
}

func BenchmarkCalculateHeaderChecksum(b *testing.B) {
	sessionID := uuid.New()
	payload := make([]byte, 1024)
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)
	pkt.SeqNum = 12345

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pkt.CalculateHeaderChecksum()
	}
}
