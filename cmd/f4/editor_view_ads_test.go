package main

import (
	"runtime"
	"testing"
)

func TestEditorView_IsAlternateDataStream(t *testing.T) {
	if runtime.GOOS != "windows" {
		if isAlternateDataStream("/path/to/file.txt:stream") {
			t.Error("isAlternateDataStream should return false on non-Windows platforms")
		}
		return
	}

	tests := []struct {
		path string
		want bool
	}{
		{`C:\test.txt`, false},
		{`C:\test.txt:stream`, true},
		{`\\server\share\test.txt`, false},
		{`\\server\share\test.txt:stream`, true},
		{`test.txt:stream`, true},
		{`test.txt`, false},
	}

	for _, tt := range tests {
		if got := isAlternateDataStream(tt.path); got != tt.want {
			t.Errorf("isAlternateDataStream(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
