// Package service provides systemd service management for Half-Tunnel.
package service

import (
	"testing"
)

func TestServiceName(t *testing.T) {
	tests := []struct {
		serviceType ServiceType
		expected    string
	}{
		{ClientService, "half-tunnel-client"},
		{ServerService, "half-tunnel-server"},
	}

	for _, tc := range tests {
		t.Run(string(tc.serviceType), func(t *testing.T) {
			result := ServiceName(tc.serviceType)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestServiceFilePath(t *testing.T) {
	tests := []struct {
		serviceType ServiceType
		expected    string
	}{
		{ClientService, "/etc/systemd/system/half-tunnel-client.service"},
		{ServerService, "/etc/systemd/system/half-tunnel-server.service"},
	}

	for _, tc := range tests {
		t.Run(string(tc.serviceType), func(t *testing.T) {
			result := ServiceFilePath(tc.serviceType)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetDefaultBinaryPath(t *testing.T) {
	tests := []struct {
		serviceType ServiceType
		expected    string
	}{
		{ClientService, "/usr/local/bin/ht-client"},
		{ServerService, "/usr/local/bin/ht-server"},
	}

	for _, tc := range tests {
		t.Run(string(tc.serviceType), func(t *testing.T) {
			result := GetDefaultBinaryPath(tc.serviceType)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	tests := []struct {
		serviceType ServiceType
		expected    string
	}{
		{ClientService, "/etc/half-tunnel/client.yml"},
		{ServerService, "/etc/half-tunnel/server.yml"},
	}

	for _, tc := range tests {
		t.Run(string(tc.serviceType), func(t *testing.T) {
			result := GetDefaultConfigPath(tc.serviceType)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestIsInstalled_NotInstalled(t *testing.T) {
	// Test when service is not installed
	// The service file should not exist in a normal test environment
	if IsInstalled(ClientService) {
		t.Skip("half-tunnel-client service is already installed")
	}
	if IsInstalled(ServerService) {
		t.Skip("half-tunnel-server service is already installed")
	}
}
