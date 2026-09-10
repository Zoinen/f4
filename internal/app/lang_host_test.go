package app

import (
	"testing"
)

func TestFormatVersionSHA(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"v0.1.1-alpha-a1b2c3d4", "v0.1.1-alpha-a1b2c3d"},
		{"f4 version 0.1 (commit ffeeddcc)", "f4 version 0.1 (commit ffeeddc)"},
		{"v0.1.1-a1b2c3d45", "v0.1.1-a1b2c3d45"},
		{"v0.1.1-a1b2c3d", "v0.1.1-a1b2c3d"},
	}

	for _, tt := range tests {
		if got := formatVersionSHA(tt.in); got != tt.want {
			t.Errorf("formatVersionSHA(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
