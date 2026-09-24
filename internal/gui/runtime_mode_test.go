package gui

import (
	"errors"
	"testing"
)

func TestWithRuntimeRestoresPreviousMode(t *testing.T) {
	Running = false
	t.Cleanup(func() { Running = false })
	if err := WithRuntime(func() error {
		if !Running {
			t.Fatal("runtime callback did not see GUI mode")
		}
		return nil
	}); err != nil || Running {
		t.Fatalf("success result=%v Running=%v", err, Running)
	}

	Running = true
	want := errors.New("setup failed")
	if err := WithRuntime(func() error {
		if !Running {
			t.Fatal("nested runtime callback lost GUI mode")
		}
		return want
	}); !errors.Is(err, want) || !Running {
		t.Fatalf("error result=%v Running=%v", err, Running)
	}
}
