//go:build windows

package netfox

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestBuildWSLArgsDefaultDistro(t *testing.T) {
	got := buildWSLArgs("")
	want := []string{"--", "/bin/sh"}
	if len(got) != len(want) {
		t.Fatalf("buildWSLArgs(\"\") = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("buildWSLArgs(\"\")[%d] = %q, want %q (full: %q)", i, got[i], want[i], got)
		}
	}
}

func TestBuildWSLArgsNamedDistro(t *testing.T) {
	got := buildWSLArgs("Ubuntu")
	want := []string{"-d", "Ubuntu", "--", "/bin/sh"}
	if len(got) != len(want) {
		t.Fatalf("buildWSLArgs(\"Ubuntu\") = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("buildWSLArgs(\"Ubuntu\")[%d] = %q, want %q (full: %q)", i, got[i], want[i], got)
		}
	}
}

// TestDialWSLSubprocessHonoursACancelledContext: a reconnect the user gave
// up on must not spawn wsl.exe at all.
func TestDialWSLSubprocessHonoursACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, closer, err := dialWSLSubprocess(ctx, "Ubuntu")
	if !errors.Is(err, context.Canceled) {
		if closer != nil {
			_ = closer.Close() // unexpected dial cleanup only
		}
		t.Fatalf("dial answered %v, want context.Canceled", err)
	}
	if closer != nil {
		t.Fatal("a cancelled dial handed back something to close")
	}
}

// TestDialWSLSubprocessReportsAMissingBinary: no wsl.exe on PATH must fail
// the dial itself rather than spawn something that can never work -- and
// must not depend on the CI runner actually having WSL installed.
func TestDialWSLSubprocessReportsAMissingBinary(t *testing.T) {
	orig := wslLookPath
	wslLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { wslLookPath = orig }()

	_, _, closer, err := dialWSLSubprocess(context.Background(), "Ubuntu")
	if err == nil {
		if closer != nil {
			_ = closer.Close() // unexpected dial cleanup only
		}
		t.Fatal("dialling with no wsl.exe binary on PATH succeeded")
	}
	if closer != nil {
		t.Fatal("a failed dial handed back something to close")
	}
}

// TestWSLFishDialerBuildsAFishDialer just confirms the dialer returns
// something callable with the FishDialer signature -- the behavior itself
// is dialWSLSubprocess's, already covered above.
func TestWSLFishDialerBuildsAFishDialer(t *testing.T) {
	var _ FishDialer = wslFishDialer("Ubuntu")
	var _ FishDialer = wslFishDialer("")
}
