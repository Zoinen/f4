//go:build !extralite

package plughost

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

// TestScaffoldedPluginActuallyRuns is the test that matters. A scaffolder that
// emits a plugin which does not load would fail in front of the one audience
// least equipped to debug it.
func TestScaffoldedPluginActuallyRuns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	if _, err := ScaffoldPlugin(dir, "notes"); err != nil {
		t.Fatalf("ScaffoldPlugin: %v", err)
	}

	api := newLuaTestHostAPI()
	plugin := NewLuaPlugin(filepath.Join(dir, "plugin.lua"))
	if err := plugin.Init(api); err != nil {
		t.Fatalf("the scaffolded plugin did not load: %v", err)
	}
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close scaffolded plugin: %v", err)
		}
	})

	factory, ok := api.drives["notes"]
	if !ok {
		t.Fatalf("the scaffolded plugin did not mount its drive, got %v", api.drives)
	}

	fs := factory()
	ctx := context.Background()

	var items []vfs.VFSItem
	if err := fs.ReadDir(ctx, "/", func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("the drive listed %d files, want the ones the template writes", len(items))
	}

	File, err := fs.Open(ctx, items[0].Name)
	if err != nil {
		t.Fatalf("Open(%q): %v", items[0].Name, err)
	}
	defer func() { _ = File.Close() }()

	buf := make([]byte, File.Size())
	n, err := File.ReadAt(ctx, buf, 0)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "notes") {
		t.Errorf("the first file reads %q, want it to mention the plugin name", buf[:n])
	}
}
