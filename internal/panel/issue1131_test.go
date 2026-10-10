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

// typeIntoFilter types text the way it is typed into an open filter window:
// plain keys, no Alt.
func typeIntoFilter(t *testing.T, fp *FileSystemPanel, text string) {
	t.Helper()
	for _, r := range text {
		e := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r}
		if !fp.ProcessKey(e) {
			t.Fatalf("panel did not consume %q as filter input", r)
		}
	}
}

func openAutoFilter(t *testing.T, fp *FileSystemPanel) {
	t.Helper()
	fp.ToggleAutoFilter()
	if !fp.AutoFilterActive() {
		t.Fatal("ToggleAutoFilter did not open the filter")
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
// the filter window hides the rows that do not match rather than moving the
// cursor, and Esc gives them back.
func TestIssue1131AutofilterNarrowsPanel(t *testing.T) {
	fp := newAutoFilterPanel(t)
	openAutoFilter(t, fp)
	typeIntoFilter(t, fp, "alpha")

	if !fp.autoFilterOn {
		t.Fatalf("the filter did not narrow the panel: rows %v", panelNames(fp))
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
	if fp.autoFilterOn || fp.FastFindMode || fp.AutoFilterActive() {
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
	fp := newAutoFilterPanel(t)
	openAutoFilter(t, fp)
	typeIntoFilter(t, fp, "alpha")
	if !fp.autoFilterOn {
		t.Fatal("the filter did not narrow the panel")
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

// Navigation keys walk the narrowed list instead of closing the filter: that
// walk is what the filter is for. Erasing the whole query does not close it
// either -- the window stays up, shows every row and waits for a new query;
// only toggling it again (Alt, Ctrl+Alt+F), Esc or Enter closes it.
func TestIssue1131AutofilterStaysOpenUntilClosed(t *testing.T) {
	fp := newAutoFilterPanel(t)
	openAutoFilter(t, fp)

	down := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}
	// An empty filter hides nothing, and Down still does not close it.
	if !fp.ProcessKey(down) {
		t.Fatal("Down was not consumed by the panel")
	}
	if !fp.AutoFilterActive() {
		t.Fatal("Down closed the empty filter")
	}

	typeIntoFilter(t, fp, "alpha")
	// The filtered list is "..", then the two alpha rows; start on the first.
	fp.SetCursorIndex(1)
	before := fp.GetCursorIndex()
	if !fp.ProcessKey(down) {
		t.Fatal("Down was not consumed by the panel")
	}
	if !fp.autoFilterOn {
		t.Fatal("Down closed the filter")
	}
	if fp.GetCursorIndex() == before {
		t.Error("Down did not move the cursor inside the filtered list")
	}

	backspace := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK}
	for i := 0; i < len("alpha")+2; i++ {
		if !fp.ProcessKey(backspace) {
			t.Fatal("Backspace was not consumed by the filter")
		}
	}
	if !fp.AutoFilterActive() {
		t.Fatal("erasing the whole query closed the filter window")
	}
	if fp.autoFilterOn {
		t.Error("an empty filter is still hiding rows")
	}
	if len(fp.Entries) != len(autoFilterNames)+1 {
		t.Errorf("erasing the query restored %v", panelNames(fp))
	}

	typeIntoFilter(t, fp, "qqq")
	if len(fp.Entries) != 2 {
		t.Errorf("a new query after erasing shows %v, want \"..\" and qqq-data.bin", panelNames(fp))
	}

	fp.ToggleAutoFilter()
	if fp.AutoFilterActive() || fp.FastFindMode || fp.autoFilterOn {
		t.Fatal("toggling the filter again did not close it")
	}
	if len(fp.Entries) != len(autoFilterNames)+1 {
		t.Errorf("closing the filter restored %v", panelNames(fp))
	}
}

// Alt+letter is the quick search whether the autofilter is enabled or not:
// the row list is untouched and the cursor moves to the first match. The
// filter lives next to it, on a key of its own.
func TestIssue1131AltLetterStaysQuickSearch(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	for _, enabled := range []bool{false, true} {
		config.App.PanelAutoFilter = enabled

		fp := newAutoFilterPanel(t)
		typeIntoPanel(t, fp, "alpha")

		if fp.autoFilterOn || fp.AutoFilterActive() {
			t.Fatalf("enabled=%v: Alt+letter opened the filter", enabled)
		}
		if len(fp.Entries) != len(autoFilterNames)+1 {
			t.Fatalf("enabled=%v: quick search changed the row list: %v", enabled, panelNames(fp))
		}
		if fp.FastFindStr != "alpha" {
			t.Errorf("enabled=%v: quick search string is %q, want the plain query", enabled, fp.FastFindStr)
		}
		if name := fp.GetRawSelectedName(); name != "alpha.txt" && name != "alpha-notes.md" {
			t.Errorf("enabled=%v: cursor sits on %q, want a matching row", enabled, name)
		}

		// Backspace to nothing still ends a quick search, as it always did.
		backspace := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK}
		for i := 0; i < len("alpha"); i++ {
			fp.ProcessKey(backspace)
		}
		if fp.FastFindMode {
			t.Errorf("enabled=%v: erasing the quick search left it open", enabled)
		}
	}
}

// Opening the filter over a quick search in progress keeps what was typed
// and turns it into an unanchored filter.
func TestIssue1131FilterTakesOverQuickSearch(t *testing.T) {
	fp := newAutoFilterPanel(t)
	typeIntoPanel(t, fp, "qqq")
	openAutoFilter(t, fp)
	if fp.FastFindStr != "*qqq" {
		t.Errorf("filter query is %q, want the quick search's, unanchored", fp.FastFindStr)
	}
	if len(fp.Entries) != 2 {
		t.Errorf("filter over the quick search shows %v", panelNames(fp))
	}
}
