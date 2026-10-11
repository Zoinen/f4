package multiarc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// vfs_test.go never sets fakeBackend.listErr, so every method that starts
// with v.state.ensureListed(...) -- ReadDir, Stat, SetPath, Open -- only
// ever exercises the "listing succeeded" half of that call. This file
// covers the other half, plus the handful of MultiArcVFS methods that
// vfs_test.go and provider_test.go never touch at all (path helpers,
// GetCapabilities, Search, ParentVFS, Close, SetAttributes) and the two
// Open failure modes past a successful listing (extractOne itself failing,
// and the extracted member missing where extractOne said it would be).

var errListingFailed = errors.New("multiarc test: listing failed")

func TestMultiArcVFSListErrorPropagates(t *testing.T) {
	v := newTestVFS(t, fakeBackend{listErr: errListingFailed})
	ctx := context.Background()

	if err := v.ReadDir(ctx, "", func([]vfs.VFSItem) {}); !errors.Is(err, errListingFailed) {
		t.Errorf("ReadDir error = %v, want %v", err, errListingFailed)
	}
	if _, err := v.Stat(ctx, "/x"); !errors.Is(err, errListingFailed) {
		t.Errorf("Stat error = %v, want %v", err, errListingFailed)
	}
	if err := v.SetPath("/x"); !errors.Is(err, errListingFailed) {
		t.Errorf("SetPath error = %v, want %v", err, errListingFailed)
	}
	if _, err := v.Open(ctx, "/x"); !errors.Is(err, errListingFailed) {
		t.Errorf("Open error = %v, want %v", err, errListingFailed)
	}
}

// sync.Once means the backend is only ever asked to list once per state:
// a failed listing must keep failing the same way on every subsequent
// call, rather than being retried into a different (or nil-panicking)
// result.
func TestMultiArcVFSListErrorIsCachedAcrossCalls(t *testing.T) {
	v := newTestVFS(t, fakeBackend{listErr: errListingFailed})
	ctx := context.Background()

	first := v.ReadDir(ctx, "", func([]vfs.VFSItem) {})
	second := v.ReadDir(ctx, "", func([]vfs.VFSItem) {})
	if !errors.Is(first, errListingFailed) || !errors.Is(second, errListingFailed) {
		t.Fatalf("expected both calls to fail with the cached listing error, got %v then %v", first, second)
	}
}

func TestMultiArcVFSSetAttributesIsReadOnly(t *testing.T) {
	v := newTestVFS(t, fakeBackend{})
	if err := v.SetAttributes(context.Background(), "/x", vfs.VFSItem{}); !errors.Is(err, errReadOnly) {
		t.Errorf("SetAttributes error = %v, want %v", err, errReadOnly)
	}
}

func TestMultiArcVFSMiscAccessors(t *testing.T) {
	v := newTestVFS(t, fakeBackend{entries: []entry{{Path: "top.txt"}}})

	if !v.IsAtRoot() {
		t.Error("a freshly opened archive should report IsAtRoot")
	}
	if got := v.GetTitle(); got != "test.tar.gz" {
		t.Errorf("GetTitle = %q, want test.tar.gz", got)
	}
	if !v.IsAbs("/x") || v.IsAbs("x") {
		t.Errorf("IsAbs disagrees with path.IsAbs for /x and x")
	}
	if got := v.Join("a", "b", "c"); got != "a/b/c" {
		t.Errorf("Join = %q, want a/b/c", got)
	}
	if got := v.Base("/a/b/c.txt"); got != "c.txt" {
		t.Errorf("Base = %q, want c.txt", got)
	}
	if got := v.Dir("/a/b/c.txt"); got != "/a/b" {
		t.Errorf("Dir = %q, want /a/b", got)
	}
	if abs, err := v.Abs("sub"); err != nil || abs != "/sub" {
		t.Errorf("Abs(sub) = (%q, %v), want (/sub, nil)", abs, err)
	}
	if abs, err := v.Abs("/already/abs"); err != nil || abs != "/already/abs" {
		t.Errorf("Abs(/already/abs) = (%q, %v)", abs, err)
	}

	if err := v.SetPath("/"); err != nil {
		t.Fatalf("SetPath(/): %v", err)
	}
	// SetPath("") means "stay put": at the root, abs("") is still the root.
	if err := v.SetPath(""); err != nil {
		t.Fatalf("SetPath(\"\"): %v", err)
	}
	if !v.IsAtRoot() {
		t.Error("IsAtRoot should still hold after SetPath at the root")
	}

	if caps := v.GetCapabilities(); !caps.HasRandomAccess {
		t.Errorf("GetCapabilities = %#v, want HasRandomAccess", caps)
	}
	if ch, err := v.Search(context.Background(), "/", "*.txt"); ch != nil || err != nil {
		t.Errorf("Search = (%v, %v), want (nil, nil): multiarc never implements a native search", ch, err)
	}
	if v.ParentVFS() != nil {
		t.Error("ParentVFS should return the nil parent newTestVFS was built with")
	}
	if err := v.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
}

func TestMultiArcVFSOpenExtractOneFails(t *testing.T) {
	extractErr := errors.New("multiarc test: extractOne failed")
	b := fakeBackend{
		entries: []entry{{Path: "file.txt"}},
		extractOneFunc: func(context.Context, string, string, string) error {
			return extractErr
		},
	}
	v := newTestVFS(t, b)
	t.Cleanup(closeSharedMultiArcTempDirs)

	if _, err := v.Open(context.Background(), "/file.txt"); !errors.Is(err, extractErr) {
		t.Errorf("Open error = %v, want %v", err, extractErr)
	}
}

func TestMultiArcVFSOpenExtractedMemberMissing(t *testing.T) {
	// extractOne reports success without actually writing anything: Open
	// must still fail cleanly rather than handing back a *os.File on a
	// path that was never created.
	b := fakeBackend{
		entries: []entry{{Path: "file.txt"}},
		extractOneFunc: func(context.Context, string, string, string) error {
			return nil
		},
	}
	v := newTestVFS(t, b)
	t.Cleanup(closeSharedMultiArcTempDirs)

	if _, err := v.Open(context.Background(), "/file.txt"); err == nil {
		t.Error("expected an error when the extracted member is missing on disk")
	}
}

func TestMultiArcVFSOpenUnknownMember(t *testing.T) {
	v := newTestVFS(t, fakeBackend{entries: []entry{{Path: "file.txt"}}})
	if _, err := v.Open(context.Background(), "/nope.txt"); err == nil {
		t.Error("expected an error opening a member the archive does not have")
	}
}

// extractedFile.ReadAt is covered by TestMultiArcVFSOpenExtractsAndReads;
// Read (the sequential path) is not exercised anywhere.
func TestExtractedFileSequentialRead(t *testing.T) {
	b := fakeBackend{
		entries: []entry{{Path: "file.txt", Size: 5, SizeKnown: true}},
		extractOneFunc: func(ctx context.Context, localPath, destDir, member string) error {
			full := filepath.Join(destDir, filepath.FromSlash(member))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return err
			}
			return os.WriteFile(full, []byte("hello"), 0o600)
		},
	}
	v := newTestVFS(t, b)
	t.Cleanup(closeSharedMultiArcTempDirs)

	f, err := v.Open(context.Background(), "/file.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 5)
	n, err := f.Read(context.Background(), buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("Read content = %q, want hello", buf[:n])
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Read(canceled, buf); err == nil {
		t.Error("Read should honor an already-canceled context")
	}
	if _, err := f.ReadAt(canceled, buf, 0); err == nil {
		t.Error("ReadAt should honor an already-canceled context")
	}
}
