package client

import (
	"net"
	"testing"

	"github.com/sahmadiut/half-tunnel/internal/socks5"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.UpstreamURL != "ws://localhost:8080/upstream" {
		t.Errorf("Expected UpstreamURL ws://localhost:8080/upstream, got %s", config.UpstreamURL)
	}
	if config.DownstreamURL != "ws://localhost:8081/downstream" {
		t.Errorf("Expected DownstreamURL ws://localhost:8081/downstream, got %s", config.DownstreamURL)
	}
	if config.SOCKS5Addr != "127.0.0.1:1080" {
		t.Errorf("Expected SOCKS5Addr 127.0.0.1:1080, got %s", config.SOCKS5Addr)
	}
	if !config.ReconnectEnabled {
		t.Error("Expected reconnect enabled by default")
	}
	if config.ReconnectConfig == nil {
		t.Error("Expected reconnect config to be set")
	}
}

func TestNewClient(t *testing.T) {
	client := New(nil, nil)
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if client.config == nil {
		t.Error("Expected non-nil config")
	}
	if client.log == nil {
		t.Error("Expected non-nil logger")
	}
}

func TestFormatConnectPayloadIPv4(t *testing.T) {
	payload := formatConnectPayload("192.168.1.1", 8080)

	if len(payload) != 7 {
		t.Fatalf("Expected payload length 7, got %d", len(payload))
	}

	if payload[0] != socks5.AddrTypeIPv4 {
		t.Errorf("Expected address type IPv4, got %d", payload[0])
	}

	ip := net.IP(payload[1:5])
	if ip.String() != "192.168.1.1" {
		t.Errorf("Expected IP 192.168.1.1, got %s", ip.String())
	}

	port := uint16(payload[5])<<8 | uint16(payload[6])
	if port != 8080 {
		t.Errorf("Expected port 8080, got %d", port)
	}
}

func TestFormatConnectPayloadIPv6(t *testing.T) {
	payload := formatConnectPayload("::1", 443)

	if len(payload) != 19 {
		t.Fatalf("Expected payload length 19, got %d", len(payload))
	}

	if payload[0] != socks5.AddrTypeIPv6 {
		t.Errorf("Expected address type IPv6, got %d", payload[0])
	}

	ip := net.IP(payload[1:17])
	if ip.String() != "::1" {
		t.Errorf("Expected IP ::1, got %s", ip.String())
	}

	port := uint16(payload[17])<<8 | uint16(payload[18])
	if port != 443 {
		t.Errorf("Expected port 443, got %d", port)
	}
}

func TestFormatConnectPayloadDomain(t *testing.T) {
	payload := formatConnectPayload("example.com", 80)

	expectedLen := 1 + 1 + len("example.com") + 2 // type + len + domain + port
	if len(payload) != expectedLen {
		t.Fatalf("Expected payload length %d, got %d", expectedLen, len(payload))
	}

	if payload[0] != socks5.AddrTypeDomain {
		t.Errorf("Expected address type Domain, got %d", payload[0])
	}

	domainLen := int(payload[1])
	if domainLen != len("example.com") {
		t.Errorf("Expected domain length %d, got %d", len("example.com"), domainLen)
	}

	domain := string(payload[2 : 2+domainLen])
	if domain != "example.com" {
		t.Errorf("Expected domain example.com, got %s", domain)
	}

	portOffset := 2 + domainLen
	port := uint16(payload[portOffset])<<8 | uint16(payload[portOffset+1])
	if port != 80 {
		t.Errorf("Expected port 80, got %d", port)
	}
}

func TestClientNotRunning(t *testing.T) {
	client := New(nil, nil)

	// Stop should not error when not running
	err := client.Stop()
	if err != nil {
		t.Errorf("Stop() on non-running client should not error: %v", err)
	}
}
