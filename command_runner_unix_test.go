//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestLocalCommandRunnerStreamsMergedLinesAndExitStatus(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	var got []string
	code, err := NewLocalCommandRunner().RunCommand(
		context.Background(),
		dir,
		"pwd; if IFS= read -r line; then exit 91; else printf 'stdin-eof\\n'; fi; printf 'stderr-line\\n' >&2; printf partial; exit 7",
		func(line string) { got = append(got, line) },
	)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	want := []string{dir, "stdin-eof", "stderr-line", "partial"}
	if len(got) != len(want) {
		t.Fatalf("lines = %#v, want %#v", got, want)
	}
	gotDir, gotDirErr := filepath.EvalSymlinks(got[0])
	wantDir, wantDirErr := filepath.EvalSymlinks(dir)
	if gotDirErr != nil || wantDirErr != nil || gotDir != wantDir {
		t.Fatalf("command directory = %q (%v), want %q (%v)", gotDir, gotDirErr, wantDir, wantDirErr)
	}
	for i := 1; i < len(want); i++ {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q, want %q (all: %#v)", i, got[i], want[i], got)
		}
	}

	info := NewLocalCommandRunner().CommandRunnerInfo()
	if info.Dialect != vfs.CommandDialectPOSIX || info.MaxParallel != 0 {
		t.Fatalf("runner info = %+v", info)
	}
}

func TestLocalCommandRunnerCancellationKillsProcessGroup(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	pidPath := dir + "/child.pid"
	quotedPIDPath, err := QuoteCommandPath(vfs.CommandDialectPOSIX, pidPath)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, runErr := NewLocalCommandRunner().RunCommand(
			ctx,
			dir,
			"sleep 30 & child=$!; printf '%s\\n' \"$child\" > "+quotedPIDPath+"; wait",
			nil,
		)
		done <- runErr
	}()

	var childPID int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(pidPath)
		if readErr == nil {
			childPID, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatalf("child pid %q: %v", data, err)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		cancel()
		<-done
		t.Fatal("child process did not start")
	}

	cancel()
	select {
	case runErr := <-done:
		if !errors.Is(runErr, context.Canceled) {
			t.Fatalf("RunCommand error = %v, want context.Canceled", runErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunCommand did not return after cancellation")
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(childPID, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(childPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("child process %d survived cancellation (kill probe: %v)", childPID, err)
	}
}
