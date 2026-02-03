// Package crypto provides encryption utilities for the Half-Tunnel system.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

// Errors
var (
	ErrInvalidKeySize   = errors.New("invalid key size")
	ErrEncryptionFailed = errors.New("encryption failed")
	ErrDecryptionFailed = errors.New("decryption failed")
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrAuthFailed       = errors.New("authentication failed")
)

// KeySize constants
const (
	AES128KeySize = 16
	AES256KeySize = 32
	HMACKeySize   = 32
	NonceSize     = 12 // GCM standard nonce size
)

// AESGCMCipher provides AES-GCM encryption/decryption.
type AESGCMCipher struct {
	aead cipher.AEAD
}

// NewAESGCMCipher creates a new AES-GCM cipher with the given key.
func NewAESGCMCipher(key []byte) (*AESGCMCipher, error) {
	if len(key) != AES128KeySize && len(key) != AES256KeySize {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &AESGCMCipher{aead: aead}, nil
}

// Encrypt encrypts plaintext using AES-GCM.
// Returns: nonce || ciphertext || tag
func (c *AESGCMCipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, ErrEncryptionFailed
	}

	ciphertext := c.aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext that was encrypted with Encrypt.
func (c *AESGCMCipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < c.aead.NonceSize() {
		return nil, ErrInvalidCiphertext
	}

	nonce := ciphertext[:c.aead.NonceSize()]
	ciphertext = ciphertext[c.aead.NonceSize():]

	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// HMAC provides HMAC-SHA256 authentication.
type HMAC struct {
	key []byte
}

// NewHMAC creates a new HMAC with the given key.
func NewHMAC(key []byte) (*HMAC, error) {
	if len(key) < HMACKeySize {
		return nil, ErrInvalidKeySize
	}
	return &HMAC{key: key}, nil
}

// Sign generates an HMAC tag for the given data.
func (h *HMAC) Sign(data []byte) []byte {
	mac := hmac.New(sha256.New, h.key)
	mac.Write(data)
	return mac.Sum(nil)
}

// Verify verifies an HMAC tag for the given data.
func (h *HMAC) Verify(data, tag []byte) bool {
	expected := h.Sign(data)
	return hmac.Equal(expected, tag)
}

// GenerateKey generates a random key of the specified size.
func GenerateKey(size int) ([]byte, error) {
	key := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// GenerateAES256Key generates a random 256-bit AES key.
func GenerateAES256Key() ([]byte, error) {
	return GenerateKey(AES256KeySize)
}

// GenerateHMACKey generates a random HMAC key.
func GenerateHMACKey() ([]byte, error) {
	return GenerateKey(HMACKeySize)
}

// Argon2 parameters (OWASP recommendations)
const (
	Argon2Time    = 3      // Number of iterations
	Argon2Memory  = 64 * 1024 // Memory in KB (64 MB)
	Argon2Threads = 4      // Number of threads
	Argon2KeyLen  = 32     // Output key length
)

// DeriveKey derives a key from a password using Argon2id.
// Argon2id is the recommended KDF for password hashing, resistant to both
// side-channel and GPU attacks.
func DeriveKey(password []byte, salt []byte) []byte {
	return argon2.IDKey(password, salt, Argon2Time, Argon2Memory, Argon2Threads, Argon2KeyLen)
}

// DeriveKeySHA256 derives a key using SHA-256 (fast, for non-password use cases).
// WARNING: Do not use for password-based key derivation - use DeriveKey instead.
func DeriveKeySHA256(data []byte, salt []byte) []byte {
	combined := append(data, salt...)
	hash := sha256.Sum256(combined)
	return hash[:]
}
