package netfox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestFishStreamingUsesSeparateSession(t *testing.T) {
	dial := localShellDialer(t)
	t.Setenv("F4_NO_FFINDJOB", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	v, err := NewFishVFSOnDialer(ctx, nil, dial, "search")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = v.Close() }()
	device := vfs.DevicePath{Scheme: "net", Device: "search", Domain: "fish"}
	v.SetDevicePath(device)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.txt"), []byte("needle"), 0600); err != nil {
		t.Fatal(err)
	}
	var hits int
	var progressPaths []string
	query := vfs.FindQuery{Masks: []string{"*.txt"}, Progress: func(p vfs.FindProgress) {
		progressPaths = append(progressPaths, p.Path)
	}}
	err = v.FindFilesStream(ctx, device.Public(root), query, func(h vfs.FoundEntry) {
		hits++
		// This request would deadlock until ctx expires if ffind held the
		// same session. The search callback still holds its own session lock.
		requestCtx, stop := context.WithTimeout(ctx, time.Second)
		defer stop()
		if _, err := v.client().Pwd(requestCtx); err != nil {
			t.Errorf("panel session unavailable during search: %v", err)
		}
		if h.Item.Name != "one.txt" {
			t.Errorf("unexpected hit: %+v", h)
		}
		if want := device.Public(filepath.Join(root, "one.txt")); h.Path != want {
			t.Errorf("result path=%q, want qualified path %q", h.Path, want)
		}
	})
	if err != nil || hits != 1 {
		t.Fatalf("hits=%d error=%v", hits, err)
	}
	if len(progressPaths) < 2 {
		t.Fatalf("missing initial/result progress: %v", progressPaths)
	}
	for _, p := range progressPaths {
		if !strings.HasPrefix(p, device.Public(root)) {
			t.Errorf("progress exposed an unqualified path: %q", p)
		}
	}
	if err := v.FindFilesStream(ctx, "net://another/fish/", query, func(vfs.FoundEntry) {
		t.Fatal("search accepted a path belonging to another connection")
	}); err == nil || errors.Is(err, vfs.ErrFindOptionsUnsupported) {
		t.Fatalf("wrong-connection path did not fail validation: %v", err)
	}
	// A provider opened on a one-off stream cannot create a second session.
	v.conn.mu.Lock()
	v.conn.dial, v.conn.dialAlt = nil, nil
	v.conn.mu.Unlock()
	err = v.FindFilesStream(ctx, root, vfs.FindQuery{Masks: []string{"*"}}, func(vfs.FoundEntry) { t.Fatal("unexpected hit without a dialer") })
	if !errors.Is(err, vfs.ErrFindOptionsUnsupported) {
		t.Fatalf("missing-dialer fallback: %v", err)
	}
}
