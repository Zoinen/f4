//go:build !lite

package netfox

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestSMBLiveServer runs against a real SMB server when F4_SMB_TEST_URL names
// one (smb://user:pass@host/share, the share holding a.txt with "hello" in it
// and a subdirectory "sub"); the CI sandbox starts a local Samba for it.
func TestSMBLiveServer(t *testing.T) {
	raw := os.Getenv("F4_SMB_TEST_URL")
	if raw == "" {
		t.Skip("F4_SMB_TEST_URL is not set")
	}
	base, _, _ := strings.Cut(strings.TrimSuffix(raw, "/"), "/f4test")
	ctx := context.Background()

	root, err := (&smbURIProvider{}).OpenURI(ctx, nil, base)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	shares := readAll(t, root, "/")
	found := false
	for _, s := range shares {
		if s.Name == "f4test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("share f4test is not listed: %v", names(shares))
	}

	v, err := (&smbURIProvider{}).OpenURI(ctx, nil, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = v.Close() }()
	if got := v.GetPath(); got != "/f4test" {
		t.Errorf("path after opening the share URL = %q", got)
	}
	listing := readAll(t, v, "/f4test")
	if !strings.Contains(names(listing), "a.txt") {
		t.Fatalf("share listing = %q, want a.txt", names(listing))
	}
	f, err := v.Open(ctx, "/f4test/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	// Write, rename and delete on the live share (the sandbox makes it
	// writable for its user).
	if err := v.MkDir(ctx, "/f4test/newdir"); err != nil {
		t.Fatalf("MkDir: %v", err)
	}
	w, err := v.Create(ctx, "/f4test/newdir/w.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("written")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(ctx, "/f4test/newdir/w.txt", "/f4test/newdir/r.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if got := names(readAll(t, v, "/f4test/newdir")); got != "r.txt" {
		t.Errorf("newdir = %q, want r.txt", got)
	}
	if err := v.Remove(ctx, "/f4test/newdir"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := v.Stat(ctx, "/f4test/newdir"); err == nil {
		t.Error("newdir still there after Remove")
	}

	buf := make([]byte, 16)
	n, _ := f.ReadAt(ctx, buf, 1)
	if got := string(buf[:n]); !strings.HasPrefix(got, "ello") {
		t.Errorf("ReadAt(1) = %q, want ello...", got)
	}
}
