// Package socks5 provides a SOCKS5 proxy server implementation.
package socks5

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// Errors
var (
	ErrUnsupportedVersion     = errors.New("unsupported SOCKS version")
	ErrUnsupportedCommand     = errors.New("unsupported command")
	ErrUnsupportedAddressType = errors.New("unsupported address type")
	ErrAuthFailed             = errors.New("authentication failed")
	ErrConnectionRefused      = errors.New("connection refused")
)

// SOCKS5 constants
const (
	Version5 = 0x05

	// Authentication methods
	AuthNone         = 0x00
	AuthUserPass     = 0x02
	AuthNoAcceptable = 0xFF

	// Commands
	CmdConnect      = 0x01
	CmdBind         = 0x02
	CmdUDPAssociate = 0x03

	// Address types
	AddrTypeIPv4   = 0x01
	AddrTypeDomain = 0x03
	AddrTypeIPv6   = 0x04

	// Reply codes
	ReplySuccess                 = 0x00
	ReplyGeneralFailure          = 0x01
	ReplyConnectionRefused       = 0x05
	ReplyCommandNotSupported     = 0x07
	ReplyAddressTypeNotSupported = 0x08
)

// Config holds SOCKS5 server configuration.
type Config struct {
	ListenAddr string
	Username   string
	Password   string
}

// ConnectRequest represents a SOCKS5 CONNECT request.
type ConnectRequest struct {
	// DestHost is the destination hostname or IP address.
	DestHost string
	// DestPort is the destination port.
	DestPort uint16
	// ClientConn is the client connection that needs to be proxied.
	ClientConn net.Conn
}

// ConnectHandler is called for each SOCKS5 CONNECT request.
// The handler is responsible for establishing the connection to the destination
// and copying data between the client and destination.
type ConnectHandler func(ctx context.Context, req *ConnectRequest) error

// Server is a SOCKS5 proxy server.
type Server struct {
	config   *Config
	listener net.Listener
	handler  ConnectHandler
	mu       sync.RWMutex
	closed   bool
	wg       sync.WaitGroup
}

// NewServer creates a new SOCKS5 server.
func NewServer(config *Config, handler ConnectHandler) *Server {
	return &Server{
		config:  config,
		handler: handler,
	}
}

// ListenAndServe starts the SOCKS5 server.
func (s *Server) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return err
	}

	return s.Serve(ctx, listener)
}

// Serve starts the SOCKS5 server using the provided listener.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.mu.RLock()
			closed := s.closed
			s.mu.RUnlock()
			if closed {
				return nil
			}
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(ctx, conn)
		}()
	}
}

// Close stops the server.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Wait waits for all connections to complete.
func (s *Server) Wait() {
	s.wg.Wait()
}

// handleConnection handles a single SOCKS5 connection.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// 1. Handle authentication negotiation
	if err := s.handleAuth(conn); err != nil {
		return
	}

	// 2. Handle connect request
	destHost, destPort, err := s.handleRequest(conn)
	if err != nil {
		return
	}

	// 3. Call the connect handler
	req := &ConnectRequest{
		DestHost:   destHost,
		DestPort:   destPort,
		ClientConn: conn,
	}

	if err := s.handler(ctx, req); err != nil {
		// Handler already dealt with the connection
		return
	}
}

// handleAuth handles SOCKS5 authentication negotiation.
func (s *Server) handleAuth(conn net.Conn) error {
	// Read version and number of methods
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}

	if header[0] != Version5 {
		return ErrUnsupportedVersion
	}

	numMethods := int(header[1])
	methods := make([]byte, numMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	// Check if auth is required
	requireAuth := s.config.Username != "" && s.config.Password != ""

	if requireAuth {
		// Check if username/password auth is supported
		hasUserPass := false
		for _, m := range methods {
			if m == AuthUserPass {
				hasUserPass = true
				break
			}
		}

		if !hasUserPass {
			_, _ = conn.Write([]byte{Version5, AuthNoAcceptable})
			return ErrAuthFailed
		}

		// Request username/password auth
		if _, err := conn.Write([]byte{Version5, AuthUserPass}); err != nil {
			return err
		}

		// Read username/password
		if err := s.handleUserPassAuth(conn); err != nil {
			return err
		}
	} else {
		// No auth required
		hasNoAuth := false
		for _, m := range methods {
			if m == AuthNone {
				hasNoAuth = true
				break
			}
		}

		if !hasNoAuth {
			_, _ = conn.Write([]byte{Version5, AuthNoAcceptable})
			return ErrAuthFailed
		}

		if _, err := conn.Write([]byte{Version5, AuthNone}); err != nil {
			return err
		}
	}

	return nil
}

// handleUserPassAuth handles username/password authentication.
func (s *Server) handleUserPassAuth(conn net.Conn) error {
	// Read auth version
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}

	// Read username
	ulen := int(header[1])
	username := make([]byte, ulen)
	if _, err := io.ReadFull(conn, username); err != nil {
		return err
	}

	// Read password length
	plenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, plenBuf); err != nil {
		return err
	}

	// Read password
	plen := int(plenBuf[0])
	password := make([]byte, plen)
	if _, err := io.ReadFull(conn, password); err != nil {
		return err
	}

	// Verify credentials using constant-time comparison to prevent timing attacks
	usernameMatch := subtle.ConstantTimeCompare(username, []byte(s.config.Username)) == 1
	passwordMatch := subtle.ConstantTimeCompare(password, []byte(s.config.Password)) == 1
	if !usernameMatch || !passwordMatch {
		_, _ = conn.Write([]byte{0x01, 0x01}) // Auth failed
		return ErrAuthFailed
	}

	// Auth success
	_, err := conn.Write([]byte{0x01, 0x00})
	return err
}

// handleRequest handles the SOCKS5 request.
func (s *Server) handleRequest(conn net.Conn) (string, uint16, error) {
	// Read request header: VER CMD RSV ATYP
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", 0, err
	}

	if header[0] != Version5 {
		return "", 0, ErrUnsupportedVersion
	}

	if header[1] != CmdConnect {
		_ = s.sendReply(conn, ReplyCommandNotSupported, nil, 0)
		return "", 0, ErrUnsupportedCommand
	}

	// Parse destination address
	var destHost string
	switch header[3] {
	case AddrTypeIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, err
		}
		destHost = net.IP(addr).String()

	case AddrTypeDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", 0, err
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", 0, err
		}
		destHost = string(domain)

	case AddrTypeIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", 0, err
		}
		destHost = net.IP(addr).String()

	default:
		_ = s.sendReply(conn, ReplyAddressTypeNotSupported, nil, 0)
		return "", 0, ErrUnsupportedAddressType
	}

	// Read port
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", 0, err
	}
	destPort := binary.BigEndian.Uint16(portBuf)

	return destHost, destPort, nil
}

// SendSuccessReply sends a success reply to the client.
func (s *Server) SendSuccessReply(conn net.Conn, bindAddr string, bindPort uint16) error {
	ip := net.ParseIP(bindAddr)
	return s.sendReply(conn, ReplySuccess, ip, bindPort)
}

// SendFailureReply sends a failure reply to the client.
func (s *Server) SendFailureReply(conn net.Conn, code byte) error {
	return s.sendReply(conn, code, nil, 0)
}

// sendReply sends a SOCKS5 reply to the client.
func (s *Server) sendReply(conn net.Conn, code byte, bindAddr net.IP, bindPort uint16) error {
	reply := make([]byte, 10)
	reply[0] = Version5
	reply[1] = code
	reply[2] = 0x00 // RSV

	if bindAddr == nil {
		bindAddr = net.IPv4zero
	}

	if ip4 := bindAddr.To4(); ip4 != nil {
		reply[3] = AddrTypeIPv4
		copy(reply[4:8], ip4)
		binary.BigEndian.PutUint16(reply[8:10], bindPort)
		_, err := conn.Write(reply)
		return err
	}

	// IPv6
	reply = make([]byte, 22)
	reply[0] = Version5
	reply[1] = code
	reply[2] = 0x00
	reply[3] = AddrTypeIPv6
	copy(reply[4:20], bindAddr.To16())
	binary.BigEndian.PutUint16(reply[20:22], bindPort)
	_, err := conn.Write(reply)
	return err
}

// Addr returns the listener address.
func (s *Server) Addr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// FormatDestination formats the destination as host:port string.
func FormatDestination(host string, port uint16) string {
	return fmt.Sprintf("%s:%d", host, port)
}
