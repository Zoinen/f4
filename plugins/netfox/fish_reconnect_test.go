package netfox

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/netfox/fishplus"
)

// localShellDialer starts a POSIX shell per call, which is as close to a
// reconnect as a test without a network gets: every call is a new process with
// a new pid, exactly what a redialled ssh session hands back.
func localShellDialer(t *testing.T) FishDialer {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX shell on Windows")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell available")
	}
	return func(ctx context.Context) (io.Writer, io.Reader, io.Closer, error) {
		cmd := exec.Command(shell)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, nil, nil, err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, nil, err
		}
		t.Cleanup(func() {
			_ = stdin.Close() // process cleanup only
			done := make(chan struct{})
			go func() {
				_ = cmd.Wait() // process cleanup only
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill() // process cleanup only
			}
		})
		return stdin, stdout, stdin, nil
	}
}

// TestFishReconnectReplacesADeadSession is the whole point: a session that
// stopped answering is swapped for a working one, and the panel keeps standing
// where it was, because the path is a client-side string that never depended on
// the shell that died.
func TestFishReconnectReplacesADeadSession(t *testing.T) {
	dial := localShellDialer(t)
	v, err := NewFishVFSOnDialer(context.Background(), nil, dial, "local")
	if err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Errorf("close FISH+ filesystem: %v", err)
		}
	}()

	was := v.GetPath()
	dead := v.conn.current()
	dead.Session().MarkBroken()

	fresh, err := v.conn.reconnect(context.Background(), dead)
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if fresh == dead {
		t.Fatal("reconnect handed back the session that had died")
	}
	if fresh.Session().Broken() {
		t.Fatal("the replacement session is already broken")
	}
	if v.GetPath() != was {
		t.Fatalf("the panel moved from %q to %q", was, v.GetPath())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if got, err := fresh.Session().Ping(ctx, "alive"); err != nil || got != "alive" {
		t.Fatalf("the replacement session does not answer: %q, %v", got, err)
	}
}

// TestFishReconnectYieldsToOneThatAlreadyHappened covers two views of one
// connection meeting the same dead session: the second must take the session
// the first installed rather than replacing a session that is already working.
func TestFishReconnectYieldsToOneThatAlreadyHappened(t *testing.T) {
	dial := localShellDialer(t)
	v, err := NewFishVFSOnDialer(context.Background(), nil, dial, "local")
	if err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Errorf("close FISH+ filesystem: %v", err)
		}
	}()

	dead := v.conn.current()
	dead.Session().MarkBroken()

	first, err := v.conn.reconnect(context.Background(), dead)
	if err != nil {
		t.Fatalf("first reconnect: %v", err)
	}
	second, err := v.conn.reconnect(context.Background(), dead)
	if err != nil {
		t.Fatalf("second reconnect: %v", err)
	}
	if second != first {
		t.Fatal("the second reconnect built a session of its own")
	}
}

// TestFishReconnectWithoutADialerRefuses: a caller that handed over a pair of
// streams has no second pair, and saying so is better than pretending.
func TestFishReconnectWithoutADialerRefuses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX shell on Windows")
	}
	v := newLocalFishVFS(t)

	dead := v.conn.current()
	dead.Session().MarkBroken()

	if _, err := v.conn.reconnect(context.Background(), dead); !errors.Is(err, ErrNoDialer) {
		t.Fatalf("reconnect without a dialer answered %v, want ErrNoDialer", err)
	}
}

// TestFishReconnectAfterCloseRefuses keeps a torn-down connection from being
// resurrected by a request that was still in flight when the last panel left.
func TestFishReconnectAfterCloseRefuses(t *testing.T) {
	dial := localShellDialer(t)
	v, err := NewFishVFSOnDialer(context.Background(), nil, dial, "local")
	if err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("open: %v", err)
	}
	dead := v.conn.current()
	if err := v.Close(); err != nil {
		t.Fatalf("close filesystem before reconnect: %v", err)
	}

	if _, err := v.conn.reconnect(context.Background(), dead); !errors.Is(err, fishplus.ErrBroken) {
		t.Fatalf("reconnect after close answered %v, want ErrBroken", err)
	}
}
