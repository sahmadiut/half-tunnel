package logger

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestWithDuration(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  "debug",
		Format: "json",
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Redirect output to buffer
	log.zl = log.zl.Output(&buf)

	duration := 123 * time.Millisecond
	log.WithDuration("test_duration", duration).Info().Msg("test message")

	// Parse JSON output
	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if result["test_duration"] != float64(123) {
		t.Errorf("Expected test_duration to be 123, got %v", result["test_duration"])
	}
}

func TestWithBytes(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  "debug",
		Format: "json",
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Redirect output to buffer
	log.zl = log.zl.Output(&buf)

	byteCount := int64(1024 * 1024)
	log.WithBytes("bytes_sent", byteCount).Info().Msg("test message")

	// Parse JSON output
	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if result["bytes_sent"] != float64(1048576) {
		t.Errorf("Expected bytes_sent to be 1048576, got %v", result["bytes_sent"])
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Level:  "debug",
		Format: "json",
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Redirect output to buffer
	log.zl = log.zl.Output(&buf)

	fields := map[string]interface{}{
		"field1": "value1",
		"field2": 42,
	}
	log.WithFields(fields).Info().Msg("test message")

	// Parse JSON output
	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if result["field1"] != "value1" {
		t.Errorf("Expected field1 to be 'value1', got %v", result["field1"])
	}
	if result["field2"] != float64(42) {
		t.Errorf("Expected field2 to be 42, got %v", result["field2"])
	}
}
