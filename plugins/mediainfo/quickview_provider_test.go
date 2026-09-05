package mediainfo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestMediaQuickViewClaimsOnlyBinaryMedia(t *testing.T) {
	store, err := newSettingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plugin := &Plugin{store: store}
	provider := &mediaQuickViewProvider{plugin: plugin}
	fs := vfs.NewOSVFS(t.TempDir())

	for _, name := range []string{"movie.mp4", "sound.FLAC", "clip.webm"} {
		if !provider.CanPreview(vfs.QuickViewRequest{VFS: fs, Path: name, Item: vfs.VFSItem{Name: name}}) {
			t.Errorf("provider did not claim %q", name)
		}
	}
	for _, name := range []string{"poster.jpg", "captions.srt", "notes.txt", "folder.mp4"} {
		item := vfs.VFSItem{Name: name, IsDir: name == "folder.mp4"}
		if provider.CanPreview(vfs.QuickViewRequest{VFS: fs, Path: name, Item: item}) {
			t.Errorf("provider unexpectedly claimed %q", name)
		}
	}
}

func TestMediaQuickViewPreviewsWaveAndDeclinesCorruptContainer(t *testing.T) {
	directory := t.TempDir()
	store, err := newSettingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plugin := &Plugin{store: store, cache: newReportCache(4)}
	provider := &mediaQuickViewProvider{plugin: plugin}
	fs := vfs.NewOSVFS(directory)

	wavePath := filepath.Join(directory, "tone.wav")
	if err := os.WriteFile(wavePath, minimalWave(), 0o600); err != nil {
		t.Fatal(err)
	}
	waveItem, err := fs.Stat(context.Background(), wavePath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Preview(context.Background(), vfs.QuickViewRequest{VFS: fs, Path: wavePath, Item: waveItem})
	if err != nil {
		t.Fatal(err)
	}
	if result.Label != "Wave" || len(result.Lines) == 0 {
		t.Fatalf("preview = %#v", result)
	}

	brokenPath := filepath.Join(directory, "broken.wav")
	if err := os.WriteFile(brokenPath, []byte("RIFF\x0c\x00\x00\x00WAVEfmt \x64\x00\x00\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	brokenItem, err := fs.Stat(context.Background(), brokenPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Preview(context.Background(), vfs.QuickViewRequest{VFS: fs, Path: brokenPath, Item: brokenItem})
	if !errors.Is(err, vfs.ErrQuickViewUnsupported) {
		t.Fatalf("corrupt preview error = %v", err)
	}
}

func TestMediaQuickViewCanBeDisabledAtRuntime(t *testing.T) {
	store, err := newSettingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := store.snapshot()
	settings.EnableQuickView = false
	if err := store.save(settings); err != nil {
		t.Fatal(err)
	}
	plugin := &Plugin{store: store}
	provider := &mediaQuickViewProvider{plugin: plugin}
	if provider.CanPreview(vfs.QuickViewRequest{VFS: vfs.NewOSVFS(t.TempDir()), Path: "movie.mp4", Item: vfs.VFSItem{Name: "movie.mp4"}}) {
		t.Fatal("disabled provider claimed a file")
	}
}
