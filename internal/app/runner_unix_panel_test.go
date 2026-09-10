//go:build !windows

package app

import (
	"testing"

	"context"
	"errors"
	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/vfs"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func TestLocalCommandRunnerCancellationKillsProcessGroup(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	pidPath := dir + "/child.pid"
	quotedPIDPath, err := cmdline.QuoteCommandPath(vfs.CommandDialectPOSIX, pidPath)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, runErr := terminal.NewLocalCommandRunner().RunCommand(
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
			pidText := strings.TrimSpace(string(data))
			if pidText == "" {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			childPID, err = strconv.Atoi(pidText)
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
