package observer

// This file is package observer (white-box, like
// plugins/archive/password_test.go) rather than package observer_test: it
// overrides observerPasswordPrompt and calls newObserverVFS directly, both
// unexported. It drives f4#1563's own generic BSD test fixture
// (testdata/stub/observer_stub.c, see its own doc comment) rather than the
// real isoimg.wasm, so this test needs no real password-protected container
// -- isoimg's own ISZ/AES support is stubbed out for licensing reasons (see
// doc.go/status/1563.md), so a genuinely password-protected ISO is not
// available to test against at all.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// stubFixturePath mirrors observer_test.go's own fixturePath constant (that
// one lives in package observer_test, so it is not visible from here).
const stubFixturePath = "testdata/observer_stub.wasm"

func loadStubFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(stubFixturePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("%s is missing; build it with scripts/build_observer_test_wasm.sh (requires wasi-sdk) before running this test -- CI does this in quick.yml/build.yml", stubFixturePath)
		}
		t.Fatalf("reading %s: %v", stubFixturePath, err)
	}
	return b
}

// stubPasswordMagic and stubTestPassword must match
// testdata/stub/observer_stub.c's own kMagicPassword/kTestPassword.
const (
	stubPasswordMagic = "F4OBSVP1"
	stubTestPassword  = "f4test-secret"
)

func writeStubPasswordProbe(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	content := append([]byte(stubPasswordMagic), []byte(" locked payload")...)
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatalf("writing %s: %v", p, err)
	}
	return p
}

// withObserverPasswordPrompt overrides observerPasswordPrompt for the
// duration of the test, restoring it on cleanup -- the same pattern
// plugins/archive's password tests use for archivePasswordPrompt.
func withObserverPasswordPrompt(t *testing.T, fn func(ctx context.Context, containerName string) (string, error)) {
	t.Helper()
	previous := observerPasswordPrompt
	observerPasswordPrompt = fn
	t.Cleanup(func() { observerPasswordPrompt = previous })
}

func TestOpenStorageWithPasswordPrompt_WrongThenRight(t *testing.T) {
	wasmBytes := loadStubFixture(t)
	dir := t.TempDir()
	path := writeStubPasswordProbe(t, dir, "secret.bin")
	ctx := context.Background()

	f, err := os.Open(path) //nolint:gosec // G304: path is this test's own t.TempDir() file.
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	ra := &fileReaderAt{f: f}

	mount := NewSingleFileFS(ctx, "secret.bin", ra)
	mod, err := LoadModule(ctx, wasmBytes, mount, nil)
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	defer func() { _ = mod.Close() }()
	if _, err := mod.LoadSubModule(""); err != nil {
		t.Fatalf("LoadSubModule: %v", err)
	}

	attempts := []string{"wrong-password", stubTestPassword}
	calls := 0
	withObserverPasswordPrompt(t, func(context.Context, string) (string, error) {
		if calls >= len(attempts) {
			t.Fatal("prompted for a password more times than expected")
		}
		p := attempts[calls]
		calls++
		return p, nil
	})

	res, err := openStorageWithPasswordPrompt(ctx, mod, "secret.bin", "/secret.bin")
	if err != nil {
		t.Fatalf("openStorageWithPasswordPrompt: %v", err)
	}
	if res.Code != SORSuccess {
		t.Errorf("Code = %d, want SORSuccess (%d)", res.Code, SORSuccess)
	}
	if calls != 2 {
		t.Errorf("prompted %d times, want 2 (one wrong, one right)", calls)
	}
}

func TestOpenStorageWithPasswordPrompt_Cancelled(t *testing.T) {
	wasmBytes := loadStubFixture(t)
	dir := t.TempDir()
	path := writeStubPasswordProbe(t, dir, "secret.bin")
	ctx := context.Background()

	f, err := os.Open(path) //nolint:gosec // G304: path is this test's own t.TempDir() file.
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	ra := &fileReaderAt{f: f}

	mount := NewSingleFileFS(ctx, "secret.bin", ra)
	mod, err := LoadModule(ctx, wasmBytes, mount, nil)
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	defer func() { _ = mod.Close() }()
	if _, err := mod.LoadSubModule(""); err != nil {
		t.Fatalf("LoadSubModule: %v", err)
	}

	withObserverPasswordPrompt(t, func(context.Context, string) (string, error) {
		return "", context.Canceled
	})

	_, err = openStorageWithPasswordPrompt(ctx, mod, "secret.bin", "/secret.bin")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestOpenStorageWithPasswordPrompt_NotNeeded(t *testing.T) {
	// The common case (no password at all) must never call the prompt.
	wasmBytes := loadStubFixture(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.bin")
	if err := os.WriteFile(path, []byte("F4OBSV01 unlocked payload"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	ctx := context.Background()

	f, err := os.Open(path) //nolint:gosec // G304: path is this test's own t.TempDir() file.
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	ra := &fileReaderAt{f: f}

	mount := NewSingleFileFS(ctx, "plain.bin", ra)
	mod, err := LoadModule(ctx, wasmBytes, mount, nil)
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	defer func() { _ = mod.Close() }()
	if _, err := mod.LoadSubModule(""); err != nil {
		t.Fatalf("LoadSubModule: %v", err)
	}

	withObserverPasswordPrompt(t, func(context.Context, string) (string, error) {
		t.Fatal("prompt called for a container that needs no password")
		return "", nil
	})

	res, err := openStorageWithPasswordPrompt(ctx, mod, "plain.bin", "/plain.bin")
	if err != nil {
		t.Fatalf("openStorageWithPasswordPrompt: %v", err)
	}
	if res.Code != SORSuccess {
		t.Errorf("Code = %d, want SORSuccess (%d)", res.Code, SORSuccess)
	}
}

func TestProvider_CanOpen_PasswordProtected(t *testing.T) {
	wasmBytes := loadStubFixture(t)
	modulesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modulesDir, isoimgModuleFileName), wasmBytes, 0o600); err != nil {
		t.Fatalf("writing %s: %v", isoimgModuleFileName, err)
	}
	dir := t.TempDir()
	isoPath := writeStubPasswordProbe(t, dir, "secret.iso")
	parent := vfs.NewOSVFS(dir)
	p := NewProvider(modulesDir)

	if !p.CanOpen(context.Background(), parent, isoPath) {
		t.Error("CanOpen = false, want true: a password-protected container should still be recognized, so Enter reaches Open and can prompt for its password")
	}
}

func TestNewObserverVFS_PasswordPrompt(t *testing.T) {
	wasmBytes := loadStubFixture(t)
	dir := t.TempDir()
	path := writeStubPasswordProbe(t, dir, "secret.iso")
	parent := vfs.NewOSVFS(dir)

	attempts := []string{"wrong-password", stubTestPassword}
	calls := 0
	withObserverPasswordPrompt(t, func(context.Context, string) (string, error) {
		if calls >= len(attempts) {
			t.Fatal("prompted for a password more times than expected")
		}
		p := attempts[calls]
		calls++
		return p, nil
	})

	v, err := newObserverVFS(context.Background(), parent, path, wasmBytes, "", "")
	if err != nil {
		t.Fatalf("newObserverVFS: %v", err)
	}
	defer func() { _ = v.Close() }()

	if calls != 2 {
		t.Errorf("prompted %d times, want 2 (one wrong, one right)", calls)
	}
}

func TestNewObserverVFS_PasswordCancelled(t *testing.T) {
	wasmBytes := loadStubFixture(t)
	dir := t.TempDir()
	path := writeStubPasswordProbe(t, dir, "secret.iso")
	parent := vfs.NewOSVFS(dir)

	withObserverPasswordPrompt(t, func(context.Context, string) (string, error) {
		return "", context.Canceled
	})

	_, err := newObserverVFS(context.Background(), parent, path, wasmBytes, "", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("newObserverVFS error = %v, want context.Canceled", err)
	}
}

// fileReaderAt adapts an *os.File to observer.ReaderAt without depending on
// vfs.NewOSVFS's own Open (whose returned type this test does not need for
// the low-level openStorageWithPasswordPrompt tests, only for the
// newObserverVFS ones above, which go through parent.Open instead).
type fileReaderAt struct {
	f *os.File
}

func (r *fileReaderAt) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	return r.f.ReadAt(p, off)
}

func (r *fileReaderAt) Read(_ context.Context, p []byte) (int, error) {
	return r.f.Read(p)
}

func (r *fileReaderAt) Close() error { return nil }

func (r *fileReaderAt) Size() int64 {
	info, err := r.f.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}
