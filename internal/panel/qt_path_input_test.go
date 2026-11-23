package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestSemanticPanelPathInputNavigatesAndSelectsFile(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldSync := config.App.SyncPanelLoad
	config.App.SyncPanelLoad = false
	t.Cleanup(func() { config.App.SyncPanelLoad = oldSync })

	root := t.TempDir()
	dir := filepath.Join(root, "folder with spaces")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 48 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file-%02d.txt", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const leaf = "Z Archive.zip"
	file := filepath.Join(dir, leaf)
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)

	fsp := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(root))
	waitForLoad(t, fsp)
	t.Cleanup(func() {
		fsp.cancelProviderOpen()
		if fsp.CancelLoad != nil {
			fsp.CancelLoad()
		}
	})
	pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ActiveIdx: 0}

	submit := func(path string) {
		t.Helper()
		if !pf.HandleSemanticAction(map[string]any{
			"action": "panel.navigatePath",
			"side":   0,
			"path":   path,
			"typed":  true,
		}) {
			t.Fatalf("path input rejected %q", path)
		}
		waitForLoad(t, fsp)
	}

	input := file
	if vfs.WindowsPersonality() {
		input = "/" + filepath.ToSlash(filepath.Dir(file)) + `\` + strings.ToLower(leaf)
	}
	submit("  \"" + input + "\"  ")
	if got := fsp.Vfs.GetPath(); got != dir {
		t.Fatalf("path after typing %q = %q, want parent %q", input, got, dir)
	}
	if got := fsp.GetRawSelectedName(); got != leaf || fsp.Table.TopPos == 0 {
		t.Fatalf("cursor = %q, top row = %d, want %q revealed", got, fsp.Table.TopPos, leaf)
	}
	firstReveal := fsp.cursorRevealRevision()
	if firstReveal <= 0 {
		t.Fatal("file target did not request a viewport reveal")
	}

	submit("~")
	if got := fsp.Vfs.GetPath(); got != root {
		t.Fatalf("home path = %q, want %q", got, root)
	}
	submit("'folder with spaces'")
	if got := fsp.Vfs.GetPath(); got != dir {
		t.Fatalf("relative directory = %q, want %q", got, dir)
	}
	fsp.SetCursorIndex(0)
	submit("'" + leaf + "'")
	if got := fsp.GetRawSelectedName(); got != leaf {
		t.Fatalf("cursor in current folder = %q, want %q", got, leaf)
	}
	if got := fsp.cursorRevealRevision(); got <= firstReveal {
		t.Fatalf("viewport reveal revision = %d, want newer than %d", got, firstReveal)
	}
}
