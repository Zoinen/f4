package main

import (
	"strings"
	"unicode"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// historySearch adds incremental filtering to a VMenu while keeping the menu
// itself as the frame. Keeping the original item index in UserData lets callers
// delete the right history entry even when only a filtered subset is visible.
type historySearch struct {
	menu       *vtui.VMenu
	title      string
	hint       string
	all        []string
	query      []rune
	prefixOnly bool
}

var (
	activeHistorySearch       *historySearch
	historySearchPreviousDraw func(*vtui.ScreenBuf)
)

type historySearchEntry struct {
	index int
}

func newHistorySearch(menu *vtui.VMenu, items []string, hint string) *historySearch {
	s := &historySearch{
		menu:  menu,
		title: menu.GetTitle(),
		hint:  hint,
		all:   append([]string(nil), items...),
	}
	s.applyFilter()
	s.installRenderer()
	return s
}

func (s *historySearch) applyFilter() {
	items := make([]vtui.MenuItem, 0, len(s.all))
	// History providers keep the newest entry first. Dialogs show chronological
	// order instead: the oldest entry at the top and the newest at the bottom.
	for i := len(s.all) - 1; i >= 0; i-- {
		text := s.all[i]
		matched, _ := historySearchMatch(text, s.query, s.prefixOnly)
		if matched {
			items = append(items, vtui.MenuItem{
				Text:     text,
				UserData: historySearchEntry{index: i},
			})
		}
	}
	s.menu.Items = items
	s.menu.ItemCount = len(items)
	s.menu.TopPos = 0
	s.resize()
	// Open at the most recent visible entry. SetSelectPos also scrolls it into
	// view, which places a long history at the bottom of the viewport.
	s.menu.SetSelectPos(len(items) - 1)
	vtui.FrameManager.Redraw()
}

func (s *historySearch) resize() {
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	if scrW <= 0 || scrH <= 0 {
		return
	}

	width := scrW - 6
	if width > 120 {
		width = 120
	}
	height := len(s.menu.Items) + 2
	maxH := scrH - 4
	if maxH < 6 {
		maxH = 6
	}
	if height > maxH {
		height = maxH
	}

	x1 := (scrW - width) / 2
	y1 := (scrH - height) / 2
	s.menu.SetPosition(x1, y1, x1+width-1, y1+height-1)
}

func (s *historySearch) selected() (int, string, bool) {
	idx := s.menu.SelectPos
	if idx < 0 || idx >= len(s.menu.Items) {
		return 0, "", false
	}
	entry, ok := s.menu.Items[idx].UserData.(historySearchEntry)
	if !ok || entry.index < 0 || entry.index >= len(s.all) {
		return 0, "", false
	}
	return entry.index, s.all[entry.index], true
}

func (s *historySearch) selectOriginalIndex(originalIndex int) bool {
	for visibleIndex, item := range s.menu.Items {
		entry, ok := item.UserData.(historySearchEntry)
		if ok && entry.index == originalIndex {
			s.menu.SetSelectPos(visibleIndex)
			return true
		}
	}
	return false
}

func (s *historySearch) deleteSelected() bool {
	idx, _, ok := s.selected()
	if !ok {
		return false
	}
	s.all = append(s.all[:idx], s.all[idx+1:]...)
	s.applyFilter()
	return true
}

// setItems replaces the full item list (used by the "clear all" and
// "remove missing paths" hotkeys) and re-applies the active filter.
func (s *historySearch) setItems(items []string) {
	s.all = append([]string(nil), items...)
	s.applyFilter()
}

func (s *historySearch) processKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0

	if e.VirtualKeyCode == vtinput.VK_F2 && !shift && !ctrl && !alt {
		s.prefixOnly = !s.prefixOnly
		s.applyFilter()
		return true
	}
	if e.VirtualKeyCode == vtinput.VK_BACK && !shift && !ctrl && !alt && len(s.query) > 0 {
		s.query = s.query[:len(s.query)-1]
		s.applyFilter()
		return true
	}
	if e.Char != 0 && !ctrl && !alt && unicode.IsPrint(e.Char) {
		s.query = append(s.query, e.Char)
		s.applyFilter()
		return true
	}
	return false
}

func (s *historySearch) displayTitle() string {
	if len(s.query) == 0 && !s.prefixOnly {
		return s.title
	}
	text := string(s.query)
	if s.prefixOnly {
		text += "*"
	}
	return s.title + " [" + text + "]"
}

func (s *historySearch) installRenderer() {
	if activeHistorySearch != nil {
		activeHistorySearch.cleanup()
	}
	// Capture the previous painter into a local so nested installs (test
	// runs, back-to-back Alt+F8 → Alt+F12) don't recurse into themselves
	// by re-reading the package-level historySearchPreviousDraw.
	prev := vtui.FrameManager.OnRender
	historySearchPreviousDraw = prev
	activeHistorySearch = s
	vtui.FrameManager.OnRender = func(scr *vtui.ScreenBuf) {
		if prev != nil {
			prev(scr)
		}
		active := activeHistorySearch
		if active == nil {
			return
		}
		// The menu is gone for good only after IsDone. When another
		// frame is pushed on top (e.g. F1 help), the menu is still
		// alive in the stack — skip the paint but keep our renderer
		// installed so the hint returns after the modal closes.
		if active.menu.IsDone() {
			active.cleanup()
			return
		}
		if vtui.FrameManager.GetTopFrame() != active.menu {
			return
		}
		active.draw(scr)
	}
}

func (s *historySearch) cleanup() {
	if activeHistorySearch == s {
		vtui.FrameManager.OnRender = historySearchPreviousDraw
		activeHistorySearch = nil
		historySearchPreviousDraw = nil
	}
}

func (s *historySearch) draw(scr *vtui.ScreenBuf) {
	p := vtui.NewPainter(scr)
	titleAttr := vtui.Palette[vtui.ColMenuTitle]
	if s.menu.IsFocused() {
		titleAttr = vtui.Palette[vtui.ColDialogHighlightBoxTitle]
	}
	s.drawSearchTitle(scr, titleAttr)
	if s.hint != "" {
		p.DrawTitle(s.menu.X1, s.menu.Y2, s.menu.X2, s.hint, titleAttr)
	}

	height := s.menu.Y2 - s.menu.Y1 - 1
	for row := 0; row < height; row++ {
		itemIdx := s.menu.TopPos + row
		if itemIdx >= len(s.menu.Items) {
			break
		}
		text := s.menu.Items[itemIdx].Text
		_, highlights := historySearchMatch(text, s.query, s.prefixOnly)

		baseAttr := vtui.Palette[vtui.ColMenuText]
		highlightAttr := vtui.Palette[vtui.ColMenuHighlight]
		if itemIdx == s.menu.SelectPos {
			baseAttr = vtui.Palette[vtui.ColMenuSelectedText]
			highlightAttr = vtui.Palette[vtui.ColMenuSelectedHighlight]
		}

		y := s.menu.Y1 + 1 + row
		p.Fill(s.menu.X1+1, y, s.menu.X2-1, y, ' ', baseAttr)
		cells := []vtui.CharInfo{{Char: uint64(' '), Attributes: baseAttr}}
		for i, r := range []rune(text) {
			attr := baseAttr
			if i < len(highlights) && highlights[i] {
				attr = highlightAttr
			}
			sanitized, width := vtui.SanitizeRune(r)
			if width == 0 {
				continue
			}
			cells = append(cells, vtui.CharInfo{Char: uint64(sanitized), Attributes: attr})
			for j := 1; j < width; j++ {
				cells = append(cells, vtui.CharInfo{Char: vtui.WideCharFiller, Attributes: attr})
			}
		}
		maxCells := s.menu.X2 - s.menu.X1 - 1
		if len(cells) > maxCells {
			cells = cells[:maxCells]
		}
		scr.Write(s.menu.X1+1, y, cells)
	}
	// Redrawing the rows above may cover the menu's scrollbar cell.
	s.menu.DrawScrollBar(scr)
}

func (s *historySearch) drawSearchTitle(scr *vtui.ScreenBuf, titleAttr uint64) {
	highlightAttr := vtui.Palette[vtui.ColMenuHighlight]
	query := string(s.query)
	if s.prefixOnly {
		query += "*"
	}

	cells := make([]vtui.CharInfo, 0, len(s.title)+len(query)+6)
	appendText := func(text string, attr uint64) {
		cells = append(cells, vtui.StringToCharInfo(text, attr)...)
	}
	appendText(" "+s.title, titleAttr)
	if query != "" {
		appendText(" [", titleAttr)
		appendText(query, highlightAttr)
		appendText("]", titleAttr)
	}
	appendText(" ", titleAttr)

	width := s.menu.X2 - s.menu.X1 + 1
	maxCells := width - 2
	if maxCells <= 0 {
		return
	}
	if len(cells) > maxCells {
		cells = cells[:maxCells]
	}
	x := s.menu.X1 + (width-len(cells))/2
	scr.Write(x, s.menu.Y1, cells)
}

// historySearchMatch returns whether text passes the filter and a rune mask for
// every occurrence that should be highlighted. Comparison is case-insensitive.
func historySearchMatch(text string, query []rune, prefixOnly bool) (bool, []bool) {
	textRunes := []rune(text)
	highlights := make([]bool, len(textRunes))
	if len(query) == 0 {
		return true, highlights
	}
	if len(query) > len(textRunes) {
		return false, highlights
	}

	equalAt := func(start int) bool {
		return strings.EqualFold(string(textRunes[start:start+len(query)]), string(query))
	}
	if prefixOnly {
		if !equalAt(0) {
			return false, highlights
		}
		for i := range query {
			highlights[i] = true
		}
		return true, highlights
	}

	found := false
	for start := 0; start+len(query) <= len(textRunes); start++ {
		if !equalAt(start) {
			continue
		}
		found = true
		for i := range query {
			highlights[start+i] = true
		}
	}
	return found, highlights
}
