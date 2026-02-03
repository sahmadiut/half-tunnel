// Package protocol defines the wire format for split-path communication with UUID correlation.
package protocol

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/google/uuid"
)

// Magic bytes to identify Half-Tunnel packets
const (
	MagicByte1 byte = 0x48 // 'H'
	MagicByte2 byte = 0x54 // 'T'
)

// Protocol version
const Version byte = 0x01

// Packet flags
type Flag byte

const (
	FlagData      Flag = 0x01
	FlagAck       Flag = 0x02
	FlagFin       Flag = 0x04
	FlagKeepAlive Flag = 0x08
	FlagHandshake Flag = 0x10
	FlagHMAC      Flag = 0x80 // Indicates HMAC is present
)

// Header sizes
const (
	HeaderSize     = 2 + 1 + 1 + 16 + 4 + 4 + 4 + 2 // 34 bytes
	HMACSize       = 32
	MaxPayloadSize = 65535
)

// Errors
var (
	ErrInvalidMagic     = errors.New("invalid magic bytes")
	ErrInvalidVersion   = errors.New("unsupported protocol version")
	ErrPayloadTooLarge  = errors.New("payload exceeds maximum size")
	ErrInsufficientData = errors.New("insufficient data for packet")
)

// Packet represents a Half-Tunnel protocol packet.
type Packet struct {
	Magic      [2]byte
	Version    byte
	Flags      Flag
	SessionID  uuid.UUID
	StreamID   uint32
	SeqNum     uint32
	AckNum     uint32
	PayloadLen uint16
	Payload    []byte
	HMAC       []byte // Optional, 32 bytes if FlagHMAC is set
}

// NewPacket creates a new packet with default magic and version.
func NewPacket(sessionID uuid.UUID, streamID uint32, flags Flag, payload []byte) (*Packet, error) {
	if len(payload) > MaxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	return &Packet{
		Magic:      [2]byte{MagicByte1, MagicByte2},
		Version:    Version,
		Flags:      flags,
		SessionID:  sessionID,
		StreamID:   streamID,
		PayloadLen: uint16(len(payload)),
		Payload:    payload,
	}, nil
}

// Marshal serializes the packet to binary format.
func (p *Packet) Marshal() ([]byte, error) {
	size := HeaderSize + int(p.PayloadLen)
	if p.Flags&FlagHMAC != 0 {
		size += HMACSize
	}

	buf := make([]byte, size)
	offset := 0

	// Magic
	buf[offset] = p.Magic[0]
	buf[offset+1] = p.Magic[1]
	offset += 2

	// Version
	buf[offset] = p.Version
	offset++

	// Flags
	buf[offset] = byte(p.Flags)
	offset++

	// SessionID (16 bytes UUID)
	copy(buf[offset:offset+16], p.SessionID[:])
	offset += 16

	// StreamID
	binary.BigEndian.PutUint32(buf[offset:], p.StreamID)
	offset += 4

	// SeqNum
	binary.BigEndian.PutUint32(buf[offset:], p.SeqNum)
	offset += 4

	// AckNum
	binary.BigEndian.PutUint32(buf[offset:], p.AckNum)
	offset += 4

	// PayloadLen
	binary.BigEndian.PutUint16(buf[offset:], p.PayloadLen)
	offset += 2

	// Payload
	copy(buf[offset:offset+int(p.PayloadLen)], p.Payload)
	offset += int(p.PayloadLen)

	// HMAC (optional)
	if p.Flags&FlagHMAC != 0 && len(p.HMAC) == HMACSize {
		copy(buf[offset:], p.HMAC)
	}

	return buf, nil
}

// Unmarshal deserializes binary data into a packet.
func Unmarshal(data []byte) (*Packet, error) {
	if len(data) < HeaderSize {
		return nil, ErrInsufficientData
	}

	p := &Packet{}
	offset := 0

	// Magic
	p.Magic[0] = data[offset]
	p.Magic[1] = data[offset+1]
	if p.Magic[0] != MagicByte1 || p.Magic[1] != MagicByte2 {
		return nil, ErrInvalidMagic
	}
	offset += 2

	// Version
	p.Version = data[offset]
	if p.Version != Version {
		return nil, ErrInvalidVersion
	}
	offset++

	// Flags
	p.Flags = Flag(data[offset])
	offset++

	// SessionID
	copy(p.SessionID[:], data[offset:offset+16])
	offset += 16

	// StreamID
	p.StreamID = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// SeqNum
	p.SeqNum = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// AckNum
	p.AckNum = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// PayloadLen
	p.PayloadLen = binary.BigEndian.Uint16(data[offset:])
	offset += 2

	// Calculate expected total size
	expectedSize := HeaderSize + int(p.PayloadLen)
	if p.Flags&FlagHMAC != 0 {
		expectedSize += HMACSize
	}

	if len(data) < expectedSize {
		return nil, ErrInsufficientData
	}

	// Payload
	if p.PayloadLen > 0 {
		p.Payload = make([]byte, p.PayloadLen)
		copy(p.Payload, data[offset:offset+int(p.PayloadLen)])
		offset += int(p.PayloadLen)
	}

	// HMAC (optional)
	if p.Flags&FlagHMAC != 0 {
		p.HMAC = make([]byte, HMACSize)
		copy(p.HMAC, data[offset:offset+HMACSize])
	}

	return p, nil
}

// ReadPacket reads a packet from an io.Reader.
func ReadPacket(r io.Reader) (*Packet, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	// Parse header to get payload length and flags
	flags := Flag(header[3])
	payloadLen := binary.BigEndian.Uint16(header[32:34])

	// Calculate additional bytes needed
	additionalSize := int(payloadLen)
	if flags&FlagHMAC != 0 {
		additionalSize += HMACSize
	}

	// Read additional data if any
	var fullData []byte
	if additionalSize > 0 {
		additional := make([]byte, additionalSize)
		if _, err := io.ReadFull(r, additional); err != nil {
			return nil, err
		}
		fullData = append(header, additional...)
	} else {
		fullData = header
	}

	return Unmarshal(fullData)
}

// IsData returns true if the packet contains data.
func (p *Packet) IsData() bool {
	return p.Flags&FlagData != 0
}

// IsAck returns true if the packet is an acknowledgment.
func (p *Packet) IsAck() bool {
	return p.Flags&FlagAck != 0
}

// IsFin returns true if the packet signals connection termination.
func (p *Packet) IsFin() bool {
	return p.Flags&FlagFin != 0
}

// IsKeepAlive returns true if the packet is a keep-alive.
func (p *Packet) IsKeepAlive() bool {
	return p.Flags&FlagKeepAlive != 0
}

// IsHandshake returns true if the packet is a handshake.
func (p *Packet) IsHandshake() bool {
	return p.Flags&FlagHandshake != 0
}

// HasHMAC returns true if the packet contains HMAC.
func (p *Packet) HasHMAC() bool {
	return p.Flags&FlagHMAC != 0
}
