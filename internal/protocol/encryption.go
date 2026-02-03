// Package protocol defines the wire format for split-path communication with UUID correlation.
package protocol

import (
	"github.com/sahmadiut/half-tunnel/pkg/crypto"
)

// PacketCrypto provides encryption and authentication for packets.
type PacketCrypto struct {
	cipher *crypto.AESGCMCipher
	hmac   *crypto.HMAC
}

// NewPacketCrypto creates a new PacketCrypto with the given encryption and HMAC keys.
// encryptionKey should be 16 or 32 bytes for AES-128 or AES-256.
// hmacKey should be at least 32 bytes.
func NewPacketCrypto(encryptionKey, hmacKey []byte) (*PacketCrypto, error) {
	cipher, err := crypto.NewAESGCMCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	hmac, err := crypto.NewHMAC(hmacKey)
	if err != nil {
		return nil, err
	}

	return &PacketCrypto{
		cipher: cipher,
		hmac:   hmac,
	}, nil
}

// NewPacketCryptoEncryptOnly creates a PacketCrypto with only encryption (no HMAC).
func NewPacketCryptoEncryptOnly(encryptionKey []byte) (*PacketCrypto, error) {
	cipher, err := crypto.NewAESGCMCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	return &PacketCrypto{
		cipher: cipher,
	}, nil
}

// NewPacketCryptoHMACOnly creates a PacketCrypto with only HMAC (no encryption).
func NewPacketCryptoHMACOnly(hmacKey []byte) (*PacketCrypto, error) {
	hmac, err := crypto.NewHMAC(hmacKey)
	if err != nil {
		return nil, err
	}

	return &PacketCrypto{
		hmac: hmac,
	}, nil
}

// EncryptPacket encrypts the packet's payload and returns a new packet with encrypted payload.
// The original packet is not modified.
func (pc *PacketCrypto) EncryptPacket(p *Packet) (*Packet, error) {
	if pc.cipher == nil {
		// No encryption configured, return copy of original
		return copyPacket(p), nil
	}

	if len(p.Payload) == 0 {
		// No payload to encrypt
		return copyPacket(p), nil
	}

	encryptedPayload, err := pc.cipher.Encrypt(p.Payload)
	if err != nil {
		return nil, err
	}

	// Create new packet with encrypted payload
	encrypted := copyPacket(p)
	encrypted.Payload = encryptedPayload
	encrypted.PayloadLen = uint16(len(encryptedPayload))

	return encrypted, nil
}

// DecryptPacket decrypts the packet's payload and returns a new packet with decrypted payload.
// The original packet is not modified.
func (pc *PacketCrypto) DecryptPacket(p *Packet) (*Packet, error) {
	if pc.cipher == nil {
		// No encryption configured, return copy of original
		return copyPacket(p), nil
	}

	if len(p.Payload) == 0 {
		// No payload to decrypt
		return copyPacket(p), nil
	}

	decryptedPayload, err := pc.cipher.Decrypt(p.Payload)
	if err != nil {
		return nil, err
	}

	// Create new packet with decrypted payload
	decrypted := copyPacket(p)
	decrypted.Payload = decryptedPayload
	decrypted.PayloadLen = uint16(len(decryptedPayload))

	return decrypted, nil
}

// SignPacket adds HMAC authentication to the packet.
// The HMAC is computed over the entire packet (header + payload) and stored in the HMAC field.
// The FlagHMAC is set to indicate the presence of HMAC.
func (pc *PacketCrypto) SignPacket(p *Packet) (*Packet, error) {
	if pc.hmac == nil {
		// No HMAC configured, return copy of original
		return copyPacket(p), nil
	}

	// Create signed packet
	signed := copyPacket(p)
	signed.Flags |= FlagHMAC

	// Marshal packet without HMAC to get data to sign
	// Temporarily set HMAC to nil for marshaling
	signed.HMAC = nil

	// Get header + payload data for signing
	dataToSign, err := marshalWithoutHMAC(signed)
	if err != nil {
		return nil, err
	}

	// Compute HMAC
	signed.HMAC = pc.hmac.Sign(dataToSign)

	return signed, nil
}

// VerifyPacket verifies the HMAC of the packet.
// Returns true if the HMAC is valid or if no HMAC is present.
func (pc *PacketCrypto) VerifyPacket(p *Packet) bool {
	if pc.hmac == nil {
		// No HMAC verification configured
		return true
	}

	if !p.HasHMAC() {
		// No HMAC present in packet
		return true
	}

	if len(p.HMAC) != HMACSize {
		return false
	}

	// Get data without HMAC for verification
	dataToVerify, err := marshalWithoutHMAC(p)
	if err != nil {
		return false
	}

	return pc.hmac.Verify(dataToVerify, p.HMAC)
}

// EncryptAndSign encrypts the payload and signs the packet.
// This is a convenience method that combines EncryptPacket and SignPacket.
func (pc *PacketCrypto) EncryptAndSign(p *Packet) (*Packet, error) {
	encrypted, err := pc.EncryptPacket(p)
	if err != nil {
		return nil, err
	}

	return pc.SignPacket(encrypted)
}

// VerifyAndDecrypt verifies the packet signature and decrypts the payload.
// Returns an error if verification fails.
func (pc *PacketCrypto) VerifyAndDecrypt(p *Packet) (*Packet, error) {
	if !pc.VerifyPacket(p) {
		return nil, ErrHMACVerificationFailed
	}

	return pc.DecryptPacket(p)
}

// ErrHMACVerificationFailed is returned when HMAC verification fails.
var ErrHMACVerificationFailed = errHMACVerificationFailed{}

type errHMACVerificationFailed struct{}

func (errHMACVerificationFailed) Error() string {
	return "HMAC verification failed"
}

// copyPacket creates a deep copy of a packet.
func copyPacket(p *Packet) *Packet {
	newPacket := &Packet{
		Magic:      p.Magic,
		Version:    p.Version,
		Flags:      p.Flags,
		SessionID:  p.SessionID,
		StreamID:   p.StreamID,
		SeqNum:     p.SeqNum,
		AckNum:     p.AckNum,
		PayloadLen: p.PayloadLen,
	}

	if len(p.Payload) > 0 {
		newPacket.Payload = make([]byte, len(p.Payload))
		copy(newPacket.Payload, p.Payload)
	}

	if len(p.HMAC) > 0 {
		newPacket.HMAC = make([]byte, len(p.HMAC))
		copy(newPacket.HMAC, p.HMAC)
	}

	return newPacket
}

// marshalWithoutHMAC marshals the packet header and payload without the HMAC field.
// This is used for computing HMAC signatures.
func marshalWithoutHMAC(p *Packet) ([]byte, error) {
	size := HeaderSize + int(p.PayloadLen)
	buf := make([]byte, size)
	offset := 0

	// Magic
	buf[offset] = p.Magic[0]
	buf[offset+1] = p.Magic[1]
	offset += 2

	// Version
	buf[offset] = p.Version
	offset++

	// Flags (keep HMAC flag set to indicate signature is present)
	buf[offset] = byte(p.Flags)
	offset++

	// SessionID
	copy(buf[offset:offset+16], p.SessionID[:])
	offset += 16

	// StreamID
	buf[offset] = byte(p.StreamID >> 24)
	buf[offset+1] = byte(p.StreamID >> 16)
	buf[offset+2] = byte(p.StreamID >> 8)
	buf[offset+3] = byte(p.StreamID)
	offset += 4

	// SeqNum
	buf[offset] = byte(p.SeqNum >> 24)
	buf[offset+1] = byte(p.SeqNum >> 16)
	buf[offset+2] = byte(p.SeqNum >> 8)
	buf[offset+3] = byte(p.SeqNum)
	offset += 4

	// AckNum
	buf[offset] = byte(p.AckNum >> 24)
	buf[offset+1] = byte(p.AckNum >> 16)
	buf[offset+2] = byte(p.AckNum >> 8)
	buf[offset+3] = byte(p.AckNum)
	offset += 4

	// PayloadLen
	buf[offset] = byte(p.PayloadLen >> 8)
	buf[offset+1] = byte(p.PayloadLen)
	offset += 2

	// Payload
	if len(p.Payload) > 0 {
		copy(buf[offset:], p.Payload)
	}

	return buf, nil
}
