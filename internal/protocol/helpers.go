// Package protocol defines the wire format for split-path communication with UUID correlation.
package protocol

import (
	"github.com/google/uuid"
)

// NewHandshakePacket creates a new handshake packet for session establishment.
// This is sent from client to server via the upstream path.
func NewHandshakePacket(sessionID uuid.UUID) (*Packet, error) {
	return NewPacket(sessionID, 0, FlagHandshake, nil)
}

// NewHandshakeAckPacket creates a handshake acknowledgment packet.
// This is sent from server to client via the downstream path to confirm session establishment.
func NewHandshakeAckPacket(sessionID uuid.UUID) (*Packet, error) {
	pkt, err := NewPacket(sessionID, 0, FlagHandshake|FlagAck, nil)
	if err != nil {
		return nil, err
	}
	return pkt, nil
}

// NewDataPacket creates a new data packet for a specific stream.
func NewDataPacket(sessionID uuid.UUID, streamID uint32, payload []byte) (*Packet, error) {
	return NewPacket(sessionID, streamID, FlagData, payload)
}

// NewDataAckPacket creates a data packet that also acknowledges received data.
func NewDataAckPacket(sessionID uuid.UUID, streamID uint32, payload []byte, ackNum uint32) (*Packet, error) {
	pkt, err := NewPacket(sessionID, streamID, FlagData|FlagAck, payload)
	if err != nil {
		return nil, err
	}
	pkt.AckNum = ackNum
	return pkt, nil
}

// NewAckPacket creates an acknowledgment-only packet.
func NewAckPacket(sessionID uuid.UUID, streamID uint32, ackNum uint32) (*Packet, error) {
	pkt, err := NewPacket(sessionID, streamID, FlagAck, nil)
	if err != nil {
		return nil, err
	}
	pkt.AckNum = ackNum
	return pkt, nil
}

// NewKeepAlivePacket creates a keep-alive packet for connection health monitoring.
func NewKeepAlivePacket(sessionID uuid.UUID) (*Packet, error) {
	return NewPacket(sessionID, 0, FlagKeepAlive, nil)
}

// NewKeepAliveAckPacket creates a keep-alive acknowledgment packet.
func NewKeepAliveAckPacket(sessionID uuid.UUID) (*Packet, error) {
	return NewPacket(sessionID, 0, FlagKeepAlive|FlagAck, nil)
}

// NewFinPacket creates a connection termination packet for a specific stream.
func NewFinPacket(sessionID uuid.UUID, streamID uint32) (*Packet, error) {
	return NewPacket(sessionID, streamID, FlagFin, nil)
}

// NewFinAckPacket creates a FIN acknowledgment packet.
func NewFinAckPacket(sessionID uuid.UUID, streamID uint32, ackNum uint32) (*Packet, error) {
	pkt, err := NewPacket(sessionID, streamID, FlagFin|FlagAck, nil)
	if err != nil {
		return nil, err
	}
	pkt.AckNum = ackNum
	return pkt, nil
}

// SetSequenceNumber sets the sequence number on the packet and returns it for chaining.
func (p *Packet) SetSequenceNumber(seqNum uint32) *Packet {
	p.SeqNum = seqNum
	return p
}

// SetAckNumber sets the acknowledgment number on the packet and returns it for chaining.
func (p *Packet) SetAckNumber(ackNum uint32) *Packet {
	p.AckNum = ackNum
	return p
}

// WithAck adds the ACK flag to the packet and sets the acknowledgment number.
func (p *Packet) WithAck(ackNum uint32) *Packet {
	p.Flags |= FlagAck
	p.AckNum = ackNum
	return p
}

// IsControlPacket returns true if this is a control packet (StreamID 0).
func (p *Packet) IsControlPacket() bool {
	return p.StreamID == 0
}

// PacketType returns a string description of the packet type based on flags.
func (p *Packet) PacketType() string {
	switch {
	case p.IsHandshake() && p.IsAck():
		return "HANDSHAKE_ACK"
	case p.IsHandshake():
		return "HANDSHAKE"
	case p.IsFin() && p.IsAck():
		return "FIN_ACK"
	case p.IsFin():
		return "FIN"
	case p.IsKeepAlive() && p.IsAck():
		return "KEEPALIVE_ACK"
	case p.IsKeepAlive():
		return "KEEPALIVE"
	case p.IsData() && p.IsAck():
		return "DATA_ACK"
	case p.IsData():
		return "DATA"
	case p.IsAck():
		return "ACK"
	default:
		return "UNKNOWN"
	}
}

// Clone creates a deep copy of the packet.
func (p *Packet) Clone() *Packet {
	return copyPacket(p)
}
