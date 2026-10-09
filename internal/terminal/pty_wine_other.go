//go:build !windows

package terminal

// Off Windows there is no Wine to run a native terminal under; the platform's
// own pty is the only one.

func newWinePTY() (PtyBackend, bool, error) { return nil, false, nil }

func nativeShellActive() bool { return false }
