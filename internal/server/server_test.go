package server

import (
	"testing"
	
	"github.com/sahmadiut/half-tunnel/internal/socks5"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.UpstreamAddr != ":8080" {
		t.Errorf("Expected UpstreamAddr :8080, got %s", config.UpstreamAddr)
	}
	if config.DownstreamAddr != ":8081" {
		t.Errorf("Expected DownstreamAddr :8081, got %s", config.DownstreamAddr)
	}
	if config.UpstreamPath != "/upstream" {
		t.Errorf("Expected UpstreamPath /upstream, got %s", config.UpstreamPath)
	}
	if config.DownstreamPath != "/downstream" {
		t.Errorf("Expected DownstreamPath /downstream, got %s", config.DownstreamPath)
	}
}

func TestParseConnectPayload(t *testing.T) {
	tests := []struct {
		name     string
		payload  []byte
		wantHost string
		wantPort uint16
		wantErr  bool
	}{
		{
			name: "IPv4",
			payload: []byte{
				socks5.AddrTypeIPv4,
				127, 0, 0, 1, // 127.0.0.1
				0x1F, 0x90, // 8080
			},
			wantHost: "127.0.0.1",
			wantPort: 8080,
			wantErr:  false,
		},
		{
			name: "Domain",
			payload: []byte{
				socks5.AddrTypeDomain,
				11, // length
				'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm',
				0x01, 0xBB, // 443
			},
			wantHost: "example.com",
			wantPort: 443,
			wantErr:  false,
		},
		{
			name: "IPv6",
			payload: []byte{
				socks5.AddrTypeIPv6,
				0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, // ::1
				0x00, 0x50, // 80
			},
			wantHost: "::1",
			wantPort: 80,
			wantErr:  false,
		},
		{
			name:    "TooShort",
			payload: []byte{0x01, 0x01},
			wantErr: true,
		},
		{
			name:    "InvalidType",
			payload: []byte{0xFF, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := parseConnectPayload(tt.payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseConnectPayload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if host != tt.wantHost {
					t.Errorf("parseConnectPayload() host = %v, want %v", host, tt.wantHost)
				}
				if port != tt.wantPort {
					t.Errorf("parseConnectPayload() port = %v, want %v", port, tt.wantPort)
				}
			}
		})
	}
}

func TestNewServer(t *testing.T) {
	server := New(nil, nil)
	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.config == nil {
		t.Error("Expected non-nil config")
	}
	if server.log == nil {
		t.Error("Expected non-nil logger")
	}
}

func TestServerGetters(t *testing.T) {
	server := New(nil, nil)
	
	// Test session count
	if count := server.GetSessionCount(); count != 0 {
		t.Errorf("Expected 0 sessions, got %d", count)
	}
	
	// Test NAT entry count
	if count := server.GetNatEntryCount(); count != 0 {
		t.Errorf("Expected 0 NAT entries, got %d", count)
	}
}
