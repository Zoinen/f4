package filemenu

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreparationCoalescesSelectionAndStopsOnClose(t *testing.T) {
	Close()
	old := desktopClient.command
	log := filepath.Join(t.TempDir(), "requests.jsonl")
	desktopClient.command = func(ctx context.Context) (*exec.Cmd, error) {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPersistentChild$")
		cmd.Env = append(os.Environ(), "F4_PERSISTENT_TEST_CHILD=1", "F4_PERSISTENT_TEST_LOG="+log)
		return cmd, nil
	}
	defer func() {
		Close()
		desktopClient.mu.Lock()
		desktopClient.command = old
		desktopClient.mu.Unlock()
		desktopClient.processMu.Lock()
		desktopClient.closed = false
		desktopClient.processMu.Unlock()
	}()
	EnablePreparation()
	var paths []string
	for _, name := range []string{"first.txt", "second.txt", "last.txt"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
		Prepare([]string{path})
	}
	var data []byte
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		data, _ = os.ReadFile(log)
		if strings.HasSuffix(string(data), "\n") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Count(string(data), "\n") != 1 {
		t.Fatalf("expected one prepared selection: %s", data)
	}
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	if request.Operation != "prepare" || len(request.Paths) != 1 || request.Paths[0] != paths[2] {
		t.Fatalf("prepared obsolete selection: %+v", request)
	}
	Prepare([]string{paths[0]})
	Close()
	time.Sleep(200 * time.Millisecond)
	after, _ := os.ReadFile(log)
	if string(after) != string(data) {
		t.Fatal("preparation continued after Close")
	}
	if desktopClient.process != nil {
		t.Fatal("helper survived Close")
	}
}
