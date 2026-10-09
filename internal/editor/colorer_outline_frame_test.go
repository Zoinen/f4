package editor

import (
	"strings"
	"testing"

	colorer "github.com/unxed/colorer4go"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func outlineKey(vk uint16, char rune, ctrl bool) *vtinput.InputEvent {
	e := &vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vk, Char: char}
	if ctrl {
		e.ControlKeyState = vtinput.LeftCtrlPressed
	}
	return e
}

// An editor over "fn alpha / fn alphabet / fn beta / fn beta2" at lines 0, 2,
// 4, 6, with beta nested one level deeper.
func outlineFrameFixture(t *testing.T) (*EditorView, *colorerOutlineFrame) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	old := config.App.EditorColorerOldOutline
	config.App.EditorColorerOldOutline = false
	t.Cleanup(func() { config.App.EditorColorerOldOutline = old })

	ev := NewEditorView(piecetable.New([]byte("fn alpha\n\nfn alphabet\n\nfn beta\n\nfn beta2\n")), nil, "a.txt")
	t.Cleanup(func() { ev.Close() })
	ev.SetPosition(0, 0, 79, 24)
	entry := func(line, level int, label string) colorerOutlineEntry {
		return colorerOutlineEntry{line: line, label: label, item: colorer.OutlineItem{Region: "def:Function", Start: 3, Level: level}}
	}
	entries := []colorerOutlineEntry{entry(0, 1, "alpha"), entry(2, 1, "alphabet"), entry(4, 2, "beta"), entry(6, 2, "beta2")}
	ev.CursorLine, ev.CursorPos = 5, 0
	return ev, newColorerOutlineFrame(ev, entries)
}

func outlineLabels(f *colorerOutlineFrame) []string {
	var labels []string
	for _, row := range f.rows {
		labels = append(labels, f.entries[row].label)
	}
	return labels
}

func TestColorerOutlineFrame_FilterBackspaceAndTab(t *testing.T) {
	_, f := outlineFrameFixture(t)
	if got := outlineLabels(f); len(got) != 4 || f.SelectPos != 2 {
		t.Fatalf("rows %v selected %d, want all four with beta (the item above line 5) selected", got, f.SelectPos)
	}
	f.key(outlineKey(0, 'A', false))
	f.key(outlineKey(0, 'l', false))
	if got := outlineLabels(f); strings.Join(got, ",") != "alpha,alphabet" {
		t.Fatalf("filter %q rows %v", string(f.filter), got)
	}
	if want := "al?pha"; !strings.HasSuffix(f.title(), want) {
		t.Errorf("title %q, want the completion %q", f.title(), want)
	}
	f.key(outlineKey(vtinput.VK_TAB, 0, false))
	if string(f.filter) != "alpha" {
		t.Errorf("Tab set the filter to %q, want alpha", string(f.filter))
	}
	f.key(outlineKey(0, 'x', false)) // matches nothing: dropped again
	if string(f.filter) != "alpha" || len(f.rows) != 2 {
		t.Errorf("filter %q rows %v after a character nothing matches", string(f.filter), outlineLabels(f))
	}
	for range 5 {
		f.key(outlineKey(vtinput.VK_BACK, 0, false))
	}
	if len(f.filter) != 0 || len(f.rows) != 4 {
		t.Errorf("filter %q rows %v after Backspace", string(f.filter), outlineLabels(f))
	}
}

func TestColorerOutlineFrame_LevelsPreviewAndEscape(t *testing.T) {
	ev, f := outlineFrameFixture(t)
	f.key(outlineKey(vtinput.VK_LEFT, 0, true))
	if got := outlineLabels(f); strings.Join(got, ",") != "alpha,alphabet" {
		t.Fatalf("Ctrl+Left rows %v, want the top level only", got)
	}
	f.key(outlineKey(vtinput.VK_RIGHT, 0, true))
	if len(f.rows) != 4 {
		t.Fatalf("Ctrl+Right rows %v, want all four again", outlineLabels(f))
	}

	f.key(outlineKey(vtinput.VK_DOWN, 0, true))
	if ev.CursorLine != 6 || ev.CursorPos != 3 {
		t.Fatalf("Ctrl+Down put the cursor at %d:%d, want beta2 at 6:3", ev.CursorLine, ev.CursorPos)
	}
	f.key(outlineKey(vtinput.VK_DOWN, 0, true)) // wraps to the first
	if ev.CursorLine != 0 {
		t.Fatalf("Ctrl+Down from the last item went to line %d, want 0", ev.CursorLine)
	}
	f.key(outlineKey(vtinput.VK_ESCAPE, 0, false))
	if ev.CursorLine != 5 || ev.CursorPos != 0 {
		t.Errorf("Esc left the cursor at %d:%d, want it back at 5:0", ev.CursorLine, ev.CursorPos)
	}
}

func TestColorerOutlineFrame_EnterAndCtrlEnter(t *testing.T) {
	ev, f := outlineFrameFixture(t)
	f.key(outlineKey(vtinput.VK_RETURN, 0, false))
	if ev.CursorLine != 4 || ev.CursorPos != 3 {
		t.Fatalf("Enter put the cursor at %d:%d, want beta at 4:3", ev.CursorLine, ev.CursorPos)
	}

	ev2, g := outlineFrameFixture(t)
	g.key(outlineKey(vtinput.VK_RETURN, 0, true))
	text, _ := ev2.lineTextForHighlight(5)
	if strings.TrimRight(text, "\n") != "beta" || ev2.CursorLine != 5 {
		t.Errorf("Ctrl+Enter made line 5 %q with the cursor on line %d, want the label inserted there", text, ev2.CursorLine)
	}
}

func TestColorerOutlineAutoFilter(t *testing.T) {
	rows := []string{"  1   f alpha", "  3   f alphabet"}
	if got := string(colorerOutlineAutoFilter(rows, []rune("AL"))); got != "alpha" {
		t.Errorf("completion %q, want alpha", got)
	}
	if got := string(colorerOutlineAutoFilter(rows[:1], []rune("al"))); got != "al" {
		t.Errorf("one row completed to %q; FarColorer completes only among several", got)
	}
}
