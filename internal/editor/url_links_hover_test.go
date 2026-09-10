package editor

import (
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/vtui"
)

// URL detection is tested in internal/viewer; these two hover cases drive an
// editor and a terminal view, which stayed here.

func TestEditorURLHoverAddsUnderlineOnlyToHoveredLink(t *testing.T) {
	const text = "https://example.org"
	links := viewer.FindURLLinks(text)
	ev := &EditorView{hoverURL: links[0].URL, TabSize: 8}
	cells := ev.fillCellsWithLinks(nil, []byte(text), terminal.DefaultTermAttr, terminal.DefaultTermAttr, 0, false, 0, 0, nil, links, 0, false, -1, 0, 0, 0)
	if len(cells) != len(text) {
		t.Fatalf("rendered %d cells, want %d", len(cells), len(text))
	}
	for i, cell := range cells {
		if cell.Attributes&vtui.CommonLvbUnderscore == 0 {
			t.Errorf("cell %d was not underlined", i)
		}
	}
	ev.hoverURL = "https://other.example"
	cells = ev.fillCellsWithLinks(nil, []byte(text), terminal.DefaultTermAttr, terminal.DefaultTermAttr, 0, false, 0, 0, nil, links, 0, false, -1, 0, 0, 0)
	for i, cell := range cells {
		if cell.Attributes&vtui.CommonLvbUnderscore != 0 {
			t.Errorf("cell %d was underlined for a different URL", i)
		}
	}
}
func TestTerminalURLHoverUnderlinesVisibleLink(t *testing.T) {
	tv := terminal.NewTerminalView(40, 3)
	defer tv.Close()
	tv.SetPosition(0, 0, 39, 2)
	tv.SetVisible(true)
	for i, r := range "https://example.org" {
		tv.Lines[0][i] = vtui.CharInfo{Char: testutil.Uint64Rune(r), Attributes: terminal.DefaultTermAttr}
	}
	if !tv.UpdateURLHover(4, 0) {
		t.Fatal("hover state did not change")
	}
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 3)
	theme.SetDefaultF4Palette()
	tv.Show(scr)
	if scr.GetCell(4, 0).Attributes&vtui.CommonLvbUnderscore == 0 {
		t.Fatal("hovered terminal URL was not underlined")
	}
	if tv.UpdateURLHover(30, 0) != true {
		t.Fatal("moving off the URL did not clear hover state")
	}
}
