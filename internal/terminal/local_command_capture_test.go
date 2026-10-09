package terminal

import (
	"context"
	"strings"
	"testing"
)

func TestRunLocalCommandCaptureUsesTheLocalShellTransport(t *testing.T) {
	out, err := RunLocalCommandCapture(context.Background(), t.TempDir(), "echo f4_capture_api_test")
	if err != nil {
		t.Fatalf("RunLocalCommandCapture: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "f4_capture_api_test" {
		t.Fatalf("captured output = %q, want %q", got, "f4_capture_api_test")
	}
}
