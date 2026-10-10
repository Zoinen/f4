//go:build lite || (!linux && !darwin && !freebsd)

package fusefs

import (
	"errors"
	"testing"
)

func TestUnsupportedServerDoesNotStart(t *testing.T) {
	if Supported() {
		t.Fatal("unsupported build advertises FUSE support")
	}
	if err := startServer(t.Context(), nil, Options{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("startServer error = %v, want unsupported", err)
	}
}
