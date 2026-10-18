//go:build linux

package main

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/unxed/f4/internal/winex11drag"
	"github.com/unxed/vtui"
)

// TestMain keeps one X connection open for the whole run: Xvfb resets itself
// when its last client leaves, and the next test would find the server gone.
func TestMain(m *testing.M) {
	var keep *xgb.Conn
	if os.Getenv("DISPLAY") != "" {
		if conn, err := xgb.NewConn(); err == nil {
			keep = conn
		}
	}
	code := m.Run()
	if keep != nil {
		keep.Close()
	}
	os.Exit(code)
}

// startTarget runs the target on the test's X server, waits until it is ready
// and returns the channel with its exit code.
func startTarget(t *testing.T, cfg config) <-chan int {
	t.Helper()
	if os.Getenv("DISPLAY") == "" {
		t.Skip("no DISPLAY")
	}
	if conn, err := xgb.NewConn(); err != nil {
		t.Skipf("no X server: %v", err)
	} else {
		conn.Close()
	}
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pr.Close() })
	var diag strings.Builder
	code := make(chan int, 1)
	go func() {
		// diag is read only after the pipe closes, which happens after run
		// returned, so the writes cannot race with the read.
		code <- run(cfg, pw, &diag)
		_ = pw.Close()
	}()
	r := bufio.NewReader(pr)
	line, err := r.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("target did not become ready: %q, %v (diagnostics: %q)", line, err, diag.String())
	}
	// The pipe is synchronous: keep reading so the target never blocks on
	// its DROP line.
	go func() { _, _ = io.Copy(io.Discard, r) }()
	return code
}

// dragOnto drags the path from a winex11drag session onto the target window.
func dragOnto(t *testing.T, path string) {
	t.Helper()
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	s, err := winex11drag.NewSession(conn)
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	defer s.Close()

	done := make(chan error, 1)
	go func() {
		_, err := s.Drag(vtui.DragPayload{Paths: []string{path}}, vtui.DropCopy)
		done <- err
	}()
	// Keep the pointer over the target long enough for it to answer a
	// position, then let go.
	for range 100 {
		s.Motion(50, 50, xproto.TimeCurrentTime)
		time.Sleep(5 * time.Millisecond)
	}
	s.Release(xproto.TimeCurrentTime)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Drag: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Drag did not finish")
	}
}

func waitCode(t *testing.T, code <-chan int) int {
	t.Helper()
	select {
	case c := <-code:
		return c
	case <-time.After(10 * time.Second):
		t.Fatal("the target did not exit")
		return -1
	}
}

func TestTargetReceivesDrop(t *testing.T) {
	const path = "/tmp/xdndtarget live drop.txt"
	out := filepath.Join(t.TempDir(), "drop.txt")
	code := startTarget(t, config{x: 10, y: 10, w: 100, h: 100, timeout: 20 * time.Second, wantPath: path, out: out})

	dragOnto(t, path)

	if c := waitCode(t, code); c != exitOK {
		t.Fatalf("exit code %d, want %d", c, exitOK)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if want := vtui.FormatURIList([]string{path}); string(got) != want {
		t.Fatalf("stored %q, want %q", got, want)
	}
}

func TestTargetFlagsWrongList(t *testing.T) {
	code := startTarget(t, config{x: 10, y: 10, w: 100, h: 100, timeout: 20 * time.Second, wantPath: "/tmp/xdndtarget expected.txt"})

	dragOnto(t, "/tmp/xdndtarget other.txt")

	if c := waitCode(t, code); c != exitMismatch {
		t.Fatalf("exit code %d, want %d", c, exitMismatch)
	}
}

func TestTargetTimesOutWithoutDrop(t *testing.T) {
	code := startTarget(t, config{x: 10, y: 10, w: 100, h: 100, timeout: 300 * time.Millisecond})
	if c := waitCode(t, code); c != exitFailure {
		t.Fatalf("exit code %d, want %d", c, exitFailure)
	}
}
