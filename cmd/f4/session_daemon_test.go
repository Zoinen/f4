//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A daemon that dies at startup must be reported as soon as it dies, not at
// the end of daemonStartTimeout -- and must not be attached to.
func TestWaitForDaemonSocketDaemonExits(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "f4.sock")

	cmd := exec.Command("/bin/sh", "-c", "exit 3")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start a helper process here: %v", err)
	}

	start := time.Now()
	if waitForDaemonSocket(cmd, sockPath) {
		t.Fatal("waitForDaemonSocket() = true for a daemon that created no socket")
	}
	if waited := time.Since(start); waited >= daemonStartTimeout {
		t.Errorf("waited %s for a process that had already exited", waited)
	}
}

func TestWaitForDaemonSocketAppears(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "f4.sock")

	cmd := exec.Command("/bin/sh", "-c", "sleep 0.1; : > "+sockPath+"; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start a helper process here: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	if !waitForDaemonSocket(cmd, sockPath) {
		t.Error("waitForDaemonSocket() = false although the socket appeared")
	}
}

// A daemon killed by a signal says nothing; a daemon that failed to load says
// it on stderr, and that is what the client has to show.
func TestReadStartupLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f4.sock.startup")
	if got := readStartupLog(path); got != "" {
		t.Errorf("readStartupLog() of a missing file = %q, want empty", got)
	}
	if got := readStartupLog(""); got != "" {
		t.Errorf("readStartupLog(\"\") = %q, want empty", got)
	}

	const msg = "symbol lookup error: undefined symbol: malloc"
	if err := os.WriteFile(path, []byte(msg+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := readStartupLog(path); got != msg {
		t.Errorf("readStartupLog() = %q, want %q", got, msg)
	}

	removeStartupLog(path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("removeStartupLog left %s behind (%v)", path, err)
	}
}
