package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseServiceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "client", want: serviceNameClient},
		{input: " server ", want: serviceNameServer},
		{input: "unknown", wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got, err := parseServiceName(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", test.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestRenderServiceFile(t *testing.T) {
	t.Parallel()

	content := renderServiceFile("Half-Tunnel Server", "/opt/half-tunnel", "ht-server", "/etc/half-tunnel/server.yml", "root")

	assertContains(t, content, "Description=Half-Tunnel Server")
	assertContains(t, content, "ExecStart=/opt/half-tunnel/ht-server -config /etc/half-tunnel/server.yml")
	assertContains(t, content, "User=root")
}

func TestWriteServiceFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "half-tunnel.service")
	content := "service-content"

	if err := writeServiceFile(path, content, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read service file: %v", err)
	}
	if string(data) != content {
		t.Fatalf("expected content %q, got %q", content, string(data))
	}

	if err := writeServiceFile(path, "new-content", false); err == nil {
		t.Fatalf("expected error when file exists without overwrite")
	}

	if err := writeServiceFile(path, "new-content", true); err != nil {
		t.Fatalf("unexpected error with overwrite: %v", err)
	}

	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read service file after overwrite: %v", err)
	}
	if string(data) != "new-content" {
		t.Fatalf("expected overwritten content, got %q", string(data))
	}
}

func TestFormatActionTitle(t *testing.T) {
	t.Parallel()

	if got := formatActionTitle("start"); got != "Start" {
		t.Fatalf("expected Start, got %q", got)
	}
	if got := formatActionTitle(""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func assertContains(t *testing.T, content, expected string) {
	t.Helper()
	if !strings.Contains(content, expected) {
		t.Fatalf("expected %q to contain %q", content, expected)
	}
}
