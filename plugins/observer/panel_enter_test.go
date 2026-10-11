package observer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/plugins/observer"
	"github.com/unxed/f4/vfs"
)

// TestProvider_PanelEnterAllowed_DefaultMaskAllowsEverything checks the
// out-of-the-box behaviour (f4#1563 part 8): ObserverEnterExcludeMask is
// empty by default, so Enter keeps browsing any container Provider
// recognizes, exactly as it did before this mask existed.
func TestProvider_PanelEnterAllowed_DefaultMaskAllowsEverything(t *testing.T) {
	previous := config.App.ObserverEnterExcludeMask
	t.Cleanup(func() { config.App.ObserverEnterExcludeMask = previous })
	config.App.ObserverEnterExcludeMask = ""

	root := t.TempDir()
	isoPath := filepath.Join(root, "image.iso")
	if err := os.WriteFile(isoPath, []byte("not a real iso, the mask check never reads content"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := observer.NewProvider(t.TempDir())
	parent := vfs.NewOSVFS(root)

	if !p.PanelEnterAllowed(context.Background(), parent, "image.iso") {
		t.Error("PanelEnterAllowed = false, want true: an empty mask must bar nothing")
	}
}

// TestProvider_PanelEnterAllowed_MaskBarsMatchingNames checks that a
// configured ObserverEnterExcludeMask holds Enter back for a name it
// matches, case-insensitively, while a name outside the mask keeps ordinary
// Enter -- the same far2l file-mask semantics
// plugins/archive.ArchiveProvider already gives ArchiveEnterExcludeMask.
func TestProvider_PanelEnterAllowed_MaskBarsMatchingNames(t *testing.T) {
	previous := config.App.ObserverEnterExcludeMask
	t.Cleanup(func() { config.App.ObserverEnterExcludeMask = previous })
	config.App.ObserverEnterExcludeMask = "*.iso,*.nrg|keep.iso"

	root := t.TempDir()
	p := observer.NewProvider(t.TempDir())
	parent := vfs.NewOSVFS(root)
	ctx := context.Background()

	if p.PanelEnterAllowed(ctx, parent, "install.ISO") {
		t.Error("install.ISO: expected the mask to bar Enter (case-insensitive match)")
	}
	if p.PanelEnterAllowed(ctx, parent, "disk.nrg") {
		t.Error("disk.nrg: expected the mask to bar Enter")
	}
	if !p.PanelEnterAllowed(ctx, parent, "keep.iso") {
		t.Error("keep.iso: expected the exclude side of the mask (\"|keep.iso\") to take it back")
	}
	if !p.PanelEnterAllowed(ctx, parent, "readme.txt") {
		t.Error("readme.txt: expected a name outside the mask to keep ordinary Enter")
	}
}

// TestProvider_PanelEnterAllowed_NonLocalAlwaysAllowed checks that the mask
// never applies off the local file system (inside a nested archive, or on a
// remote panel), mirroring ArchiveProvider.PanelEnterAllowed's own reasoning:
// there is no association and no system opener there for Enter to defer to.
func TestProvider_PanelEnterAllowed_NonLocalAlwaysAllowed(t *testing.T) {
	previous := config.App.ObserverEnterExcludeMask
	t.Cleanup(func() { config.App.ObserverEnterExcludeMask = previous })
	config.App.ObserverEnterExcludeMask = "*.iso"

	p := observer.NewProvider(t.TempDir())
	if !p.PanelEnterAllowed(context.Background(), &vfs.NullVFS{}, "image.iso") {
		t.Error("PanelEnterAllowed = false off the local file system, want true regardless of the mask")
	}
}

// TestProvider_PanelEnterAllowed_CanceledContext mirrors CanOpen's own
// cancellation check: a context that is already done must not be treated as
// permission to enter.
func TestProvider_PanelEnterAllowed_CanceledContext(t *testing.T) {
	p := observer.NewProvider(t.TempDir())
	root := t.TempDir()
	parent := vfs.NewOSVFS(root)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if p.PanelEnterAllowed(ctx, parent, "image.iso") {
		t.Error("PanelEnterAllowed = true with a canceled context, want false")
	}
}
