package main

import (
	"reflect"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func newSearchableHelpForTest(t *testing.T, lines []string) (*vtui.HelpView, *vtui.ScreenBuf) {
	return newSearchableHelpForTestAtSize(t, 80, 25, lines)
}

func newSearchableHelpForTestAtSize(t *testing.T, width, height int, lines []string) (*vtui.HelpView, *vtui.ScreenBuf) {
	t.Helper()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(width, height)
	vtui.FrameManager.Init(scr)
	engine := vtui.NewHelpEngine(&memoryHelpVFS{files: map[string]string{}})
	engine.AddTopic(&vtui.HelpTopic{Name: "Test", Lines: lines})
	oldEngine := vtui.GlobalHelpEngine
	vtui.GlobalHelpEngine = engine
	t.Cleanup(func() {
		vtui.GlobalHelpEngine = oldEngine
		currentHelpSearch = nil
		currentHelpZoom = nil
	})
	view := vtui.NewHelpView(engine, "Test")
	vtui.FrameManager.Push(view)
	return view, scr
}

func TestVisibleHelpLineRemovesFormatting(t *testing.T) {
	got, centered := visibleHelpLine("^#Title# ~Open file~Viewer@ now")
	if !centered {
		t.Fatal("center marker was not detected")
	}
	if got != "Title Open file now" {
		t.Fatalf("visible line = %q, want %q", got, "Title Open file now")
	}
}

func TestHelpSearchFindsCaseInsensitiveMatchesAndCycles(t *testing.T) {
	view, _ := newSearchableHelpForTest(t, []string{
		"No match here",
		"First Needle",
		"Second needle and NEEDLE",
	})

	for _, r := range "needle" {
		if !handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r}) {
			t.Fatalf("character %q was not consumed", r)
		}
	}
	if got := currentHelpSearch.matches[currentHelpSearch.selected]; got.line != 1 || got.start != 6 {
		t.Fatalf("first match = %#v, want line 1 at rune 6", got)
	}
	if !moveHelpSearch(view, false) || currentHelpSearch.matches[currentHelpSearch.selected].line != 2 || currentHelpSearch.matches[currentHelpSearch.selected].start != 7 {
		t.Fatalf("F3 did not advance to the next line: %#v", currentHelpSearch.matches[currentHelpSearch.selected])
	}
	if !moveHelpSearch(view, false) || currentHelpSearch.matches[currentHelpSearch.selected].start != 18 {
		t.Fatalf("F3 did not advance to the second occurrence: %#v", currentHelpSearch.matches[currentHelpSearch.selected])
	}
	if !moveHelpSearch(view, true) || currentHelpSearch.matches[currentHelpSearch.selected].start != 7 {
		t.Fatalf("Shift+F3 did not move backwards: %#v", currentHelpSearch.matches[currentHelpSearch.selected])
	}
}

func TestHelpSearchRendersHighlightAndHint(t *testing.T) {
	view, scr := newSearchableHelpForTest(t, []string{"before Needle after"})
	for _, r := range "needle" {
		handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}
	view.Show(scr)
	x1, y1, _, y2 := view.GetPosition()
	titleAttr := scr.GetCell((view.X1+view.X2)/2, y1).Attributes
	titleBackground := vtui.GetRGBBack(titleAttr)
	renderHelpSearch(scr)

	matchCell := scr.GetCell(x1+2+len("before "), y1+1)
	if got, want := vtui.GetRGBFore(matchCell.Attributes), uint32(0xFFFF00); got != want {
		t.Fatalf("current match foreground = %#x, want yellow %#x", got, want)
	}
	if got, want := vtui.GetRGBBack(matchCell.Attributes), vtui.GetRGBBack(vtui.Palette[vtui.ColHelpSelectedLink]); got != want {
		t.Fatalf("current match background = %#x, want selected background %#x", got, want)
	}
	foundHint := false
	for x := x1 + 2; x < view.X2-1; x++ {
		if testRune(scr.GetCell(x, y2).Char) == 'F' && testRune(scr.GetCell(x+1, y2).Char) == '3' {
			foundHint = true
			break
		}
	}
	if !foundHint {
		t.Fatal("search navigation hint was not rendered on the help border")
	}
	if got := scr.GetCell((x1+view.X2)/2, y2).Attributes; got != titleAttr {
		t.Fatalf("bottom hint attr = %#x, want title attr %#x", got, titleAttr)
	}
	foundHighlightedQuery := false
	for x := x1 + 2; x < view.X2; x++ {
		cell := scr.GetCell(x, y1)
		if testRune(cell.Char) == 'n' && vtui.GetRGBFore(cell.Attributes) == vtui.GetRGBFore(vtui.Palette[vtui.ColHelpLink]) {
			if got := vtui.GetRGBBack(cell.Attributes); got != titleBackground {
				t.Fatalf("query changed title background to %#x, want %#x", got, titleBackground)
			}
			foundHighlightedQuery = true
			break
		}
	}
	if !foundHighlightedQuery {
		t.Fatal("live query was not highlighted in the help title")
	}
}

func TestHelpSearchHotkeysRepeatAndBackspace(t *testing.T) {
	_, _ = newSearchableHelpForTest(t, []string{"one needle", "two needle"})
	for _, r := range "needle" {
		handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}
	if !handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, ControlKeyState: vtinput.LeftCtrlPressed}) {
		t.Fatal("Ctrl+Enter was not consumed by help search")
	}
	if currentHelpSearch.matches[currentHelpSearch.selected].line != 1 {
		t.Fatalf("Ctrl+Enter match line = %d, want 1", currentHelpSearch.matches[currentHelpSearch.selected].line)
	}
	if !handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed}) {
		t.Fatal("Ctrl+Shift+Enter was not consumed by help search")
	}
	if currentHelpSearch.matches[currentHelpSearch.selected].line != 0 {
		t.Fatalf("Ctrl+Shift+Enter match line = %d, want 0", currentHelpSearch.matches[currentHelpSearch.selected].line)
	}
	if !handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F3}) {
		t.Fatal("F3 was not consumed by help search")
	}
	if currentHelpSearch.matches[currentHelpSearch.selected].line != 1 {
		t.Fatalf("F3 match line = %d, want 1", currentHelpSearch.matches[currentHelpSearch.selected].line)
	}
	if !handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK}) {
		t.Fatal("Backspace was not consumed by help search")
	}
	if got := string(currentHelpSearch.query); got != "needl" {
		t.Fatalf("query after Backspace = %q, want needl", got)
	}
}

func TestHelpSearchEscapeClosesHelpImmediately(t *testing.T) {
	view, _ := newSearchableHelpForTest(t, []string{"one needle"})
	for _, r := range "needle" {
		handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}
	if handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}) {
		t.Fatal("Escape was consumed before HelpView could close")
	}
	if currentHelpSearch != nil {
		t.Fatal("Escape left Help search state active")
	}
	view.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	if vtui.FrameManager.GetTopFrame() != view || !view.IsDone() {
		t.Fatal("Escape did not close Help while clearing the search")
	}
}

func TestHelpBackspaceDoesNotCloseRootTopic(t *testing.T) {
	view, _ := newSearchableHelpForTest(t, []string{"root help"})
	backspace := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK}
	if !handleHelpSearchHotkey(backspace) {
		t.Fatal("Backspace on the root Help topic was not consumed")
	}
	if view.IsDone() || vtui.FrameManager.GetTopFrame() != view {
		t.Fatal("Backspace closed the root Help window")
	}
}

func TestHelpBackspaceStillReturnsToPreviousTopic(t *testing.T) {
	view, _ := newSearchableHelpForTest(t, []string{"root help"})
	vtui.GlobalHelpEngine.AddTopic(&vtui.HelpTopic{Name: "Second", Lines: []string{"second topic"}})
	view.SwitchTopic("Second")
	backspace := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK}
	if handleHelpSearchHotkey(backspace) {
		t.Fatal("Backspace with topic history should be passed to HelpView")
	}
	view.ProcessKey(backspace)
	historyLen, ok := nestedHelpLen(reflect.ValueOf(view), "history")
	if view.IsDone() || !ok || historyLen != 0 {
		t.Fatalf("Backspace did not return to the previous topic: history=%d, %v; done=%v", historyLen, ok, view.IsDone())
	}
}

func TestHelpSearchHighlightsAllVisibleMatches(t *testing.T) {
	view, scr := newSearchableHelpForTest(t, []string{"needle and NEEDLE"})
	for _, r := range "needle" {
		handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}
	view.Show(scr)
	renderHelpSearch(scr)
	x1, y1, _, _ := view.GetPosition()
	first := scr.GetCell(x1+2, y1+1)
	second := scr.GetCell(x1+2+len("needle and "), y1+1)
	if got, want := vtui.GetRGBFore(first.Attributes), uint32(0xFFFF00); got != want {
		t.Fatalf("current match foreground = %#x, want yellow %#x", got, want)
	}
	if got, want := vtui.GetRGBFore(second.Attributes), vtui.GetRGBFore(vtui.Palette[vtui.ColHelpLink]); got != want {
		t.Fatalf("other match foreground = %#x, want %#x", got, want)
	}
	if got, want := vtui.GetRGBBack(first.Attributes), vtui.GetRGBBack(vtui.Palette[vtui.ColHelpSelectedLink]); got != want {
		t.Fatalf("current match background = %#x, want %#x", got, want)
	}
	if got, want := vtui.GetRGBBack(second.Attributes), vtui.GetRGBBack(vtui.Palette[vtui.ColHelpText]); got != want {
		t.Fatalf("other match background = %#x, want unchanged %#x", got, want)
	}
}

func TestHelpSearchHighlightFollowsManualScrolling(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "ordinary line"
	}
	lines[0] = "needle at the top"
	view, scr := newSearchableHelpForTest(t, lines)
	vtui.FrameManager.Pop()
	wrapped := &struct{ *vtui.HelpView }{HelpView: view}
	vtui.FrameManager.Push(wrapped)
	for _, r := range "needle" {
		handleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}

	wrapped.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	wrapped.Show(scr)
	renderHelpSearch(scr)

	scrollTop, ok := helpViewScrollTop(wrapped)
	if !ok || scrollTop == 0 {
		t.Fatalf("HelpView scrollTop = %d, want a positive value after PageDown", scrollTop)
	}
	if currentHelpSearch.scrollTop != scrollTop {
		t.Fatalf("search scrollTop = %d, actual HelpView scrollTop = %d", currentHelpSearch.scrollTop, scrollTop)
	}
	x1, y1, _, _ := view.GetPosition()
	staleCell := scr.GetCell(x1+2, y1+1)
	if got := vtui.GetRGBFore(staleCell.Attributes); got == vtui.GetRGBFore(vtui.Palette[vtui.ColDialogHighlightText]) || got == vtui.GetRGBFore(vtui.Palette[vtui.ColHelpLink]) {
		t.Fatal("highlight from the old scroll position remained over the first visible row")
	}
}

func TestHelpSearchReadsScrollPositionFromEmbeddedHelpView(t *testing.T) {
	view, _ := newSearchableHelpForTest(t, make([]string, 40))
	wrapped := &struct{ *vtui.HelpView }{HelpView: view}
	wrapped.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	want, ok := helpViewScrollTop(view)
	if !ok || want == 0 {
		t.Fatalf("direct HelpView scrollTop = %d, want positive", want)
	}
	if got, ok := helpViewScrollTop(wrapped); !ok || got != want {
		t.Fatalf("embedded HelpView scrollTop = %d, %v; want %d, true", got, ok, want)
	}
}

func TestHelpShowsZoomButtonAndRestoresPreviousBounds(t *testing.T) {
	view, scr := newSearchableHelpForTestAtSize(t, 80, 40, []string{"Help text"})
	view.Show(scr)
	renderHelpSearch(scr)
	if !view.ShowZoom {
		t.Fatal("Help zoom support was not enabled")
	}
	x1, y1, x2, y2 := view.GetPosition()
	if got := testRune(scr.GetCell(x2-6, y1).Char); got != vtui.UIStrings.ZoomSymbol {
		t.Fatalf("zoom button symbol = %q, want %q", got, vtui.UIStrings.ZoomSymbol)
	}
	if !handleHelpSearchHotkey(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: testInt16(x2 - 6), MouseY: testInt16(y1),
	}) {
		t.Fatal("zoom button click was not handled")
	}
	maxX1, maxY1, _, _ := view.GetPosition()
	if maxX1 != 0 || maxY1 != 0 || currentHelpZoom == nil {
		t.Fatalf("zoomed Help bounds start at (%d,%d), zoom state=%v", maxX1, maxY1, currentHelpZoom)
	}
	_, _, zoomedX2, _ := view.GetPosition()
	if !handleHelpSearchHotkey(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: testInt16(zoomedX2 - 6), MouseY: 0,
	}) {
		t.Fatal("restore button click was not handled")
	}
	gotX1, gotY1, gotX2, gotY2 := view.GetPosition()
	if gotX1 != x1 || gotY1 != y1 || gotX2 != x2 || gotY2 != y2 || currentHelpZoom != nil {
		t.Fatalf("restored bounds=(%d,%d)-(%d,%d), want (%d,%d)-(%d,%d)", gotX1, gotY1, gotX2, gotY2, x1, y1, x2, y2)
	}
}
