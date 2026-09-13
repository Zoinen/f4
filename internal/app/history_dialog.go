package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/history"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// historySearch adds incremental filtering to a VMenu while keeping the menu
// itself as the frame. Keeping the original item index in UserData lets callers
// delete the right history entry even when only a filtered subset is visible.

type historySearch struct {
	menu          *vtui.VMenu
	title         string
	hint          string
	all           []history.HistoryRecord
	secondary     []string
	query         []rune
	prefixOnly    bool
	showSecond    bool
	secondWidth   int
	supportsLocks bool
	showDetails   bool
	showTimes     bool
	dateColumn    bool
	timeMode      int
	showDirPrefix bool
	dirPrefixLen  int
	// pinSlotOf turns on the pinned area (issue #407): marked entries are
	// listed above the chronological ones and carry the digit of the far2l
	// bookmark slot they own instead of the plain lock star. It reports -1
	// for a record with no slot. Folder history sets it; the command and
	// view/edit histories leave it nil and keep their flat list.
	pinSlotOf       func(history.HistoryRecord) int
	onLockToggled   func()
	onTimesChanged  func(int)
	onPrefixChanged func(int)
	onDetails       func(history.HistoryRecord)
	onCtrlF10       func(history.HistoryRecord)
}

var (
	activeHistorySearch       *historySearch
	historySearchPreviousDraw func(*vtui.ScreenBuf)
)

type historySearchEntry struct {
	index int
}

func newHistorySearch(menu *vtui.VMenu, items []history.HistoryRecord, hint string) *historySearch {
	s := prepareHistorySearch(menu, items, hint)
	s.applyFilter()
	return s
}

// The three dialogs configure columns before doing their first list pass.
func prepareHistorySearch(menu *vtui.VMenu, items []history.HistoryRecord, hint string) *historySearch {
	menu.SemanticBottomHint = hint
	menu.SemanticPresentation = "window"
	menu.ColorTextIdx = vtui.ColDialogText
	menu.ColorSelectedTextIdx = vtui.ColDialogSelectedButton
	menu.ColorHighlightIdx = vtui.ColDialogHighlightText
	menu.ColorSelectedHighlightIdx = vtui.ColDialogHighlightSelectedButton
	menu.ColorBoxIdx = vtui.ColDialogBox
	menu.ColorTitleIdx = vtui.ColDialogBoxTitle
	if menu.ScrollBar != nil {
		menu.ScrollBar.ColorIdx = vtui.ColDialogBox
	}
	s := &historySearch{
		menu:  menu,
		title: menu.GetTitle(),
		hint:  hint,
		all:   append([]history.HistoryRecord(nil), items...),
	}
	s.installRenderer()
	return s
}

func (s *historySearch) applyFilter() {
	// The console's custom painter and the native menu share this title.
	// Keep the query visible even when filtering produces no rows.
	s.menu.SetTitle(s.displayTitle())
	items := make([]vtui.MenuItem, 0, len(s.all))
	var pinned []vtui.MenuItem
	// History providers keep the newest entry first. Dialogs show chronological
	// order instead: the oldest entry at the top and the newest at the bottom.
	for i := len(s.all) - 1; i >= 0; i-- {
		text := s.displayText(s.all[i])
		if len(s.query) != 0 {
			matched, _ := historySearchMatch(text, s.query, s.prefixOnly)
			if !matched {
				continue
			}
		}
		item := vtui.MenuItem{Text: text, UserData: historySearchEntry{index: i}}
		item.Details = s.semanticDetails(s.all[i])
		// Pinned folders get their own area above the chronological list, so
		// they stay one glance away however long the history grows (#407).
		if s.pinSlotOf != nil && s.isPinned(i) {
			pinned = append(pinned, item)
			continue
		}
		items = append(items, item)
	}
	if len(pinned) > 0 {
		// Slot order first — that is the order of the digit hotkeys — then
		// the folders pinned past the tenth, which have no digit to sort by.
		sort.SliceStable(pinned, func(a, b int) bool {
			return s.pinRank(pinned[a]) < s.pinRank(pinned[b])
		})
		if len(items) > 0 {
			pinned = append(pinned, vtui.MenuItem{Separator: true})
		}
		items = append(pinned, items...)
	}
	s.menu.Items = items
	s.menu.ItemCount = len(items)
	s.menu.TopPos = 0
	s.resize()
	// VMenu's default renderer draws MenuItem.Text after the leading margin,
	// without knowing about the frame's right border. Keep its copy bounded so
	// a long command cannot paint into the terminal outside the dialog. The
	// custom renderer below still obtains the complete text from s.all, so
	// filtering and command insertion retain the original value.
	for i := range s.menu.Items {
		entry, ok := s.menu.Items[i].UserData.(historySearchEntry)
		if !ok || entry.index < 0 || entry.index >= len(s.all) {
			continue
		}
		s.menu.Items[i].Text = s.defaultMenuText(s.menu.Items[i].Text)
	}
	// Open at the most recent visible entry. SetSelectPos also scrolls it into
	// view, which places a long history at the bottom of the viewport.
	s.menu.SetSelectPos(len(items) - 1)
	s.menu.SemanticItemsRevision++
	vtui.FrameManager.Redraw()
}

// Keep native columns and Unicode search masks separate from console labels.
func (s *historySearch) semanticDetails(record history.HistoryRecord) map[string]string {
	details := map[string]string{"kind": "history"}
	if !s.dateColumn && !s.showDirPrefix {
		details["primary"] = s.displayText(record)
	}
	if s.dateColumn {
		details["primary"] = record.Name
		details["date"] = strings.TrimSpace(historyTimeColumn(record.Timestamp, s.timeMode))
		details["columns"] = "dated"
	}
	if s.showDirPrefix {
		details["primary"] = record.Name
		details["path"] = record.Directory()
		details["date"] = strings.TrimSpace(historyTimeColumn(record.Timestamp, s.timeMode))
		details["columns"] = "command"
	}
	if len(s.query) == 0 {
		return details
	}
	for _, key := range []string{"primary", "path", "date"} {
		_, mask := historySearchMatch(details[key], s.query, s.prefixOnly)
		var encoded strings.Builder
		for _, matched := range mask {
			if matched {
				encoded.WriteByte('1')
			} else {
				encoded.WriteByte('0')
			}
		}
		details[key+"Matches"] = encoded.String()
	}
	return details
}

// isPinned reports whether the record at index belongs in the pinned area:
// the user marked it, or it owns a bookmark slot set elsewhere (from the
// panel with Ctrl+Shift+N, or in a bookmarks.ini shared with far2l).
func (s *historySearch) isPinned(index int) bool {
	if index < 0 || index >= len(s.all) {
		return false
	}
	if s.all[index].Lock {
		return true
	}
	return s.pinSlotOf != nil && s.pinSlotOf(s.all[index]) >= 0
}

// pinRank orders the pinned area: slot digit first, everything past the tenth
// pin last, in the chronological order applyFilter produced it in.
func (s *historySearch) pinRank(item vtui.MenuItem) int {
	unslotted := history.PinSlots
	entry, ok := item.UserData.(historySearchEntry)
	if !ok || s.pinSlotOf == nil || entry.index < 0 || entry.index >= len(s.all) {
		return unslotted
	}
	if slot := s.pinSlotOf(s.all[entry.index]); slot >= 0 {
		return slot
	}
	return unslotted
}

// marker is the character drawn in the leading column: the bookmark digit for
// a pinned folder that owns one of the ten slots, a star for one that is
// marked but ran out of digits, a blank for an ordinary entry.
func (s *historySearch) marker(index int) rune {
	if index < 0 || index >= len(s.all) {
		return ' '
	}
	if s.pinSlotOf != nil {
		if slot := s.pinSlotOf(s.all[index]); slot >= 0 && slot <= 9 {
			return rune('0' + slot)
		}
	}
	if s.all[index].Lock {
		return '*'
	}
	return ' '
}

// refreshKeepingSelection re-groups the list after a pin changed and leaves
// the cursor on the same record — applyFilter on its own drops it on the
// newest entry, which would throw the user out of the pinned area.
func (s *historySearch) refreshKeepingSelection() {
	index, _, ok := s.selected()
	s.applyFilter()
	if ok {
		s.selectOriginalIndex(index)
	}
}

func (s *historySearch) defaultMenuText(text string) string {
	// VMenu reserves one cell for its leading space and one for the right
	// border. There are no shortcuts in history menus, so this is the maximum
	// number of cells its default renderer may draw for the item text.
	maxWidth := s.menu.X2 - s.menu.X1 - 2
	if maxWidth <= 0 {
		// Unit tests and callers that have not allocated a screen yet leave the
		// menu at its zero geometry. In that state there is no drawable border
		// to clip against; preserve the logical item text until layout occurs.
		return text
	}
	// Printable ASCII needs no grapheme segmentation. Histories contain
	// thousands of mostly-ASCII paths; keep Unicode on the exact old path.
	if historyPrintableASCII(text) {
		if len(text) <= maxWidth {
			return text
		}
		return text[:maxWidth-1] + "…"
	}
	return runewidth.Truncate(text, maxWidth, "…")
}

func historyPrintableASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] < 32 || text[i] >= 127 {
			return false
		}
	}
	return true
}

func (s *historySearch) displayText(record history.HistoryRecord) string {
	if !s.showTimes && !s.showDirPrefix {
		return record.DisplayText()
	}

	var result strings.Builder
	if s.showTimes {
		result.WriteString(historyTimeColumn(record.Timestamp, s.timeMode))
	}
	if s.showDirPrefix {
		result.WriteString(historyDirectoryPrefix(record.Directory(), s.dirPrefixLen))
	}
	result.WriteString(record.Name)
	return result.String()
}

// historyTimeColumn renders the leading timestamp of one history row. A record
// stored before its history learned about timestamps has none; it gets a blank
// column of the same width so the entries below and above it stay aligned,
// the way far2l pads the command-history directory prefix.
func historyTimeColumn(stamp time.Time, mode int) string {
	layout := "2006-01-02 15:04:05 "
	switch mode {
	case config.HistoryShowDate:
		layout = "2006-01-02 "
	case config.HistoryShowNone:
		return ""
	}
	if stamp.IsZero() {
		return strings.Repeat(" ", len(layout))
	}
	return stamp.Format(layout)
}

func historyDirectoryPrefix(dir string, width int) string {
	if width <= 0 {
		return ""
	}
	if dir == "" {
		return strings.Repeat(" ", width) + " "
	}
	if historyPrintableASCII(dir) {
		if len(dir) > width {
			budget := max(1, width-3)
			dir = "..." + dir[len(dir)-budget:]
		}
		return strings.Repeat(" ", max(0, width-len(dir))) + dir + "/ "
	}
	if runewidth.StringWidth(dir) > width {
		budget := width - 3
		if budget < 1 {
			budget = 1
		}
		runes := []rune(dir)
		start := len(runes)
		used := 0
		for start > 0 {
			w := runewidth.RuneWidth(runes[start-1])
			if used+w > budget {
				break
			}
			start--
			used += w
		}
		dir = "..." + string(runes[start:])
	}
	if missing := width - runewidth.StringWidth(dir); missing > 0 {
		dir = strings.Repeat(" ", missing) + dir
	}
	return dir + "/ "
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

func (s *historySearch) selected() (int, history.HistoryRecord, bool) {
	idx := s.menu.SelectPos
	if idx < 0 || idx >= len(s.menu.Items) {
		return 0, history.HistoryRecord{}, false
	}
	entry, ok := s.menu.Items[idx].UserData.(historySearchEntry)
	if !ok || entry.index < 0 || entry.index >= len(s.all) {
		return 0, history.HistoryRecord{}, false
	}
	return entry.index, s.all[entry.index], true
}

func (s *historySearch) selectedSecondary() string {
	idx, rec, ok := s.selected()
	if !ok {
		return ""
	}
	if idx >= 0 && idx < len(s.secondary) && s.secondary[idx] != "" {
		return s.secondary[idx]
	}
	return rec.Directory()
}

func (s *historySearch) setSecondaryWidth(items []string, visible bool, width int) {
	s.secondary = make([]string, len(s.all))
	copy(s.secondary, items)
	s.showSecond = visible && s.hasSecondary()
	s.secondWidth = width
	s.applyFilter()
}

func (s *historySearch) hasSecondary() bool {
	for _, text := range s.secondary {
		if text != "" {
			return true
		}
	}
	for _, rec := range s.all {
		if rec.Directory() != "" {
			return true
		}
	}
	return false
}

func (s *historySearch) secondaryAt(index int) string {
	if index < 0 || index >= len(s.all) {
		return ""
	}
	if index < len(s.secondary) && s.secondary[index] != "" {
		return s.secondary[index]
	}
	return s.all[index].Directory()
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
	idx, rec, ok := s.selected()
	if !ok || rec.Lock {
		return false
	}
	s.all = append(s.all[:idx], s.all[idx+1:]...)
	if idx < len(s.secondary) {
		s.secondary = append(s.secondary[:idx], s.secondary[idx+1:]...)
	}
	s.applyFilter()
	return true
}

// setItems replaces the full item list (used by the "clear all" and
// "remove missing paths" hotkeys) and re-applies the active filter.
func (s *historySearch) setItems(items []history.HistoryRecord) {
	s.all = append([]history.HistoryRecord(nil), items...)
	s.applyFilter()
}

func (s *historySearch) processKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0

	if s.showDetails && e.VirtualKeyCode == vtinput.VK_F3 && !shift && !ctrl && !alt {
		_, rec, ok := s.selected()
		if ok {
			if s.onDetails != nil {
				s.onDetails(rec)
			} else {
				tStr := "None"
				if !rec.Timestamp.IsZero() {
					tStr = rec.Timestamp.Format(time.RFC1123)
				}
				msg := fmt.Sprintf("Command: %s\nDirectory: %s\nTime: %s", rec.Name, rec.Directory(), tStr)
				vtui.ShowMessage(" Details ", msg, []string{"&Ok"})
			}
		}
		return true
	}
	if s.showTimes && e.VirtualKeyCode == vtinput.VK_T && ctrl && !shift && !alt {
		s.timeMode = (s.timeMode + 1) % 3
		if s.onTimesChanged != nil {
			s.onTimesChanged(s.timeMode)
		}
		s.applyFilter()
		return true
	}
	if s.showDirPrefix && e.VirtualKeyCode == vtinput.VK_LEFT && ctrl && !shift && !alt && s.timeMode == config.HistoryShowDateTime {
		if s.dirPrefixLen > 4 {
			s.dirPrefixLen--
			if s.onPrefixChanged != nil {
				s.onPrefixChanged(s.dirPrefixLen)
			}
			s.applyFilter()
		}
		return true
	}
	if s.showDirPrefix && e.VirtualKeyCode == vtinput.VK_RIGHT && ctrl && !shift && !alt && s.timeMode == config.HistoryShowDateTime {
		s.dirPrefixLen++
		if s.onPrefixChanged != nil {
			s.onPrefixChanged(s.dirPrefixLen)
		}
		s.applyFilter()
		return true
	}
	if s.supportsLocks && e.VirtualKeyCode == vtinput.VK_INSERT && !shift && !ctrl && !alt {
		idx, _, ok := s.selected()
		if ok {
			s.all[idx].Lock = !s.all[idx].Lock
			vtui.FrameManager.Redraw()
			if s.onLockToggled != nil {
				s.onLockToggled()
			}
		}
		return true
	}
	if e.VirtualKeyCode == vtinput.VK_F10 && ctrl && !shift && !alt {
		_, rec, ok := s.selected()
		if ok && s.onCtrlF10 != nil {
			s.onCtrlF10(rec)
		}
		return true
	}
	if e.VirtualKeyCode == vtinput.VK_F2 && !shift && !ctrl && !alt {
		s.prefixOnly = !s.prefixOnly
		s.applyFilter()
		return true
	}
	if e.VirtualKeyCode == vtinput.VK_F2 && ctrl && !shift && !alt && s.hasSecondary() {
		s.showSecond = !s.showSecond
		vtui.FrameManager.Redraw()
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
	titleAttr := vtui.Palette[s.menu.ColorTitleIdx]
	if s.menu.IsFocused() {
		titleAttr = vtui.Palette[vtui.ColDialogHighlightBoxTitle]
	}
	s.drawSearchTitle(scr, titleAttr)
	if s.hint != "" {
		p.DrawTitle(s.menu.X1, s.menu.Y2, s.menu.X2, s.hint, titleAttr)
	}

	height := s.menu.Y2 - s.menu.Y1 - 1
	var itemIdx int
	for row := 0; row < height; row++ {
		itemIdx = s.menu.TopPos + row
		if itemIdx >= len(s.menu.Items) {
			break
		}
		item := s.menu.Items[itemIdx]
		if item.Separator {
			// VMenu already drew the divider between the pinned area and the
			// chronological list; painting a blank item row over it would
			// rub it out again.
			continue
		}
		text := item.Text
		commandStart, commandEnd := 0, 0
		if entry, ok := item.UserData.(historySearchEntry); ok && entry.index >= 0 && entry.index < len(s.all) {
			record := s.all[entry.index]
			text = s.displayText(record)
			if s.showDirPrefix {
				commandStart = len([]rune(text)) - len([]rune(record.Name))
				prefix, _, _ := strings.Cut(record.Name, " ")
				commandEnd = commandStart + len([]rune(prefix))
			}
		}
		_, highlights := historySearchMatch(text, s.query, s.prefixOnly)

		baseAttr := vtui.Palette[s.menu.ColorTextIdx]
		highlightAttr := vtui.Palette[s.menu.ColorHighlightIdx]
		if itemIdx == s.menu.SelectPos {
			baseAttr = vtui.Palette[s.menu.ColorSelectedTextIdx]
			highlightAttr = vtui.Palette[s.menu.ColorSelectedHighlightIdx]
		}

		y := s.menu.Y1 + 1 + row
		p.Fill(s.menu.X1+1, y, s.menu.X2-1, y, ' ', baseAttr)
		innerWidth := s.menu.X2 - s.menu.X1 - 1
		secondaryWidth := s.secondaryColumnWidth(innerWidth)
		commandWidth := innerWidth
		if secondaryWidth > 0 {
			commandWidth -= secondaryWidth + 2
		}

		markChar := uint64(' ')
		if entry, ok := item.UserData.(historySearchEntry); ok {
			markChar = uint64(s.marker(entry.index))
		}

		cells := make([]vtui.CharInfo, 0, len([]rune(text))+1)
		if s.supportsLocks {
			cells = append(cells, vtui.CharInfo{Char: markChar, Attributes: baseAttr})
			// Keep the bookmark digit/star separate from the first character of
			// the row. Folder history may start with a timestamp or a directory
			// prefix, so placing the marker directly against it makes the hotkey
			// look like part of the value (issue #407).
			cells = append(cells, vtui.CharInfo{Char: ' ', Attributes: baseAttr})
		}
		for i, r := range []rune(text) {
			attr := baseAttr
			if i >= commandStart && i < commandEnd {
				attr = vtui.SetRGBFore(attr, 0x75d977)
			}
			if i < len(highlights) && highlights[i] {
				attr = highlightAttr
			}
			sanitized, width := vtui.SanitizeRune(r)
			if width == 0 {
				continue
			}
			cells = append(cells, vtui.CharInfo{Char: vtui.RegisterCluster(string(sanitized)), Attributes: attr})
			for j := 1; j < width; j++ {
				cells = append(cells, vtui.CharInfo{Char: vtui.WideCharFiller, Attributes: attr})
			}
		}
		maxCells := commandWidth
		if len(cells) > maxCells {
			cells = cells[:maxCells]
		}
		scr.Write(s.menu.X1+1, y, cells)

		entry, ok := item.UserData.(historySearchEntry)
		if secondaryWidth > 0 && ok {
			path := truncateHistoryPath(s.secondaryAt(entry.index), secondaryWidth)
			if path != "" {
				x := s.menu.X2 - runewidth.StringWidth(path)
				p.DrawString(x, y, path, baseAttr)
			}
		}
	}
	// Redrawing the rows above may cover the menu's scrollbar cell.
	s.menu.DrawScrollBar(scr)
	s.menu.DrawWindowControls(scr)
}

func (s *historySearch) secondaryColumnWidth(innerWidth int) int {
	if !s.showSecond || !s.hasSecondary() || innerWidth < 36 {
		return 0
	}
	width := s.secondWidth
	if width <= 0 {
		width = 24
	}
	if maxWidth := innerWidth / 3; width > maxWidth {
		width = maxWidth
	}
	return width
}

func truncateHistoryPath(path string, maxWidth int) string {
	if path == "" || maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(path) <= maxWidth {
		return path
	}
	if maxWidth < 5 {
		return runewidth.Truncate(path, maxWidth, "")
	}

	leftBudget := (maxWidth - 3 + 1) / 2
	rightBudget := maxWidth - 3 - leftBudget
	left := runewidth.Truncate(path, leftBudget, "")
	runes := []rune(path)
	rightStart := len(runes)
	rightWidth := 0
	for rightStart > 0 {
		width := runewidth.RuneWidth(runes[rightStart-1])
		if rightWidth+width > rightBudget {
			break
		}
		rightStart--
		rightWidth += width
	}
	return left + "..." + string(runes[rightStart:])
}

func (s *historySearch) drawSearchTitle(scr *vtui.ScreenBuf, titleAttr uint64) {
	highlightAttr := vtui.Palette[s.menu.ColorHighlightIdx]
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
