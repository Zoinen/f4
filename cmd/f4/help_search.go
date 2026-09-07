package main

import (
	"reflect"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type helpSearchMatch struct {
	line       int
	start, end int // rune offsets in the visible (markup-free) line
}

type helpSearchState struct {
	frame     vtui.Frame
	topicName string
	query     []rune
	matches   []helpSearchMatch
	selected  int
	scrollTop int
}

var currentHelpSearch *helpSearchState

type helpWindowBounds struct {
	x1, y1, x2, y2 int
}

type helpZoomState struct {
	frame vtui.Frame
	saved helpWindowBounds
}

var currentHelpZoom *helpZoomState

func helpTopicForFrame(frame vtui.Frame) (string, *vtui.HelpTopic, bool) {
	if frame == nil || vtui.GlobalHelpEngine == nil {
		return "", nil, false
	}
	title := strings.TrimSpace(frame.GetTitle())
	const prefix = "Help:"
	if !strings.HasPrefix(title, prefix) {
		return "", nil, false
	}
	name := strings.TrimSpace(strings.TrimPrefix(title, prefix))
	topic := vtui.GlobalHelpEngine.GetTopic(name)
	return name, topic, topic != nil
}

func handleHelpSearchHotkey(e *vtinput.InputEvent) bool {
	if e == nil {
		return false
	}
	frame := vtui.FrameManager.GetTopFrame()
	_, _, isHelp := helpTopicForFrame(frame)
	if !isHelp {
		return false
	}

	if e.Type == vtinput.MouseEventType {
		if helpZoomButtonHit(frame, e) {
			toggleHelpZoom(frame)
			return true
		}
		return false
	}
	if e.Type != vtinput.KeyEventType || !e.KeyDown {
		return false
	}
	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
	if e.VirtualKeyCode == vtinput.VK_F5 && !shift && !ctrl && !alt {
		toggleHelpZoom(frame)
		return true
	}

	if (e.VirtualKeyCode == vtinput.VK_F3 && !ctrl && !alt) ||
		(e.VirtualKeyCode == vtinput.VK_RETURN && ctrl && !alt) {
		moveHelpSearch(frame, shift)
		return true
	}
	if e.VirtualKeyCode == vtinput.VK_ESCAPE && !shift && !ctrl && !alt &&
		currentHelpSearch != nil && currentHelpSearch.frame == frame && len(currentHelpSearch.query) > 0 {
		// Let HelpView receive Escape so it closes immediately. Clear only the
		// search overlay state here; Backspace remains the search-editing key.
		currentHelpSearch = nil
		return false
	}

	if e.VirtualKeyCode == vtinput.VK_BACK && !shift && !ctrl && !alt {
		if currentHelpSearch != nil && currentHelpSearch.frame == frame && len(currentHelpSearch.query) > 0 {
			currentHelpSearch.query = currentHelpSearch.query[:len(currentHelpSearch.query)-1]
			updateHelpSearch(frame)
			return true
		}
		// HelpView closes itself when PopTopic is called with empty history.
		// Preserve Backspace for topic navigation, but never let it close the
		// root Help window.
		if historyLen, ok := nestedHelpLen(reflect.ValueOf(frame), "history"); !ok || historyLen == 0 {
			return true
		}
	}
	if e.Char != 0 && !ctrl && !alt && unicode.IsPrint(e.Char) {
		ensureHelpSearch(frame)
		currentHelpSearch.query = append(currentHelpSearch.query, e.Char)
		updateHelpSearch(frame)
		return true
	}
	return false
}

func ensureHelpSearch(frame vtui.Frame) {
	topicName, _, ok := helpTopicForFrame(frame)
	if !ok {
		return
	}
	if currentHelpSearch == nil || currentHelpSearch.frame != frame || currentHelpSearch.topicName != topicName {
		currentHelpSearch = &helpSearchState{frame: frame, topicName: topicName, selected: -1}
	}
}

func updateHelpSearch(frame vtui.Frame) {
	ensureHelpSearch(frame)
	if currentHelpSearch == nil {
		return
	}
	_, topic, ok := helpTopicForFrame(frame)
	if !ok {
		currentHelpSearch = nil
		return
	}
	if len(currentHelpSearch.query) == 0 {
		currentHelpSearch = nil
		vtui.FrameManager.Redraw()
		return
	}
	currentHelpSearch.matches = collectHelpMatches(topic, string(currentHelpSearch.query))
	currentHelpSearch.selected = -1
	if len(currentHelpSearch.matches) > 0 {
		currentHelpSearch.selected = 0
		currentHelpSearch.scrollTop = scrollHelpToLine(frame, topic, currentHelpSearch.matches[0].line)
	}
	vtui.FrameManager.Redraw()
}

func moveHelpSearch(frame vtui.Frame, reverse bool) bool {
	topicName, topic, ok := helpTopicForFrame(frame)
	if !ok || currentHelpSearch == nil || currentHelpSearch.frame != frame ||
		currentHelpSearch.topicName != topicName || len(currentHelpSearch.matches) == 0 {
		return false
	}
	if reverse {
		currentHelpSearch.selected--
		if currentHelpSearch.selected < 0 {
			currentHelpSearch.selected = len(currentHelpSearch.matches) - 1
		}
	} else {
		currentHelpSearch.selected++
		if currentHelpSearch.selected >= len(currentHelpSearch.matches) {
			currentHelpSearch.selected = 0
		}
	}
	currentHelpSearch.scrollTop = scrollHelpToLine(frame, topic, currentHelpSearch.matches[currentHelpSearch.selected].line)
	vtui.FrameManager.Redraw()
	return true
}

func collectHelpMatches(topic *vtui.HelpTopic, query string) []helpSearchMatch {
	queryRunes := []rune(query)
	if topic == nil || len(queryRunes) == 0 {
		return nil
	}
	var matches []helpSearchMatch
	for lineIdx, rawLine := range topic.Lines {
		line, _ := visibleHelpLine(rawLine)
		runes := []rune(line)
		for start := 0; start+len(queryRunes) <= len(runes); start++ {
			if strings.EqualFold(string(runes[start:start+len(queryRunes)]), query) {
				matches = append(matches, helpSearchMatch{line: lineIdx, start: start, end: start + len(queryRunes)})
			}
		}
	}
	return matches
}

func visibleHelpLine(line string) (string, bool) {
	centered := strings.HasPrefix(line, "^")
	if centered {
		line = strings.TrimPrefix(line, "^")
	}
	runes := []rune(line)
	var out strings.Builder
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '#':
			continue
		case '~':
			// Opening '~' starts visible link text. Closing '~' is followed by
			// an invisible target ending at '@'.
			for i++; i < len(runes) && runes[i] != '~'; i++ {
				out.WriteRune(runes[i])
			}
			for i++; i < len(runes) && runes[i] != '@'; i++ {
			}
			continue
		default:
			out.WriteRune(runes[i])
		}
	}
	return out.String(), centered
}

func scrollHelpToLine(frame vtui.Frame, topic *vtui.HelpTopic, line int) int {
	_, y1, _, y2 := frame.GetPosition()
	contentHeight := (y2 - y1 + 1) - 2 - topic.StickyRows
	if contentHeight < 1 {
		contentHeight = 1
	}
	maxScroll := len(topic.Lines) - topic.StickyRows - contentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	desired := line - topic.StickyRows - contentHeight/2
	if desired < 0 {
		desired = 0
	}
	if desired > maxScroll {
		desired = maxScroll
	}

	ctrlUp := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP, ControlKeyState: vtinput.LeftCtrlPressed}
	ctrlDown := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN, ControlKeyState: vtinput.LeftCtrlPressed}
	for i := 0; i < len(topic.Lines); i++ {
		frame.ProcessKey(ctrlUp)
	}
	for i := 0; i < desired; i++ {
		frame.ProcessKey(ctrlDown)
	}
	if actual, ok := helpViewScrollTop(frame); ok {
		return actual
	}
	return desired
}

// vtui.HelpView does not currently expose its scroll position. Read it here so
// search overlays follow scrolling performed by keys, the wheel, or scrollbar.
func helpViewScrollTop(frame vtui.Frame) (int, bool) {
	return nestedHelpInt(reflect.ValueOf(frame), "scrollTop")
}

func nestedHelpInt(value reflect.Value, name string) (int, bool) {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return 0, false
	}
	if field := value.FieldByName(name); field.IsValid() && field.Kind() == reflect.Int {
		return int(field.Int()), true
	}
	// Some consumers embed HelpView, which itself embeds BaseWindow.
	for _, embeddedName := range []string{"HelpView", "BaseWindow"} {
		if embedded := value.FieldByName(embeddedName); embedded.IsValid() {
			if result, ok := nestedHelpInt(embedded, name); ok {
				return result, true
			}
		}
	}
	return 0, false
}

func nestedHelpLen(value reflect.Value, name string) (int, bool) {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return 0, false
	}
	if field := value.FieldByName(name); field.IsValid() && (field.Kind() == reflect.Slice || field.Kind() == reflect.Array) {
		return field.Len(), true
	}
	for _, embeddedName := range []string{"HelpView", "BaseWindow"} {
		if embedded := value.FieldByName(embeddedName); embedded.IsValid() {
			if result, ok := nestedHelpLen(embedded, name); ok {
				return result, true
			}
		}
	}
	return 0, false
}

func helpControlOffset(frame vtui.Frame) int {
	_, _, x2, _ := frame.GetPosition()
	offset := 4
	if len(vtui.FrameManager.Screens) > 1 && x2 >= vtui.FrameManager.GetScreenSize()-1 {
		offset = 6
	}
	return offset
}

func helpZoomButtonHit(frame vtui.Frame, e *vtinput.InputEvent) bool {
	if e == nil || !e.KeyDown || e.ButtonState != vtinput.FromLeft1stButtonPressed {
		return false
	}
	_, y1, x2, _ := frame.GetPosition()
	offset := helpControlOffset(frame) + 3
	mx, my := int(e.MouseX), int(e.MouseY)
	return my == y1 && mx >= x2-offset && mx <= x2-offset+2
}

func toggleHelpZoom(frame vtui.Frame) bool {
	resizable, ok := frame.(interface {
		ChangeSize(int, int)
		MoveRelative(int, int)
	})
	if !ok {
		return false
	}
	x1, y1, x2, y2 := frame.GetPosition()
	target := helpWindowBounds{}
	if currentHelpZoom != nil && currentHelpZoom.frame == frame {
		target = fitHelpBounds(currentHelpZoom.saved)
		currentHelpZoom = nil
	} else {
		currentHelpZoom = &helpZoomState{frame: frame, saved: helpWindowBounds{x1, y1, x2, y2}}
		width, height := vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight()
		target = helpWindowBounds{0, 0, width - 1, height - 2}
	}
	lastW, okW := nestedHelpInt(reflect.ValueOf(frame), "lastW")
	lastH, okH := nestedHelpInt(reflect.ValueOf(frame), "lastH")
	if !okW || !okH {
		return false
	}
	currentW, currentH := x2-x1+1, y2-y1+1
	targetW, targetH := target.x2-target.x1+1, target.y2-target.y1+1
	resizable.ChangeSize(lastW+targetW-currentW, lastH+targetH-currentH)
	nowX1, nowY1, _, _ := frame.GetPosition()
	resizable.MoveRelative(target.x1-nowX1, target.y1-nowY1)
	vtui.FrameManager.Redraw()
	return true
}

func fitHelpBounds(bounds helpWindowBounds) helpWindowBounds {
	maxX := vtui.FrameManager.GetScreenSize() - 1
	maxY := vtui.FrameManager.GetScreenHeight() - 2
	width, height := bounds.x2-bounds.x1+1, bounds.y2-bounds.y1+1
	if width > maxX+1 {
		bounds.x1, bounds.x2 = 0, maxX
	} else {
		if bounds.x1 < 0 {
			bounds.x1, bounds.x2 = 0, width-1
		}
		if bounds.x2 > maxX {
			bounds.x2, bounds.x1 = maxX, maxX-width+1
		}
	}
	if height > maxY+1 {
		bounds.y1, bounds.y2 = 0, maxY
	} else {
		if bounds.y1 < 0 {
			bounds.y1, bounds.y2 = 0, height-1
		}
		if bounds.y2 > maxY {
			bounds.y2, bounds.y1 = maxY, maxY-height+1
		}
	}
	return bounds
}

func enableHelpZoom(frame vtui.Frame) bool {
	value := reflect.ValueOf(frame)
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return false
	}
	field := value.FieldByName("ShowZoom")
	if !field.IsValid() || field.Kind() != reflect.Bool || !field.CanSet() {
		return false
	}
	field.SetBool(true)
	return true
}

func drawHelpWindowControls(scr *vtui.ScreenBuf, frame vtui.Frame) {
	x1, y1, x2, _ := frame.GetPosition()
	if x2-x1 < 10 {
		return
	}
	attr := scr.GetCell(x2, y1).Attributes
	zoom := string(vtui.UIStrings.CloseBrackets[0]) + string(vtui.UIStrings.ZoomSymbol) + string(vtui.UIStrings.CloseBrackets[1])
	closeButton := string(vtui.UIStrings.CloseBrackets[0]) + string(vtui.UIStrings.CloseSymbol) + string(vtui.UIStrings.CloseBrackets[1])
	offset := helpControlOffset(frame)
	// These are the standard BaseWindow control offsets when both buttons are
	// visible. Drawing them here makes the zoom button visible on the first
	// Help frame, before the next redraw observes ShowZoom.
	scr.Write(x2-offset-3, y1, vtui.StringToCharInfo(zoom, attr))
	scr.Write(x2-offset, y1, vtui.StringToCharInfo(closeButton, attr))
}

func renderHelpSearch(scr *vtui.ScreenBuf) {
	frame := vtui.FrameManager.GetTopFrame()
	topicName, topic, isHelp := helpTopicForFrame(frame)
	if !isHelp {
		currentHelpSearch = nil
		currentHelpZoom = nil
		return
	}
	if enableHelpZoom(frame) {
		defer drawHelpWindowControls(scr, frame)
	}

	x1, y1, x2, y2 := frame.GetPosition()
	titleAttr := scr.GetCell((x1+x2)/2, y1).Attributes
	vtui.NewPainter(scr).DrawTitle(x1, y2, x2, Msg("Help.SearchHint"), titleAttr)
	if currentHelpSearch == nil || currentHelpSearch.frame != frame || currentHelpSearch.topicName != topicName {
		currentHelpSearch = nil
		return
	}
	drawHelpSearchTitle(scr, frame, topicName, string(currentHelpSearch.query))
	if actual, ok := helpViewScrollTop(frame); ok {
		currentHelpSearch.scrollTop = actual
	}
	contentWidth := (x2 - x1 - 1) - 1 // inner width minus scrollbar
	for matchIndex, match := range currentHelpSearch.matches {
		if match.line < 0 || match.line >= len(topic.Lines) {
			continue
		}
		line, centered := visibleHelpLine(topic.Lines[match.line])
		runes := []rune(line)
		if match.start < 0 || match.end > len(runes) {
			continue
		}
		lineX := x1 + 1
		if centered {
			lineX += (contentWidth - runewidth.StringWidth(line)) / 2
		}
		lineX += runewidth.StringWidth(string(runes[:match.start]))

		lineY := y1 + 1
		if match.line < topic.StickyRows {
			lineY += match.line
		} else {
			lineY += topic.StickyRows + (match.line - topic.StickyRows - currentHelpSearch.scrollTop)
		}
		if lineY < y1+1 || lineY > y2-1 {
			continue
		}
		foreground := vtui.GetRGBFore(vtui.Palette[vtui.ColHelpLink])
		cells := vtui.StringToCharInfo(string(runes[match.start:match.end]), 0)
		if matchIndex == currentHelpSearch.selected {
			selectedAttr := vtui.SetRGBFore(
				vtui.Palette[vtui.ColHelpSelectedLink],
				0xFFFF00,
			)
			for i := range cells {
				cells[i].Attributes = selectedAttr
			}
		} else {
			for i := range cells {
				cells[i].Attributes = vtui.SetRGBFore(scr.GetCell(lineX+i, lineY).Attributes, foreground)
			}
		}
		scr.Write(lineX, lineY, cells)
	}
}

func drawHelpSearchTitle(scr *vtui.ScreenBuf, frame vtui.Frame, topicName, query string) {
	x1, y1, x2, _ := frame.GetPosition()
	// Sample the title drawn by HelpView so both foreground and background stay
	// exactly as they were before search became active.
	baseAttr := scr.GetCell((x1+x2)/2, y1).Attributes
	highlightAttr := vtui.SetRGBFore(baseAttr, vtui.GetRGBFore(vtui.Palette[vtui.ColHelpLink]))
	cells := vtui.StringToCharInfo(" Help: "+topicName+" [", baseAttr)
	cells = append(cells, vtui.StringToCharInfo(query, highlightAttr)...)
	cells = append(cells, vtui.StringToCharInfo("] ", baseAttr)...)
	maxCells := x2 - x1 - 1
	if len(cells) > maxCells {
		cells = cells[:maxCells]
	}
	x := x1 + (x2-x1+1-len(cells))/2
	scr.Write(x, y1, cells)
}
