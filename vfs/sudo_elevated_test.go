package vfs

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startElevatedTestServer serves token on a short socket path (macOS limits
// them) and returns where it listens and a channel that reports ServeElevated's
// result.
func startElevatedTestServer(t *testing.T, token string, handle func(SudoRequest) SudoResponse) (string, chan error) {
	t.Helper()
	dir, err := os.MkdirTemp("", "f4s")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	addr, err := net.ResolveUnixAddr("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	done := make(chan error, 1)
	go func() { done <- ServeElevated(l, token, handle) }()
	return sock, done
}

func TestElevatedSessionNeedsTheToken(t *testing.T) {
	token, err := NewSudoToken()
	if err != nil {
		t.Fatal(err)
	}
	sock, done := startElevatedTestServer(t, token, nil)

	if _, err := DialElevated(sock, "not-the-token", 2*time.Second); !errors.Is(err, ErrSudoAccessDenied) {
		t.Fatalf("a wrong token was answered with %v, want ErrSudoAccessDenied", err)
	}
	// The wrong guess did not end the dispatcher: the right token still works.
	conn, err := DialElevated(sock, token, 2*time.Second)
	if err != nil {
		t.Fatalf("right token refused: %v", err)
	}
	if err := sendMsg(conn, SudoRequest{Cmd: CmdPing}, -1); err != nil {
		t.Fatal(err)
	}
	var resp SudoResponse
	if _, err := recvMsg(conn, &resp); err != nil || resp.Error != "" {
		t.Fatalf("ping = %+v, %v", resp, err)
	}
	_ = conn.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServeElevated ended with %v after a clean disconnect", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the dispatcher kept running after its client left")
	}
}

func TestElevatedSessionPassesRequestsToTheHandler(t *testing.T) {
	token, _ := NewSudoToken()
	var seen []SudoRequest
	sock, _ := startElevatedTestServer(t, token, func(req SudoRequest) SudoResponse {
		seen = append(seen, req)
		return SudoResponse{Item: VFSItem{Name: req.Path}}
	})
	conn, err := DialElevated(sock, token, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	if err := sendMsg(conn, SudoRequest{Cmd: CmdStat, Path: `C:\Windows\x`}, -1); err != nil {
		t.Fatal(err)
	}
	var resp SudoResponse
	if _, err := recvMsg(conn, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Item.Name != `C:\Windows\x` || len(seen) != 1 || seen[0].Cmd != CmdStat {
		t.Fatalf("handler saw %+v, answered %+v", seen, resp)
	}

	// A second hello on an open session is not a way to change anything.
	if err := sendMsg(conn, SudoRequest{Cmd: CmdHello, Path: token}, -1); err != nil {
		t.Fatal(err)
	}
	resp = SudoResponse{}
	if _, err := recvMsg(conn, &resp); err != nil || resp.Error == "" {
		t.Fatalf("a repeated hello was answered with %+v, %v", resp, err)
	}
	if len(seen) != 1 {
		t.Fatal("the repeated hello reached the handler")
	}
}

func TestElevatedSessionWithoutHandlerRefusesWork(t *testing.T) {
	token, _ := NewSudoToken()
	sock, _ := startElevatedTestServer(t, token, nil)
	conn, err := DialElevated(sock, token, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = sendMsg(conn, SudoRequest{Cmd: CmdRemove, Path: "x"}, -1)
	var resp SudoResponse
	if _, err := recvMsg(conn, &resp); err != nil || resp.Error == "" {
		t.Fatalf("work was accepted by a dispatcher with no handler: %+v, %v", resp, err)
	}
}

func TestElevatedFrameRefusesAnOversizedLength(t *testing.T) {
	prefix := []byte{0xff, 0xff, 0xff, 0x7f}
	var resp SudoResponse
	err := readSudoFrame(bytes.NewReader(prefix), &resp)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("an oversized frame length was accepted: %v", err)
	}
	var buf bytes.Buffer
	if err := writeSudoFrame(&buf, SudoRequest{Cmd: CmdStat, Path: "p"}); err != nil {
		t.Fatal(err)
	}
	var req SudoRequest
	if err := readSudoFrame(&buf, &req); err != nil || req.Path != "p" || req.Cmd != CmdStat {
		t.Fatalf("round trip = %+v, %v", req, err)
	}
}

func TestSudoTokensAreLongAndDistinct(t *testing.T) {
	a, errA := NewSudoToken()
	b, errB := NewSudoToken()
	if errA != nil || errB != nil || len(a) != 2*sudoTokenBytes || a == b {
		t.Fatalf("tokens %q %q (%v, %v)", a, b, errA, errB)
	}
}
