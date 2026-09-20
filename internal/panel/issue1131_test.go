package panel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// autoFilterNames are chosen so that "alpha" matches exactly two of them under
// the fuzzy matcher: the other two share no substring anywhere near it.
var autoFilterNames = []string{"alpha.txt", "alpha-notes.md", "zzz-report.log", "qqq-data.bin"}

func newAutoFilterPanel(t *testing.T) *FileSystemPanel {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	root := t.TempDir()
	for _, name := range autoFilterNames {
		// #nosec G703 -- the path is inside the private test temp directory.
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	fp := NewFileSystemPanel(0, 0, 80, 25, vfs.NewOSVFS(root))
	t.Cleanup(func() {
		fp.cancelProviderOpen()
		if fp.Vfs != nil {
			_ = fp.Vfs.Close()
		}
		if fp.CancelLoad != nil {
			fp.CancelLoad()
		}
		fp.StopLoadingAnimation()
	})
	fp.SetFocus(true)
	waitForLoad(t, fp)
	if len(fp.Entries) != len(autoFilterNames)+1 {
		t.Fatalf("panel loaded %d rows, want %d plus \"..\"", len(fp.Entries), len(autoFilterNames))
	}
	return fp
}

func typeIntoPanel(t *testing.T, fp *FileSystemPanel, text string) {
	t.Helper()
	for _, r := range text {
		e := &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			Char:            r,
			ControlKeyState: vtinput.LeftAltPressed,
		}
		if !fp.ProcessKey(e) {
			t.Fatalf("panel did not consume %q as quick-search input", r)
		}
	}
}

func panelNames(fp *FileSystemPanel) []string {
	names := make([]string, 0, len(fp.Entries))
	for _, entry := range fp.Entries {
		names = append(names, entry.Name)
	}
	return names
}

// TestIssue1131AutofilterNarrowsPanel is the regression test for issue #1131:
// with the option on, the quick search hides the rows that do not match rather
// than moving the cursor, and Esc gives them back.
func TestIssue1131AutofilterNarrowsPanel(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.PanelAutoFilter = true

	fp := newAutoFilterPanel(t)
	typeIntoPanel(t, fp, "alpha")

	if !fp.autoFilterOn {
		t.Fatalf("quick search did not narrow the panel: rows %v", panelNames(fp))
	}
	// ".." plus the two matching names, whatever the sort order puts first.
	if len(fp.Entries) != 3 {
		t.Fatalf("filtered panel shows %v, want \"..\" and the two alpha rows", panelNames(fp))
	}
	if fp.Entries[0].Name != ".." {
		t.Errorf("filtered panel lost \"..\": %v", panelNames(fp))
	}
	for _, entry := range fp.Entries[1:] {
		if entry.Name != "alpha.txt" && entry.Name != "alpha-notes.md" {
			t.Errorf("non-matching row %q survived the filter", entry.Name)
		}
	}
	if len(fp.AllEntries()) != len(autoFilterNames)+1 {
		t.Errorf("the complete list lost rows: %d, want %d", len(fp.AllEntries()), len(autoFilterNames)+1)
	}
	if name := fp.GetRawSelectedName(); name != "alpha.txt" && name != "alpha-notes.md" {
		t.Errorf("cursor sits on %q, want a matching row", name)
	}

	escape := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
	focused := fp.GetRawSelectedName()
	if !fp.ProcessKey(escape) {
		t.Fatal("Esc was not consumed by the filter")
	}
	if fp.autoFilterOn || fp.FastFindMode {
		t.Fatal("Esc left the filter up")
	}
	if len(fp.Entries) != len(autoFilterNames)+1 {
		t.Fatalf("Esc restored %v, want every row back", panelNames(fp))
	}
	if got := fp.GetRawSelectedName(); got != focused {
		t.Errorf("Esc moved the cursor from %q to %q", focused, got)
	}
}

// A directory re-read while the filter is up -- a chunked load finishing, or
// the two-second mtime refresh -- must fill the complete list, not the narrowed
// one. Getting this wrong silently drops the hidden rows for good.
func TestIssue1131AutofilterSurvivesDirectoryReload(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.PanelAutoFilter = true

	fp := newAutoFilterPanel(t)
	typeIntoPanel(t, fp, "alpha")
	if !fp.autoFilterOn {
		t.Fatal("quick search did not narrow the panel")
	}

	fp.ReadDirectory()
	waitForLoad(t, fp)

	if !fp.autoFilterOn {
		t.Fatal("re-reading the same directory closed the filter")
	}
	if len(fp.Entries) != 3 {
		t.Errorf("after the reload the panel shows %v, want the filtered rows", panelNames(fp))
	}
	if len(fp.AllEntries()) != len(autoFilterNames)+1 {
		t.Fatalf("the reload rebuilt only the narrowed list: %d rows, want %d",
			len(fp.AllEntries()), len(autoFilterNames)+1)
	}

	fp.ExitFastFind()
	if len(fp.Entries) != len(autoFilterNames)+1 {
		t.Errorf("leaving the filter after a reload restored %v", panelNames(fp))
	}
}

// Navigation keys walk the narrowed list instead of closing the search: that
// walk is what the filter is for.
func TestIssue1131AutofilterKeepsFilterOnNavigation(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.PanelAutoFilter = true

	fp := newAutoFilterPanel(t)
	typeIntoPanel(t, fp, "alpha")
	before := fp.GetCursorIndex()

	down := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}
	if !fp.ProcessKey(down) {
		t.Fatal("Down was not consumed by the panel")
	}
	if !fp.autoFilterOn {
		t.Fatal("Down closed the filter")
	}
	if fp.GetCursorIndex() == before {
		t.Error("Down did not move the cursor inside the filtered list")
	}

	// Erasing back to a single character leaves no query at all -- the seeded
	// '*' is not one -- so the search ends and every row comes back.
	backspace := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK}
	for i := 0; i < len("alpha"); i++ {
		if !fp.ProcessKey(backspace) {
			t.Fatal("Backspace was not consumed by the filter")
		}
	}
	if fp.FastFindMode || fp.autoFilterOn {
		t.Fatal("erasing the whole query left the search open")
	}
	if len(fp.Entries) != len(autoFilterNames)+1 {
		t.Errorf("erasing the query restored %v", panelNames(fp))
	}
}

// With the option off the quick search behaves exactly as before: the row list
// is untouched and the cursor moves to the first match.
func TestIssue1131QuickSearchUnchangedWhenOff(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.PanelAutoFilter = false

	fp := newAutoFilterPanel(t)
	typeIntoPanel(t, fp, "alpha")

	if fp.autoFilterOn {
		t.Fatal("the panel was narrowed although the option is off")
	}
	if len(fp.Entries) != len(autoFilterNames)+1 {
		t.Fatalf("quick search changed the row list: %v", panelNames(fp))
	}
	if fp.FastFindStr != "alpha" {
		t.Errorf("quick search string is %q, want the plain query", fp.FastFindStr)
	}
	if name := fp.GetRawSelectedName(); name != "alpha.txt" && name != "alpha-notes.md" {
		t.Errorf("cursor sits on %q, want a matching row", name)
	}
}
