package protocol

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/sahmadiut/half-tunnel/pkg/crypto"
)

func TestNewPacketCrypto(t *testing.T) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()

	pc, err := NewPacketCrypto(encKey, hmacKey)
	if err != nil {
		t.Fatalf("NewPacketCrypto failed: %v", err)
	}
	if pc == nil {
		t.Fatal("PacketCrypto should not be nil")
	}
}

func TestNewPacketCryptoInvalidKeys(t *testing.T) {
	tests := []struct {
		name    string
		encKey  []byte
		hmacKey []byte
	}{
		{"invalid encryption key", []byte("short"), make([]byte, 32)},
		{"invalid hmac key", make([]byte, 32), []byte("short")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPacketCrypto(tt.encKey, tt.hmacKey)
			if err == nil {
				t.Error("Expected error for invalid key")
			}
		})
	}
}

func TestNewPacketCryptoEncryptOnly(t *testing.T) {
	encKey, _ := crypto.GenerateAES256Key()

	pc, err := NewPacketCryptoEncryptOnly(encKey)
	if err != nil {
		t.Fatalf("NewPacketCryptoEncryptOnly failed: %v", err)
	}
	if pc == nil {
		t.Fatal("PacketCrypto should not be nil")
	}
	if pc.hmac != nil {
		t.Error("HMAC should be nil for encrypt-only")
	}
}

func TestNewPacketCryptoHMACOnly(t *testing.T) {
	hmacKey, _ := crypto.GenerateHMACKey()

	pc, err := NewPacketCryptoHMACOnly(hmacKey)
	if err != nil {
		t.Fatalf("NewPacketCryptoHMACOnly failed: %v", err)
	}
	if pc == nil {
		t.Fatal("PacketCrypto should not be nil")
	}
	if pc.cipher != nil {
		t.Error("Cipher should be nil for HMAC-only")
	}
}

func TestEncryptDecryptPacket(t *testing.T) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	originalPayload := []byte("Hello, World! This is secret data.")
	original, _ := NewPacket(sessionID, 1, FlagData, originalPayload)
	original.SeqNum = 42

	// Encrypt
	encrypted, err := pc.EncryptPacket(original)
	if err != nil {
		t.Fatalf("EncryptPacket failed: %v", err)
	}

	// Encrypted payload should be different from original
	if bytes.Equal(encrypted.Payload, original.Payload) {
		t.Error("Encrypted payload should differ from original")
	}

	// Encrypted payload should be larger (nonce + tag overhead)
	if len(encrypted.Payload) <= len(original.Payload) {
		t.Error("Encrypted payload should be larger than original")
	}

	// Original should not be modified
	if !bytes.Equal(original.Payload, originalPayload) {
		t.Error("Original packet should not be modified")
	}

	// Decrypt
	decrypted, err := pc.DecryptPacket(encrypted)
	if err != nil {
		t.Fatalf("DecryptPacket failed: %v", err)
	}

	// Decrypted payload should match original
	if !bytes.Equal(decrypted.Payload, originalPayload) {
		t.Errorf("Decrypted payload mismatch: got %s, want %s", decrypted.Payload, originalPayload)
	}

	// Other fields should be preserved
	if decrypted.SessionID != original.SessionID {
		t.Error("SessionID mismatch")
	}
	if decrypted.StreamID != original.StreamID {
		t.Error("StreamID mismatch")
	}
	if decrypted.SeqNum != original.SeqNum {
		t.Error("SeqNum mismatch")
	}
}

func TestEncryptPacketEmptyPayload(t *testing.T) {
	encKey, _ := crypto.GenerateAES256Key()
	pc, _ := NewPacketCryptoEncryptOnly(encKey)

	sessionID := uuid.New()
	original, _ := NewPacket(sessionID, 0, FlagKeepAlive, nil)

	encrypted, err := pc.EncryptPacket(original)
	if err != nil {
		t.Fatalf("EncryptPacket failed: %v", err)
	}

	if len(encrypted.Payload) != 0 {
		t.Error("Encrypted packet with no payload should have no payload")
	}
}

func TestEncryptPacketNoCipher(t *testing.T) {
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCryptoHMACOnly(hmacKey)

	sessionID := uuid.New()
	payload := []byte("test payload")
	original, _ := NewPacket(sessionID, 1, FlagData, payload)

	encrypted, err := pc.EncryptPacket(original)
	if err != nil {
		t.Fatalf("EncryptPacket failed: %v", err)
	}

	// Without cipher, payload should be unchanged
	if !bytes.Equal(encrypted.Payload, payload) {
		t.Error("Payload should be unchanged without cipher")
	}
}

func TestSignVerifyPacket(t *testing.T) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	original, _ := NewPacket(sessionID, 1, FlagData, []byte("Authenticated data"))
	original.SeqNum = 100

	// Sign
	signed, err := pc.SignPacket(original)
	if err != nil {
		t.Fatalf("SignPacket failed: %v", err)
	}

	// Should have HMAC flag set
	if !signed.HasHMAC() {
		t.Error("Signed packet should have HMAC flag")
	}

	// Should have HMAC data
	if len(signed.HMAC) != HMACSize {
		t.Errorf("HMAC size should be %d, got %d", HMACSize, len(signed.HMAC))
	}

	// Verify should succeed
	if !pc.VerifyPacket(signed) {
		t.Error("Verification should succeed for signed packet")
	}

	// Tampering should fail verification
	tampered := signed.Clone()
	tampered.Payload[0] ^= 0xFF
	if pc.VerifyPacket(tampered) {
		t.Error("Verification should fail for tampered packet")
	}

	// Wrong HMAC should fail
	wrongHMAC := signed.Clone()
	wrongHMAC.HMAC[0] ^= 0xFF
	if pc.VerifyPacket(wrongHMAC) {
		t.Error("Verification should fail for wrong HMAC")
	}
}

func TestVerifyPacketNoHMAC(t *testing.T) {
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCryptoHMACOnly(hmacKey)

	sessionID := uuid.New()
	pkt, _ := NewPacket(sessionID, 1, FlagData, []byte("data"))

	// Packet without HMAC should still verify (HMAC not required)
	if !pc.VerifyPacket(pkt) {
		t.Error("Packet without HMAC should verify when HMAC not required")
	}
}

func TestSignPacketNoHMAC(t *testing.T) {
	encKey, _ := crypto.GenerateAES256Key()
	pc, _ := NewPacketCryptoEncryptOnly(encKey)

	sessionID := uuid.New()
	original, _ := NewPacket(sessionID, 1, FlagData, []byte("data"))

	signed, err := pc.SignPacket(original)
	if err != nil {
		t.Fatalf("SignPacket failed: %v", err)
	}

	// Without HMAC configured, should not add HMAC
	if signed.HasHMAC() {
		t.Error("Packet should not have HMAC without HMAC key configured")
	}
}

func TestEncryptAndSign(t *testing.T) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	originalPayload := []byte("Secret and authenticated data")
	original, _ := NewPacket(sessionID, 1, FlagData, originalPayload)

	// Encrypt and sign
	secured, err := pc.EncryptAndSign(original)
	if err != nil {
		t.Fatalf("EncryptAndSign failed: %v", err)
	}

	// Should be encrypted
	if bytes.Equal(secured.Payload, originalPayload) {
		t.Error("Payload should be encrypted")
	}

	// Should have HMAC
	if !secured.HasHMAC() {
		t.Error("Packet should have HMAC")
	}

	// Should verify
	if !pc.VerifyPacket(secured) {
		t.Error("Packet should verify")
	}
}

func TestVerifyAndDecrypt(t *testing.T) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	originalPayload := []byte("Secret and authenticated data")
	original, _ := NewPacket(sessionID, 1, FlagData, originalPayload)

	// Encrypt and sign
	secured, _ := pc.EncryptAndSign(original)

	// Verify and decrypt
	decrypted, err := pc.VerifyAndDecrypt(secured)
	if err != nil {
		t.Fatalf("VerifyAndDecrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted.Payload, originalPayload) {
		t.Errorf("Decrypted payload mismatch: got %s, want %s", decrypted.Payload, originalPayload)
	}
}

func TestVerifyAndDecryptTampered(t *testing.T) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	original, _ := NewPacket(sessionID, 1, FlagData, []byte("data"))

	// Encrypt and sign
	secured, _ := pc.EncryptAndSign(original)

	// Tamper with data
	secured.Payload[0] ^= 0xFF

	// Verify and decrypt should fail
	_, err := pc.VerifyAndDecrypt(secured)
	if err == nil {
		t.Error("VerifyAndDecrypt should fail for tampered packet")
	}
	if err != ErrHMACVerificationFailed {
		t.Errorf("Expected ErrHMACVerificationFailed, got %v", err)
	}
}

func TestCopyPacket(t *testing.T) {
	sessionID := uuid.New()
	original, _ := NewPacket(sessionID, 1, FlagData, []byte("test data"))
	original.SeqNum = 42
	original.AckNum = 41
	original.HMAC = make([]byte, HMACSize)
	for i := range original.HMAC {
		original.HMAC[i] = byte(i)
	}

	copied := copyPacket(original)

	// Verify all fields are copied
	if copied.Magic != original.Magic {
		t.Error("Magic mismatch")
	}
	if copied.Version != original.Version {
		t.Error("Version mismatch")
	}
	if copied.Flags != original.Flags {
		t.Error("Flags mismatch")
	}
	if copied.SessionID != original.SessionID {
		t.Error("SessionID mismatch")
	}
	if copied.StreamID != original.StreamID {
		t.Error("StreamID mismatch")
	}
	if copied.SeqNum != original.SeqNum {
		t.Error("SeqNum mismatch")
	}
	if copied.AckNum != original.AckNum {
		t.Error("AckNum mismatch")
	}
	if !bytes.Equal(copied.Payload, original.Payload) {
		t.Error("Payload mismatch")
	}
	if !bytes.Equal(copied.HMAC, original.HMAC) {
		t.Error("HMAC mismatch")
	}

	// Verify it's a deep copy (modifying copy doesn't affect original)
	copied.Payload[0] = 0xFF
	if original.Payload[0] == 0xFF {
		t.Error("Modifying copy should not affect original (not a deep copy)")
	}
}

func TestMarshalWithoutHMAC(t *testing.T) {
	sessionID := uuid.New()
	pkt, _ := NewPacket(sessionID, 1, FlagData|FlagHMAC, []byte("test"))
	pkt.SeqNum = 100
	pkt.AckNum = 99

	data, err := marshalWithoutHMAC(pkt)
	if err != nil {
		t.Fatalf("marshalWithoutHMAC failed: %v", err)
	}

	expectedSize := HeaderSize + len(pkt.Payload)
	if len(data) != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, len(data))
	}

	// Verify the data contains the correct header fields
	// Magic bytes
	if data[0] != MagicByte1 || data[1] != MagicByte2 {
		t.Error("Magic bytes mismatch")
	}

	// Version
	if data[2] != Version {
		t.Error("Version mismatch")
	}

	// Flags should include HMAC flag
	if Flag(data[3])&FlagHMAC == 0 {
		t.Error("HMAC flag should be set")
	}

	// To fully test, we'd need to parse without expecting HMAC
	// For now, verify the SessionID is at the correct offset (bytes 4-19)
	var parsedSessionID uuid.UUID
	copy(parsedSessionID[:], data[4:20])
	if parsedSessionID != sessionID {
		t.Error("SessionID mismatch in marshaled data")
	}
}

func BenchmarkEncryptPacket(b *testing.B) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	payload := make([]byte, 1024)
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pc.EncryptPacket(pkt)
	}
}

func BenchmarkDecryptPacket(b *testing.B) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	payload := make([]byte, 1024)
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)
	encrypted, _ := pc.EncryptPacket(pkt)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pc.DecryptPacket(encrypted)
	}
}

func BenchmarkSignPacket(b *testing.B) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	payload := make([]byte, 1024)
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pc.SignPacket(pkt)
	}
}

func BenchmarkVerifyPacket(b *testing.B) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	payload := make([]byte, 1024)
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)
	signed, _ := pc.SignPacket(pkt)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pc.VerifyPacket(signed)
	}
}

func BenchmarkEncryptAndSign(b *testing.B) {
	encKey, _ := crypto.GenerateAES256Key()
	hmacKey, _ := crypto.GenerateHMACKey()
	pc, _ := NewPacketCrypto(encKey, hmacKey)

	sessionID := uuid.New()
	payload := make([]byte, 1024)
	pkt, _ := NewPacket(sessionID, 1, FlagData, payload)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pc.EncryptAndSign(pkt)
	}
}
