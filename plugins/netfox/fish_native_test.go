package netfox

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/netfox/fishplus"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// nativeServerDialer connects a FishVFS to an in-process fishplus.Server the
// way an SSH session reaches f4 --fish-server.
func nativeServerDialer(dir string) FishDialer {
	return func(ctx context.Context) (io.Writer, io.Reader, io.Closer, error) {
		cr, sw := io.Pipe()
		sr, cw := io.Pipe()
		go func() {
			_ = (&fishplus.Server{Dir: dir}).Serve(sr, sw)
			_ = sw.Close()
		}()
		return cw, cr, nativeTestCloser(func() error { _ = cw.Close(); return cr.Close() }), nil
	}
}

type nativeTestCloser func() error

func (f nativeTestCloser) Close() error { return f() }

func TestFishVFSOverTheNativeServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FishVFS keeps remote paths in slash form with a leading /, which a Windows server does not take as absolute")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	native := fishplus.HandshakeOptions{Bootstrap: fishplus.BootstrapNative}
	v, err := NewFishVFSOnDialers(ctx, nil, nativeServerDialer(dir), native, nil, fishplus.HandshakeOptions{}, "test")
	if err != nil {
		t.Fatalf("NewFishVFSOnDialers: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })

	var names []string
	if err := v.ReadDir(ctx, dir, func(items []vfs.VFSItem) {
		for _, it := range items {
			names = append(names, it.Name)
		}
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(names) != 1 || names[0] != "a.txt" {
		t.Fatalf("listing = %v, want [a.txt]", names)
	}
	item, err := v.Stat(ctx, filepath.Join(dir, "a.txt"))
	if err != nil || item.Size != 5 {
		t.Fatalf("Stat = %#v, %v", item, err)
	}
}

// A peer without f4 fails the native attempt in whatever way a shell does; the
// connection then falls back to the alternative instead of giving up, but a
// failure to reach the peer at all is returned as it is.
func TestNativeAttemptFallsBackOnAPeerWithoutF4(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dir := t.TempDir()
	native := fishplus.HandshakeOptions{Bootstrap: fishplus.BootstrapNative}

	// The peer answers and closes at once, as "f4: command not found" does.
	noF4 := func(ctx context.Context) (io.Writer, io.Reader, io.Closer, error) {
		cr, sw := io.Pipe()
		sr, cw := io.Pipe()
		_ = sw.Close() // reading the peer ends at once
		_ = sr.Close() // and writing to it fails, instead of blocking with no reader
		return cw, cr, nativeTestCloser(func() error { return nil }), nil
	}
	v, err := NewFishVFSOnDialers(ctx, nil, noF4, native, nativeServerDialer(dir), native, "test")
	if err != nil {
		t.Fatalf("the fallback was not used: %v", err)
	}
	_ = v.Close()

	unreachable := errors.New("no route to host")
	dialFails := func(ctx context.Context) (io.Writer, io.Reader, io.Closer, error) { return nil, nil, nil, unreachable }
	if _, err := NewFishVFSOnDialers(ctx, nil, dialFails, native, nativeServerDialer(dir), native, "test"); !errors.Is(err, unreachable) {
		t.Fatalf("a dial failure = %v, want it returned as it is", err)
	}
}

func TestFishConnectionDialogCheckboxSavesTheRemoteF4Choice(t *testing.T) {
	if err := loadHostStrings(); err != nil {
		t.Skipf("host strings unavailable: %v", err)
	}
	ph := &fishProtocolHandler{}

	cfg := &NetFoxConfig{}
	ui, save := ph.BuildExtraUI(cfg, 0, 0, 56, 1)
	chk, ok := ui.(*vtui.Checkbox)
	if !ok || chk.State != 0 {
		t.Fatalf("a site without the option must show an unchecked box, got %#v", ui)
	}
	chk.State = 1
	save()
	if cfg.Options[fishRemoteF4Option] != "true" {
		t.Fatalf("Options = %v, want RemoteF4=true", cfg.Options)
	}

	ui, save = ph.BuildExtraUI(cfg, 0, 0, 56, 1)
	chk = ui.(*vtui.Checkbox)
	if chk.State != 1 {
		t.Fatal("a saved choice must show a checked box")
	}
	chk.State = 0
	save()
	if _, still := cfg.Options[fishRemoteF4Option]; still {
		t.Fatalf("Options = %v, want the option removed when unchecked", cfg.Options)
	}
}
