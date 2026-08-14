package main

// Editor "Find All", ported from Far Manager (far/editor.cpp, find_all_list):
// the search dialog's [ All ] button collects every occurrence of the pattern
// and lists them in a popup menu titled "Occurrences: N, lines: M". Enter
// jumps to the occurrence, Ctrl+Enter jumps without closing the menu,
// Ctrl+Up/Down scroll the editor behind the menu, F4 dumps the matching
// lines into a new editor, F5 zooms the menu to (almost) full screen.

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charlievieth/strcase"
	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// matchSpan is one occurrence in the byte buffer. Collection runs in a
// background goroutine, so the span carries no editor state.
type matchSpan struct {
	Off, Len int
}

// editorMatch is a span resolved against the line index on the UI thread.
type editorMatch struct {
	Off, Len int
	Line     int // 0-based logical line
	Col      int // 1-based rune column within the line
}

// findAllRow holds per-item metrics for painting the match highlight over
// the menu text. Byte offsets index the untruncated display string; cell
// values are terminal columns.
type findAllRow struct {
	byteStart, byteEnd int // match range in the display string
	cellStart          int // columns before the match
	cellWidth          int // 0 = nothing to highlight (match truncated away)
	visW               int // width of the currently truncated item text
}

// findAllFrame wraps the occurrences VMenu to draw the key hint on the
// bottom border and repaint each visible match in the highlight color,
// following the userMenuFrame pattern (vtui ships as a separate module,
// so the menu itself is not extended).
type findAllFrame struct {
	*vtui.VMenu
	bottomHint string
	accentW    int      // width of the "line│col│ " prefix, same for all items
	displays   []string // untruncated (but capped) display line per item
	rows       []findAllRow
	normalRect [4]int
	zoomed     bool
}

func (f *findAllFrame) Show(scr *vtui.ScreenBuf) {
	f.VMenu.Show(scr)
	x1, y1, x2, y2 := f.VMenu.GetPosition()
	p := vtui.NewPainter(scr)
	if f.bottomHint != "" {
		p.DrawTitle(x1, y2, x2, f.bottomHint, vtui.Palette[vtui.ColMenuTitle])
	}

	// Same visible-row bounds as VMenu.DisplayObject.
	height := y2 - y1 - 1
	for i := 0; i < height; i++ {
		idx := f.TopPos + i
		if idx >= len(f.rows) {
			break
		}
		r := f.rows[idx]
		if r.cellWidth == 0 {
			continue
		}
		textX := x1 + 2 + f.accentW
		startX := textX + r.cellStart
		maxX := textX + r.visW - 1
		if maxX > x2-1 {
			maxX = x2 - 1
		}
		if startX > maxX {
			continue
		}
		attr := vtui.Palette[vtui.ColMenuHighlight]
		if idx == f.SelectPos {
			attr = vtui.Palette[vtui.ColMenuSelectedHighlight]
		}
		sub := f.displays[idx][r.byteStart:r.byteEnd]
		p.DrawString(startX, y1+1+i, vtui.TruncateString(sub, maxX-startX+1, ""), attr)
	}
}

// retruncate refits every item text to the menu's current inner width.
// Needed after each geometry change: the Painter does not clip, so an item
// longer than the box would bleed past the border.
func (f *findAllFrame) retruncate() {
	x1, _, x2, _ := f.VMenu.GetPosition()
	innerW := (x2 - x1 + 1) - 4 - f.accentW
	// A terminal narrower than the line/col prefix leaves no room for text;
	// keep one column so the items degrade instead of going blank.
	if innerW < 1 {
		innerW = 1
	}
	for i := range f.Items {
		trunc := vtui.TruncateString(f.displays[i], innerW, "")
		f.Items[i].Text = escapeAmpersand(trunc)
		f.rows[i].visW = vtui.StringWidth(trunc)
	}
}

// zoomRect is the near-full-screen position used while F5-zoomed.
func zoomRect(scrW, scrH int) [4]int {
	return clampMenuRect([4]int{2, 1, scrW - 3, scrH - 2}, scrW, scrH)
}

// clampMenuRect moves and, if needed, shrinks a rect to fit a w x h screen.
func clampMenuRect(r [4]int, w, h int) [4]int {
	rw := min(max(r[2]-r[0]+1, 6), max(w, 6))
	rh := min(max(r[3]-r[1]+1, 3), max(h, 3))
	x := min(max(r[0], 0), max(w-rw, 0))
	y := min(max(r[1], 0), max(h-rh, 0))
	return [4]int{x, y, x + rw - 1, y + rh - 1}
}

// ResizeConsole refits the menu when the terminal changes size. The
// embedded VMenu ignores resizes, which would leave a zoomed menu painting
// past the new screen edge and restore an off-screen pre-zoom rect.
func (f *findAllFrame) ResizeConsole(w, h int) {
	f.normalRect = clampMenuRect(f.normalRect, w, h)
	var r [4]int
	if f.zoomed {
		r = zoomRect(w, h)
	} else {
		x1, y1, x2, y2 := f.VMenu.GetPosition()
		r = clampMenuRect([4]int{x1, y1, x2, y2}, w, h)
	}
	f.SetPosition(r[0], r[1], r[2], r[3])
	f.retruncate()
	f.SetSelectPos(f.SelectPos) // re-clamp TopPos to the new height
}

// findAllMaxItemWidth caps the stored display text so a single minified
// megabyte-long line cannot bloat the menu.
const findAllMaxItemWidth = 512

// findAllMatchSpans returns every non-overlapping occurrence of pattern in
// data, using the same pattern-building rules as EditorView.Search. A nil
// ctx disables cancellation.
func findAllMatchSpans(ctx context.Context, data []byte, pattern string, caseSensitive, useRegex, wholeWord bool) ([]matchSpan, error) {
	if pattern == "" {
		return nil, nil
	}

	// Only whole-word matching needs the regex engine (for the \b wrapping);
	// literal search, case-sensitive or folded, is handled below without it.
	if useRegex || wholeWord {
		re, err := buildSearchRegex(pattern, caseSensitive, useRegex, wholeWord)
		if err != nil {
			return nil, err
		}
		var spans []matchSpan
		for _, loc := range re.FindAllIndex(data, -1) {
			if loc[1] == loc[0] {
				continue // a zero-length match is not an occurrence
			}
			spans = append(spans, matchSpan{loc[0], loc[1] - loc[0]})
		}
		return spans, nil
	}

	var spans []matchSpan
	if caseSensitive {
		pat := []byte(pattern)
		curr := 0
		for {
			// A canceled scan of a huge buffer should stop burning the
			// core once the progress dialog is gone.
			if ctx != nil && len(spans)%1024 == 0 && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			idx := bytes.Index(data[curr:], pat)
			if idx < 0 {
				break
			}
			off := curr + idx
			spans = append(spans, matchSpan{off, len(pat)})
			curr = off + len(pat)
		}
		return spans, nil
	}

	// strcase folds while scanning the original data, so the offsets need
	// no translation; a folded match can differ in byte length from the
	// pattern (K U+212A matches "k"), hence CutPrefix per match.
	text := string(data)
	curr := 0
	for {
		if ctx != nil && len(spans)%1024 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		idx := strcase.Index(text[curr:], pattern)
		if idx < 0 {
			break
		}
		off := curr + idx
		after, ok := strcase.CutPrefix(text[off:], pattern)
		if !ok {
			break // cannot happen: Index just matched at off
		}
		spans = append(spans, matchSpan{off, len(text) - off - len(after)})
		curr = len(text) - len(after)
	}
	return spans, nil
}

// FindAll runs the search over the whole buffer and opens the occurrences
// menu. The async skeleton mirrors EditorView.Search.
func (ev *EditorView) FindAll(pattern string, caseSensitive, useRegex, wholeWord bool) {
	if pattern == "" {
		return
	}

	vtui.FrameManager.PostTask(func() {
		// The menu resolves byte offsets against the live line index, so
		// results collected before an edit must be dropped, not shown.
		session := ev.editSession

		runSearchWithProgress(pattern, func(ctx *vtui.TaskContext, dlg *vtui.Window) {
			bytes, errBytes := ev.pt.Bytes()
			if errBytes != nil {
				ctx.RunOnUI(func() {
					dlg.Close()
					vtui.ShowMessage(" Error ", "Failed to read file buffer.", []string{"&Ok"})
				})
				return
			}

			spans, err := findAllMatchSpans(ctx, bytes, pattern, caseSensitive, useRegex, wholeWord)

			ctx.RunOnUI(func() {
				// Closing the dialog cancels the task via OnResult; read the
				// state before Close so normal completions still deliver.
				canceled := ctx.Err() != nil
				dlg.Close()
				if canceled {
					return
				}
				if err != nil {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Invalid regular expression:\n%v", err), []string{"&Ok"})
					return
				}
				if ev.editSession != session {
					return // buffer changed while scanning; offsets are stale
				}
				if len(spans) == 0 {
					vtui.ShowMessage(Msg("Search.Title"), Msg("Search.NotFound"), []string{Msg("vtui.Ok")})
					return
				}
				ev.showFindAllMenu(pattern, bytes, spans)
			})
		})
	})
}

func (ev *EditorView) showFindAllMenu(pattern string, data []byte, spans []matchSpan) {
	matches := make([]editorMatch, len(spans))
	displays := make([]string, len(spans))
	rows := make([]findAllRow, len(spans))

	uniqueLines := 0
	prevLine := -1
	prevLineStart := 0
	prevRaw := ""     // tab-replaced line text, offsets 1:1 with the buffer
	prevDisplay := "" // prevRaw truncated (and sanitized) to findAllMaxItemWidth
	prevRuneOff := 0
	prevCol := 1
	maxCol := 1
	for i, s := range spans {
		line := ev.li.GetLineAtOffset(s.Off)
		if line != prevLine {
			uniqueLines++
			prevLine = line
			// The background indexer may still be catching up on a large
			// file; clamp every bound against the snapshot so a partially
			// indexed file cannot panic or materialize the rest of the
			// buffer as one line.
			prevLineStart = min(ev.li.GetLineOffset(line), len(data))
			lineEnd := min(prevLineStart+max(ev.getLineLength(line), 0), len(data))
			raw := strings.TrimRight(string(data[prevLineStart:lineEnd]), "\r\n")
			// Tabs become single spaces: a 1-byte-for-1-byte substitution
			// keeps the match's byte offsets valid in the display string,
			// which keeps the highlight math trivial.
			prevRaw = strings.ReplaceAll(raw, "\t", " ")
			prevDisplay = vtui.TruncateString(prevRaw, findAllMaxItemWidth, "")
			prevRuneOff = prevLineStart
			prevCol = 1
		}
		// Spans arrive in offset order, so the column is counted from the
		// previous match on the same line: a long line with many matches is
		// scanned once overall, not once per match.
		if prevRuneOff > s.Off {
			prevRuneOff = s.Off
		}
		col := prevCol + utf8.RuneCount(data[prevRuneOff:s.Off])
		prevRuneOff = s.Off
		prevCol = col
		matches[i] = editorMatch{Off: s.Off, Len: s.Len, Line: line, Col: col}
		if col > maxCol {
			maxCol = col
		}

		display := prevDisplay
		displays[i] = display

		start := s.Off - prevLineStart
		end := min(start+s.Len, len(display))
		if start < 0 || start >= len(display) || start >= end ||
			// Truncation sanitizes control characters while rebuilding the
			// string, which shifts byte offsets; highlight only when the
			// display bytes still hold the match where the buffer had it.
			end > len(prevRaw) || display[start:end] != prevRaw[start:end] {
			rows[i] = findAllRow{}
			continue
		}
		rows[i] = findAllRow{
			byteStart: start,
			byteEnd:   end,
			cellStart: vtui.StringWidth(display[:start]),
			cellWidth: vtui.StringWidth(display[start:end]),
		}
	}

	lineW := len(strconv.Itoa(matches[len(matches)-1].Line + 1))
	colW := len(strconv.Itoa(maxCol))
	prefixFor := func(m editorMatch) string {
		return fmt.Sprintf("%*d│%*d│ ", lineW, m.Line+1, colW, m.Col)
	}
	// Measured, not counted: '│' is East-Asian-Ambiguous and renders two
	// cells wide under CJK locales, where a lineW+colW+3 guess would paint
	// the highlight two columns off.
	accentW := vtui.StringWidth(prefixFor(matches[0]))

	menuTitle := " " + fmt.Sprintf(Msg("Search.AllStatistics"), len(matches), uniqueLines) + " "
	menu := vtui.NewVMenu(menuTitle)
	maxDisplayW := 0
	for i := range matches {
		if w := vtui.StringWidth(displays[i]); w > maxDisplayW {
			maxDisplayW = w
		}
		menu.AddItem(vtui.MenuItem{
			AccentPrefix: prefixFor(matches[i]),
			// Text is filled by the initial retruncate below.
			UserData: i,
		})
	}

	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	w := accentW + maxDisplayW + 4
	if minW := vtui.StringWidth(menuTitle) + 6; w < minW {
		w = minW
	}
	if minW := accentW + 10; w < minW {
		w = minW
	}
	if w > scrW-4 {
		w = scrW - 4
	}
	if w < 10 {
		w = 10
	}
	h := len(matches) + 2
	if h > 12 {
		h = 12 // Far caps the list at 10 rows plus the borders
	}
	if h > scrH-2 {
		h = scrH - 2
	}
	if h < 3 {
		h = 3
	}
	x := max((scrW-w)/2, 0)
	y := max((scrH-h)/2, 0)
	menu.SetPosition(x, y, x+w-1, y+h-1)

	frame := &findAllFrame{
		VMenu:      menu,
		bottomHint: Msg("Search.AllBottomHint"),
		accentW:    accentW,
		displays:   displays,
		rows:       rows,
		normalRect: [4]int{x, y, x + w - 1, y + h - 1},
	}
	frame.retruncate()

	// vtui pops the menu after OnAction returns, so the jump is posted for
	// after the frame stack has settled.
	menu.OnAction = func(idx int) {
		if idx < 0 || idx >= len(matches) {
			return
		}
		m := matches[idx]
		vtui.FrameManager.PostTask(func() {
			ev.selectFoundPattern(m.Off, m.Len)
		})
	}

	menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		if !e.KeyDown {
			return false
		}
		shift := e.ControlKeyState&vtinput.ShiftPressed != 0
		ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
		alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
		ctrlOnly := ctrl && !shift && !alt
		noMods := !ctrl && !shift && !alt

		if ctrlOnly {
			switch e.VirtualKeyCode {
			case vtinput.VK_RETURN:
				// Jump to the occurrence but keep the menu open.
				if idx := menu.SelectPos; idx >= 0 && idx < len(matches) {
					m := matches[idx]
					ev.selectFoundPattern(m.Off, m.Len)
				}
				return true
			case vtinput.VK_UP, vtinput.VK_DOWN:
				// Scroll the editor behind the menu, cursor stays put.
				h := ev.Y2 - ev.Y1
				maxTop := max(ev.engine.GetTotalVisualRows()-h, 0)
				if e.VirtualKeyCode == vtinput.VK_UP {
					ev.ScrollTopRow--
				} else {
					ev.ScrollTopRow++
				}
				ev.ScrollTopRow = min(max(ev.ScrollTopRow, 0), maxTop)
				vtui.FrameManager.Redraw()
				return true
			}
		}

		if noMods {
			switch e.VirtualKeyCode {
			case vtinput.VK_F4:
				menu.Close()
				vtui.FrameManager.PostTask(func() {
					ev.openFoundLinesEditor(pattern, matches, data)
				})
				return true
			case vtinput.VK_F5:
				scrW := vtui.FrameManager.GetScreenSize()
				scrH := vtui.FrameManager.GetScreenHeight()
				var r [4]int
				if !frame.zoomed {
					fx1, fy1, fx2, fy2 := frame.VMenu.GetPosition()
					frame.normalRect = [4]int{fx1, fy1, fx2, fy2}
					r = zoomRect(scrW, scrH)
					frame.zoomed = true
				} else {
					r = clampMenuRect(frame.normalRect, scrW, scrH)
					frame.zoomed = false
				}
				frame.SetPosition(r[0], r[1], r[2], r[3])
				frame.retruncate()
				frame.SetSelectPos(frame.SelectPos) // re-clamp TopPos to the new height
				vtui.FrameManager.Redraw()
				return true
			}
		}

		// Swallow the rest of the unmodified F-key range so vtui's global
		// handlers (F1 help, F9 menu bar, F12 screen list) stay quiet under
		// the modal menu; F10 is left to vtui, which closes the menu.
		if noMods &&
			e.VirtualKeyCode >= vtinput.VK_F1 && e.VirtualKeyCode <= vtinput.VK_F12 &&
			e.VirtualKeyCode != vtinput.VK_F10 {
			return true
		}
		return false
	}

	vtui.FrameManager.Push(frame)
}

// openFoundLinesEditor dumps the unique matching lines, each prefixed with
// its 1-based line number, into a fresh editor (Far's F4 in the find-all
// list).
func (ev *EditorView) openFoundLinesEditor(pattern string, matches []editorMatch, data []byte) {
	if len(matches) == 0 {
		return
	}
	lineW := len(strconv.Itoa(matches[len(matches)-1].Line + 1))

	var b strings.Builder
	prev := -1
	for _, m := range matches {
		if m.Line == prev {
			continue
		}
		prev = m.Line
		lineStart := min(ev.li.GetLineOffset(m.Line), len(data))
		lineEnd := min(lineStart+max(ev.getLineLength(m.Line), 0), len(data))
		raw := strings.TrimRight(string(data[lineStart:lineEnd]), "\r\n")
		fmt.Fprintf(&b, "%*d: %s\n", lineW, m.Line+1, raw)
	}

	editor := NewEditorView(piecetable.New([]byte(b.String())), nil, "")
	editor.DisplayTitle = fmt.Sprintf(Msg("Search.AllEditorTitle"), pattern)
	editor.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
	editor.StartIndexing()
	vtui.FrameManager.AddScreen(editor)
}
