// Package transport provides tests for Chisel transport functionality.
package transport

import (
	"testing"
	"time"
)

func TestDefaultChiselConfig(t *testing.T) {
	config := DefaultChiselConfig()

	if config == nil {
		t.Fatal("DefaultChiselConfig should not return nil")
	}

	if config.Enabled {
		t.Error("Chisel should be disabled by default")
	}

	if config.Host != "0.0.0.0" {
		t.Errorf("Expected host '0.0.0.0', got '%s'", config.Host)
	}

	if config.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", config.Port)
	}

	if config.KeepAlive != 25*time.Second {
		t.Errorf("Expected keepalive 25s, got %v", config.KeepAlive)
	}
}

func TestPortManager(t *testing.T) {
	pm := NewPortManager(9000, 9005)

	// Allocate ports
	port1, err := pm.Allocate()
	if err != nil {
		t.Fatalf("Failed to allocate first port: %v", err)
	}
	if port1 != 9000 {
		t.Errorf("Expected first port to be 9000, got %d", port1)
	}

	port2, err := pm.Allocate()
	if err != nil {
		t.Fatalf("Failed to allocate second port: %v", err)
	}
	if port2 != 9001 {
		t.Errorf("Expected second port to be 9001, got %d", port2)
	}

	// Check IsAllocated
	if !pm.IsAllocated(9000) {
		t.Error("Port 9000 should be allocated")
	}
	if !pm.IsAllocated(9001) {
		t.Error("Port 9001 should be allocated")
	}
	if pm.IsAllocated(9002) {
		t.Error("Port 9002 should not be allocated")
	}

	// Release port and reallocate
	pm.Release(9000)
	if pm.IsAllocated(9000) {
		t.Error("Port 9000 should not be allocated after release")
	}

	port3, err := pm.Allocate()
	if err != nil {
		t.Fatalf("Failed to allocate third port: %v", err)
	}
	if port3 != 9000 {
		t.Errorf("Expected third port to be 9000 (reused), got %d", port3)
	}
}

func TestPortManagerExhaustion(t *testing.T) {
	pm := NewPortManager(9000, 9002) // Only 3 ports

	// Allocate all ports
	for i := 0; i < 3; i++ {
		_, err := pm.Allocate()
		if err != nil {
			t.Fatalf("Failed to allocate port %d: %v", i, err)
		}
	}

	// Try to allocate when exhausted
	_, err := pm.Allocate()
	if err == nil {
		t.Error("Expected error when allocating from exhausted pool")
	}
}

func TestChiselDataConnectionInterface(t *testing.T) {
	// Verify ChiselDataConnection implements Transport interface
	var _ Transport = (*ChiselDataConnection)(nil)
}
