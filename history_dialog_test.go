package main

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func historyKey(char rune) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: char}
}

func TestHistorySearchFiltersAndTogglesPrefixMode(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	menu := vtui.NewVMenu("History")
	search := newHistorySearch(menu, []HistoryRecord{{Name: "xGIT status"}, {Name: "Git commit"}, {Name: "dir"}}, "F2: switch search mode")
	defer search.cleanup()

	for _, r := range "git" {
		if !search.processKey(historyKey(r)) {
			t.Fatalf("character %q was not handled", r)
		}
	}
	if got := len(menu.Items); got != 2 {
		t.Fatalf("substring filter returned %d items, want 2", got)
	}

	search.processKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F2})
	if got := len(menu.Items); got != 1 || menu.Items[0].Text != "Git commit" {
		t.Fatalf("prefix filter returned %#v, want Git commit", menu.Items)
	}
	if search.displayTitle() != "History [git*]" {
		t.Fatalf("prefix marker title = %q, want %q", search.displayTitle(), "History [git*]")
	}

	search.processKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK})
	if string(search.query) != "gi" {
		t.Fatalf("Backspace left query %q, want gi", string(search.query))
	}
}

func TestHistorySearchUsesOriginalIndexWhenFiltered(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	menu := vtui.NewVMenu("History")
	search := newHistorySearch(menu, []HistoryRecord{{Name: "first"}, {Name: "keep"}, {Name: "second match"}}, "F2: switch search mode")
	defer search.cleanup()

	for _, r := range "match" {
		search.processKey(historyKey(r))
	}
	if len(menu.Items) != 1 || !search.deleteSelected() {
		t.Fatal("failed to select and delete the filtered entry")
	}
	if len(search.all) != 2 || search.all[0].Name != "first" || search.all[1].Name != "keep" {
		t.Fatalf("wrong original entry deleted: %#v", search.all)
	}
}

func TestHistorySearchMatchIsCaseInsensitiveAndHighlightsAllMatches(t *testing.T) {
	matched, highlights := historySearchMatch("АбАба", []rune("аБ"), false)
	if !matched {
		t.Fatal("case-insensitive Unicode substring did not match")
	}
	want := []bool{true, true, true, true, false}
	if len(highlights) != len(want) {
		t.Fatalf("highlight mask length = %d, want %d", len(highlights), len(want))
	}
	for i := range want {
		if highlights[i] != want[i] {
			t.Fatalf("highlight mask = %#v, want %#v", highlights, want)
		}
	}

	if matched, _ := historySearchMatch("xAbc", []rune("abc"), true); matched {
		t.Fatal("prefix mode accepted a match that does not start at position zero")
	}
}

func TestHistorySearchDrawHighlightsMatchAndSearchTitle(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 10)
	vtui.FrameManager.Init(scr)
	menu := vtui.NewVMenu("History")
	menu.SetPosition(2, 2, 30, 7)
	search := newHistorySearch(menu, []HistoryRecord{{Name: "before MATCH after"}}, "F2: switch search mode")
	defer search.cleanup()
	for _, r := range "match" {
		search.processKey(historyKey(r))
	}
	menu.Show(scr)
	search.draw(scr)

	// Row begins with three padding cells (space, lock, space). "MATCH" starts after "before ".
	matchX := menu.X1 + 4 + len("before ")
	if got := scr.GetCell(matchX, menu.Y1+1).Attributes; got != vtui.Palette[vtui.ColMenuSelectedHighlight] {
		t.Fatalf("match color = %#x, want %#x", got, vtui.Palette[vtui.ColMenuSelectedHighlight])
	}
	if got := scr.GetCell(matchX-1, menu.Y1+1).Attributes; got != vtui.Palette[vtui.ColMenuSelectedText] {
		t.Fatalf("non-match color = %#x, want %#x", got, vtui.Palette[vtui.ColMenuSelectedText])
	}

	search.processKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F2})
	menu.Show(scr)
	search.draw(scr)
	foundBracket := false
	foundStar := false
	for x := menu.X1 + 1; x < menu.X2; x++ {
		char := rune(scr.GetCell(x, menu.Y1).Char)
		if char == '[' {
			foundBracket = true
		}
		if char == '*' {
			foundStar = true
		}
	}
	if !foundBracket || !foundStar {
		t.Fatal("search text and F2 marker were not drawn in the dialog title")
	}
	foundHighlightedQuery := false
	for x := menu.X1 + 1; x < menu.X2; x++ {
		cell := scr.GetCell(x, menu.Y1)
		if rune(cell.Char) == 'm' && cell.Attributes == vtui.Palette[vtui.ColMenuHighlight] {
			foundHighlightedQuery = true
			break
		}
	}
	if !foundHighlightedQuery {
		t.Fatal("search query in the title does not use the match highlight color")
	}
	foundF2Hint := false
	for x := menu.X1 + 1; x < menu.X2-1; x++ {
		if rune(scr.GetCell(x, menu.Y2).Char) == 'F' && rune(scr.GetCell(x+1, menu.Y2).Char) == '2' {
			foundF2Hint = true
			break
		}
	}
	if !foundF2Hint {
		t.Fatal("F2 search-mode hint was not drawn on the bottom border")
	}
}

func TestHistorySearchResizesWithFilteredItemCount(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	menu := vtui.NewVMenu("History")
	search := newHistorySearch(menu, []HistoryRecord{{Name: "alpha"}, {Name: "beta"}, {Name: "alphabet"}, {Name: "gamma"}}, "F2: switch search mode")
	defer search.cleanup()

	_, initialY1, _, initialY2 := menu.GetPosition()
	if got := initialY2 - initialY1 + 1; got != 6 {
		t.Fatalf("initial height = %d, want 6", got)
	}
	for _, r := range "alpha" {
		search.processKey(historyKey(r))
	}
	_, filteredY1, _, filteredY2 := menu.GetPosition()
	if got := filteredY2 - filteredY1 + 1; got != 4 {
		t.Fatalf("filtered height = %d, want 4", got)
	}

	for range "alpha" {
		search.processKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK})
	}
	_, restoredY1, _, restoredY2 := menu.GetPosition()
	if got := restoredY2 - restoredY1 + 1; got != 6 {
		t.Fatalf("restored height = %d, want 6", got)
	}
}

func TestHistorySearchShowsNewestAtBottomAndScrollsToIt(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 12)
	vtui.FrameManager.Init(scr)
	menu := vtui.NewVMenu("History")
	stored := []HistoryRecord{
		{Name: "newest"}, {Name: "item-09"}, {Name: "item-08"}, {Name: "item-07"}, {Name: "item-06"}, {Name: "item-05"},
		{Name: "item-04"}, {Name: "item-03"}, {Name: "item-02"}, {Name: "item-01"}, {Name: "oldest"},
	}
	search := newHistorySearch(menu, stored, "F2: switch search mode")
	defer search.cleanup()

	if menu.Items[0].Text != "oldest" {
		t.Fatalf("first displayed item = %q, want oldest", menu.Items[0].Text)
	}
	last := len(menu.Items) - 1
	if menu.Items[last].Text != "newest" {
		t.Fatalf("last displayed item = %q, want newest", menu.Items[last].Text)
	}
	if menu.SelectPos != last {
		t.Fatalf("selected item = %d, want last item %d", menu.SelectPos, last)
	}
	wantTop := menu.ItemCount - menu.ViewHeight
	if menu.TopPos != wantTop || menu.TopPos <= 0 {
		t.Fatalf("TopPos = %d, want bottom scroll position %d", menu.TopPos, wantTop)
	}
	if !menu.ShowScrollBar || menu.ItemCount <= menu.ViewHeight {
		t.Fatal("overflowing history is not configured to show a scrollbar")
	}

	menu.Show(scr)
	search.draw(scr)
	if got := scr.GetCell(menu.X2, menu.Y1+1).Char; got != vtui.ScrollUpArrow {
		t.Fatalf("scrollbar top cell = %#x, want up arrow %#x", got, vtui.ScrollUpArrow)
	}
}
