//go:build !windows

package terminal

import (
	"encoding/json"
	"fmt"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListSessionsIncludesLiveAndPurgesDeadMetadata(t *testing.T) {
	dir := sessionDir()
	stamp := time.Now().UnixNano()
	name := fmt.Sprintf("coverage-%d", stamp)
	liveJSON := filepath.Join(dir, "f4-"+name+"-live.json")
	liveSock := filepath.Join(dir, "f4-"+name+"-live.sock")
	deadJSON := filepath.Join(dir, "f4-"+name+"-dead.json")
	deadSock := filepath.Join(dir, "f4-"+name+"-dead.sock")
	malformedJSON := filepath.Join(dir, "f4-"+name+"-malformed.json")

	writeInfo := func(path string, info SessionInfo) {
		t.Helper()
		data, err := json.Marshal(info)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(liveSock, []byte("socket placeholder"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deadSock, []byte("stale socket placeholder"), 0600); err != nil {
		t.Fatal(err)
	}
	writeInfo(liveJSON, SessionInfo{PID: os.Getpid(), Title: "live", SockPath: liveSock})
	writeInfo(deadJSON, SessionInfo{PID: -1, Title: "dead", SockPath: deadSock})
	if err := os.WriteFile(malformedJSON, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, path := range []string{liveJSON, liveSock, deadJSON, deadSock, malformedJSON} {
			_ = os.Remove(path)
		}
	})

	sessions := listSessions()
	var foundLive bool
	for _, session := range sessions {
		if session.SockPath == liveSock {
			foundLive = true
			if session.Title != "live" || session.PID != os.Getpid() {
				t.Fatalf("live session = %+v, want original metadata", session)
			}
		}
		if session.SockPath == deadSock {
			t.Fatalf("stale session survived listSessions: %+v", session)
		}
	}
	if !foundLive {
		t.Fatalf("listSessions did not return live session %q", liveSock)
	}
	for _, path := range []string{deadJSON, deadSock} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("listSessions did not remove stale %s (err=%v)", path, err)
		}
	}
}

func TestWriteAndRemoveSessionInfo(t *testing.T) {
	oldManager := vtui.FrameManager
	vtui.FrameManager = vtui.NewFrameManager()
	t.Cleanup(func() { vtui.FrameManager = oldManager })

	dir := sessionDir()
	sockPath := filepath.Join(dir, fmt.Sprintf("f4-coverage-%d.sock", time.Now().UnixNano()))
	infoPath := filepath.Join(dir, fmt.Sprintf("f4-%d.json", os.Getpid()))
	if err := os.WriteFile(sockPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(infoPath)
		_ = os.Remove(sockPath)
	})

	writeSessionInfo(sockPath)
	data, err := os.ReadFile(infoPath)
	if err != nil {
		t.Fatalf("session metadata was not written: %v", err)
	}
	var info SessionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("session metadata is invalid JSON: %v", err)
	}
	if info.PID != os.Getpid() || info.Title != "f4" || info.SockPath != sockPath {
		t.Fatalf("session metadata = %+v, want current process and socket", info)
	}

	removeSessionInfo(sockPath)
	for _, path := range []string{infoPath, sockPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("removeSessionInfo left %s (err=%v)", path, err)
		}
	}
}

func TestSessionHelpersHandleInvalidDescriptorsAndLongStartupLogs(t *testing.T) {
	setCloseOnExec([]int{-1})

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := readEnd.Close(); err != nil {
		t.Fatal(err)
	}
	clearNonBlock(readEnd)
	if err := writeEnd.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "startup.log")
	const startupLogLimit = 4 << 10
	content := strings.Repeat("x", startupLogLimit+37)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got := readStartupLog(path)
	want := content[len(content)-startupLogLimit:]
	if got != want {
		t.Fatalf("readStartupLog long content length/value mismatch: got %d bytes, want %d", len(got), len(want))
	}
	removeStartupLog("")
	removeStartupLog(path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("removeStartupLog left %s (err=%v)", path, err)
	}
}

func TestRunClientReportsMissingServer(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "missing.sock")
	RunClient(sockPath, 0)
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("RunClient unexpectedly created server socket %s (err=%v)", sockPath, err)
	}
}

func TestParseAttachPayloadHandlesEmptyAndUnknownFirstLines(t *testing.T) {
	edit, left, right := parseAttachPayload("")
	if edit != "" || left != "" || right != "" {
		t.Fatalf("parseAttachPayload(\"\") = (%q, %q, %q), want all empty", edit, left, right)
	}

	edit, left, right = parseAttachPayload("UNKNOWN\nCWD /tmp\nCWD2 /var/tmp")
	if edit != "" || left != "/tmp" || right != "/var/tmp" {
		t.Fatalf("parseAttachPayload unknown first line = (%q, %q, %q), want empty edit and parsed directories", edit, left, right)
	}
}
