package constants

import "testing"

func TestGetBufferSize(t *testing.T) {
	tests := []struct {
		mode     BufferMode
		expected int
	}{
		{BufferModeSmall, SmallBufferSize},
		{BufferModeDefault, DefaultBufferSize},
		{BufferModeLarge, LargeBufferSize},
		{BufferModeMax, MaxBufferSize},
		{"unknown", DefaultBufferSize},
		{"", DefaultBufferSize},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			size := GetBufferSize(tt.mode)
			if size != tt.expected {
				t.Errorf("GetBufferSize(%s) = %d, want %d", tt.mode, size, tt.expected)
			}
		})
	}
}

func TestBufferSizeValues(t *testing.T) {
	// Verify buffer sizes are powers of 2 and in increasing order
	sizes := []int{SmallBufferSize, DefaultBufferSize, LargeBufferSize, MaxBufferSize}
	expected := []int{16384, 32768, 65536, 131072}

	for i, size := range sizes {
		if size != expected[i] {
			t.Errorf("Buffer size at index %d = %d, want %d", i, size, expected[i])
		}
	}

	// Verify sizes are in increasing order
	for i := 1; i < len(sizes); i++ {
		if sizes[i] <= sizes[i-1] {
			t.Errorf("Buffer sizes not in increasing order: %d <= %d", sizes[i], sizes[i-1])
		}
	}
}

func TestBufferModeConstants(t *testing.T) {
	// Verify buffer mode string values
	if BufferModeSmall != "small" {
		t.Errorf("BufferModeSmall = %s, want small", BufferModeSmall)
	}
	if BufferModeDefault != "default" {
		t.Errorf("BufferModeDefault = %s, want default", BufferModeDefault)
	}
	if BufferModeLarge != "large" {
		t.Errorf("BufferModeLarge = %s, want large", BufferModeLarge)
	}
	if BufferModeMax != "max" {
		t.Errorf("BufferModeMax = %s, want max", BufferModeMax)
	}
}
