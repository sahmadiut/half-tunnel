package crypto

import (
	"bytes"
	"testing"
)

func TestAESGCMCipher(t *testing.T) {
	key, err := GenerateAES256Key()
	if err != nil {
		t.Fatalf("GenerateAES256Key failed: %v", err)
	}

	cipher, err := NewAESGCMCipher(key)
	if err != nil {
		t.Fatalf("NewAESGCMCipher failed: %v", err)
	}

	plaintext := []byte("Hello, World!")

	// Encrypt
	ciphertext, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Ciphertext should be larger than plaintext (nonce + tag)
	if len(ciphertext) <= len(plaintext) {
		t.Error("Ciphertext should be larger than plaintext")
	}

	// Decrypt
	decrypted, err := cipher.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypted text mismatch: got %s, want %s", decrypted, plaintext)
	}
}

func TestAESGCMCipherInvalidKeySize(t *testing.T) {
	_, err := NewAESGCMCipher([]byte("short"))
	if err != ErrInvalidKeySize {
		t.Errorf("Expected ErrInvalidKeySize, got %v", err)
	}
}

func TestAESGCMCipherDecryptInvalid(t *testing.T) {
	key, _ := GenerateAES256Key()
	cipher, _ := NewAESGCMCipher(key)

	// Too short ciphertext
	_, err := cipher.Decrypt([]byte("short"))
	if err != ErrInvalidCiphertext {
		t.Errorf("Expected ErrInvalidCiphertext, got %v", err)
	}

	// Corrupted ciphertext
	plaintext := []byte("test")
	ciphertext, _ := cipher.Encrypt(plaintext)
	ciphertext[len(ciphertext)-1] ^= 0xFF // Corrupt last byte
	_, err = cipher.Decrypt(ciphertext)
	if err != ErrDecryptionFailed {
		t.Errorf("Expected ErrDecryptionFailed, got %v", err)
	}
}

func TestHMAC(t *testing.T) {
	key, err := GenerateHMACKey()
	if err != nil {
		t.Fatalf("GenerateHMACKey failed: %v", err)
	}

	hmac, err := NewHMAC(key)
	if err != nil {
		t.Fatalf("NewHMAC failed: %v", err)
	}

	data := []byte("Message to authenticate")

	// Sign
	tag := hmac.Sign(data)
	if len(tag) != 32 { // SHA256 output
		t.Errorf("Tag length should be 32, got %d", len(tag))
	}

	// Verify valid
	if !hmac.Verify(data, tag) {
		t.Error("Valid tag should verify")
	}

	// Verify invalid
	tag[0] ^= 0xFF
	if hmac.Verify(data, tag) {
		t.Error("Invalid tag should not verify")
	}
}

func TestHMACInvalidKeySize(t *testing.T) {
	_, err := NewHMAC([]byte("short"))
	if err != ErrInvalidKeySize {
		t.Errorf("Expected ErrInvalidKeySize, got %v", err)
	}
}

func TestGenerateKey(t *testing.T) {
	key1, err := GenerateKey(32)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("Key length should be 32, got %d", len(key1))
	}

	key2, _ := GenerateKey(32)
	if bytes.Equal(key1, key2) {
		t.Error("Two generated keys should be different")
	}
}

func TestDeriveKey(t *testing.T) {
	password := []byte("password123")
	salt := []byte("somesalt12345678") // 16 bytes minimum for Argon2

	key1 := DeriveKey(password, salt)
	if len(key1) != 32 { // Argon2 output
		t.Errorf("Derived key length should be 32, got %d", len(key1))
	}

	// Same inputs should produce same key
	key2 := DeriveKey(password, salt)
	if !bytes.Equal(key1, key2) {
		t.Error("Same inputs should produce same key")
	}

	// Different salt should produce different key
	key3 := DeriveKey(password, []byte("othersalt1234567"))
	if bytes.Equal(key1, key3) {
		t.Error("Different salt should produce different key")
	}
}

func TestDeriveKeySHA256(t *testing.T) {
	data := []byte("some data")
	salt := []byte("salt")

	key1 := DeriveKeySHA256(data, salt)
	if len(key1) != 32 {
		t.Errorf("Derived key length should be 32, got %d", len(key1))
	}

	// Same inputs should produce same key
	key2 := DeriveKeySHA256(data, salt)
	if !bytes.Equal(key1, key2) {
		t.Error("Same inputs should produce same key")
	}
}

func BenchmarkAESGCMEncrypt(b *testing.B) {
	key, _ := GenerateAES256Key()
	cipher, _ := NewAESGCMCipher(key)
	plaintext := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cipher.Encrypt(plaintext)
	}
}

func BenchmarkAESGCMDecrypt(b *testing.B) {
	key, _ := GenerateAES256Key()
	cipher, _ := NewAESGCMCipher(key)
	plaintext := make([]byte, 1024)
	ciphertext, _ := cipher.Encrypt(plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cipher.Decrypt(ciphertext)
	}
}
