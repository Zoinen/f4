//go:build !windows

package vfs

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// hangingDispatcher is a sudo dispatcher that takes a request and never
// answers, the way a PAM prompt nobody answers looks from f4.
func hangingDispatcher(t *testing.T) *SudoClient {
	t.Helper()
	sock := filepath.Join(shortSocketDir(t), "h.sock")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	t.Cleanup(func() { close(release); _ = l.Close() })
	go func() {
		conn, err := l.AcceptUnix()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var req SudoRequest
		if _, err := recvMsg(conn, &req); err != nil {
			return
		}
		<-release
	}()
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &SudoClient{conn: conn}
}

// Cancel in the progress window has to release the caller even while the
// elevated open is still waiting for a password (f4#1411).
func TestSudoOpenCancellableReturnsWhenCancelled(t *testing.T) {
	old := globalSudoClient
	t.Cleanup(func() { globalSudoClient = old })
	globalSudoClient = hangingDispatcher(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := sudoOpenCancellable(ctx, "/root/secret")
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("sudoOpenCancellable = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling did not release the caller")
	}
}

// Without a cancel the result of the open is passed on as it is.
func TestSudoOpenCancellablePassesTheResultOn(t *testing.T) {
	old := globalSudoClient
	t.Cleanup(func() { globalSudoClient = old })
	globalSudoClient = answeringDispatcher(t, "permission denied")

	if _, err := sudoOpenCancellable(context.Background(), "/root/secret"); err == nil {
		t.Fatal("a refused open should be an error")
	}
}
