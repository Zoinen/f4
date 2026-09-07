package main

// Editor "Find All", ported from Far Manager (far/editor.cpp, find_all_list):
// the search dialog's [ All ] button collects every occurrence of the pattern
// and lists each matching line once (Far lists every occurrence) in a popup
// menu titled "Occurrences: N, lines: M". Enter jumps to the line's first
// match, Ctrl+Enter jumps without closing the menu,
// Ctrl+Up/Down scroll the editor behind the menu, F4 dumps the matching
// lines into a new editor, F5 zooms the menu to (almost) full screen.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charlievieth/strcase"
	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// matchSpan is one match in the byte buffer; in a collected list it is the
// first match on its line. Collection runs in a background goroutine, so the
// span carries no editor state.
type matchSpan struct {
	Off, Len int
}

// editorMatch is a span resolved against the line index on the UI thread.
type editorMatch struct {
	Off, Len int
	Line     int // 0-based logical line
	Col      int // 1-based rune column within the line
}

// findAllRow is one occurrence resolved for painting: where it sits, the text
// of its line, and where inside that text the match falls. Rows are produced
// on demand for the dozen or so visible items and thrown away after the paint,
// so the list costs nothing per occurrence.
type findAllRow struct {
	match editorMatch
	text  string // line text, tabs flattened, capped at findAllMaxItemWidth
	// Match range within text. byteEnd == byteStart means there is nothing to
	// highlight: the match starts past the cap, or truncation rewrote the
	// bytes it would have covered.
	byteStart, byteEnd int
}

// findAllFrame is the occurrences list. It renders straight out of the span
// slice the search produced (one span per matching line): VMenu keeps the
// scroll and selection state
// (ItemCount, TopPos, SelectPos, the scrollbar), but its Items slice stays
// empty, because one MenuItem per occurrence costs ~2 µs and ~210 bytes and a
// short pattern in a large file hits millions of times — 21M matches in a
// 200 MB buffer took ~43 s and ~4.4 GB to materialize, all inside the single
// RunOnUI callback that opened the menu. Painting only ever touches the
// visible rows, so nothing about the list needed to exist in the first place.
//
// The price is that the parts of VMenu that read Items — item painting, hover,
// click, Enter — are re-implemented here (vtui ships as a separate module, so
// the menu itself is not extended).
type findAllFrame struct {
	*vtui.VMenu
	ev    *EditorView
	spans []matchSpan

	bottomHint string
	lineW      int // digits reserved for the line number
	colW       int // digits reserved for the column; grows, never shrinks
	accentW    int // width of the "line│col│ " prefix as last painted
	normalRect [4]int
	zoomed     bool
}

// columnAt returns the 1-based rune column of off within its line.
func (f *findAllFrame) columnAt(lineStart, off int) int {
	return 1 + utf8.RuneCount(f.ev.bufferRange(lineStart, off-lineStart))
}

// resolveRow turns occurrence i into everything needed to paint its row.
func (f *findAllFrame) resolveRow(i int) findAllRow {
	size := f.ev.pt.Size()
	s := f.spans[i]
	line := f.ev.li.GetLineAtOffset(s.Off)
	// The background indexer may still be catching up on a large file; clamp
	// every bound against the buffer so a partially indexed file cannot panic
	// or render the rest of the buffer as one line.
	lineStart := min(max(f.ev.li.GetLineOffset(line), 0), size)
	lineEnd := min(lineStart+max(f.ev.getLineLength(line), 0), size)
	// Only as much of the line as could fill the row is read: a minified line
	// megabytes long is not worth reading to paint 512 columns of it.
	raw := strings.TrimRight(f.ev.lineHead(lineStart, lineEnd), "\r\n")
	// Tabs become single spaces: a 1-byte-for-1-byte substitution keeps the
	// match's byte offsets valid in the display string, which keeps the
	// highlight math trivial.
	raw = strings.ReplaceAll(raw, "\t", " ")
	text := vtui.TruncateString(raw, findAllMaxItemWidth, "")

	row := findAllRow{
		match: editorMatch{
			Off:  s.Off,
			Len:  s.Len,
			Line: line,
			Col:  f.columnAt(lineStart, min(max(s.Off, lineStart), size)),
		},
		text: text,
	}

	start := s.Off - lineStart
	end := min(start+s.Len, len(text))
	if start < 0 || start >= len(text) || start >= end ||
		// Truncation sanitizes control characters while rebuilding the
		// string, which shifts byte offsets; highlight only when the display
		// bytes still hold the match where the buffer had it.
		end > len(raw) || text[start:end] != raw[start:end] {
		return row
	}
	row.byteStart, row.byteEnd = start, end
	return row
}

// prefixFor renders the "line│col│ " accent. Every row on a page is formatted
// with the same widths, so the item texts line up.
func (f *findAllFrame) prefixFor(m editorMatch) string {
	return fmt.Sprintf("%*d│%*d│ ", f.lineW, m.Line+1, f.colW, m.Col)
}

// resolvePage resolves the rows currently in view and reserves enough digits
// for the widest column among them.
func (f *findAllFrame) resolvePage(height int) []findAllRow {
	rows := make([]findAllRow, 0, max(height, 0))
	for i := 0; i < height; i++ {
		idx := f.TopPos + i
		if idx < 0 || idx >= len(f.spans) {
			break
		}
		rows = append(rows, f.resolveRow(idx))
	}
	for _, r := range rows {
		f.colW = max(f.colW, len(strconv.Itoa(r.match.Col)))
	}
	return rows
}

func (f *findAllFrame) Show(scr *vtui.ScreenBuf) {
	// Rows are read out of the buffer as they are painted, and on a mapped
	// file that buffer is the file: this paints through the mapping every
	// frame, where the editor's own paint has the same guard for the same
	// reason. A list built from materialized rows never touched it again.
	defer f.ev.guardMapping("listing occurrences")()

	f.VMenu.Show(scr) // box, title and scrollbar; Items is empty, so no rows
	x1, y1, x2, y2 := f.GetPosition()
	p := vtui.NewPainter(scr)
	if f.bottomHint != "" {
		p.DrawTitle(x1, y2, x2, f.bottomHint, vtui.Palette[vtui.ColMenuTitle])
	}

	// Same visible-row bounds as VMenu.DisplayObject.
	rows := f.resolvePage(y2 - y1 - 1)

	prefixes := make([]string, len(rows))
	accentW := 0
	for i, r := range rows {
		prefixes[i] = f.prefixFor(r.match)
		accentW = max(accentW, vtui.StringWidth(prefixes[i]))
	}
	f.accentW = accentW

	// The Painter does not clip, so every string is cut to the room left
	// inside the border. A terminal narrower than the prefix leaves nothing
	// for the text; keep one column so rows degrade instead of going blank.
	innerW := max((x2-x1+1)-4-accentW, 1)

	for i, r := range rows {
		idx := f.TopPos + i
		y := y1 + 1 + i
		attr := vtui.Palette[vtui.ColMenuText]
		hiAttr := vtui.Palette[vtui.ColMenuHighlight]
		if idx == f.SelectPos {
			attr = vtui.Palette[vtui.ColMenuSelectedText]
			hiAttr = vtui.Palette[vtui.ColMenuSelectedHighlight]
		}
		p.Fill(x1+1, y, x2-1, y, ' ', attr)
		p.DrawString(x1+2, y, vtui.TruncateString(prefixes[i], max(x2-2-x1, 0), ""), hiAttr)

		textX := x1 + 2 + accentW
		if textX > x2-1 {
			continue
		}
		text := vtui.TruncateString(r.text, min(innerW, x2-textX), "")
		p.DrawString(textX, y, text, attr)

		if r.byteEnd == r.byteStart {
			continue
		}
		startX := textX + vtui.StringWidth(r.text[:r.byteStart])
		maxX := min(textX+vtui.StringWidth(text)-1, x2-1)
		if startX > maxX {
			continue
		}
		sub := vtui.TruncateString(r.text[r.byteStart:r.byteEnd], maxX-startX+1, "")
		p.DrawString(startX, y, sub, hiAttr)
	}
}

// GetItemCount reports the listed line count. The embedded VMenu would answer
// with len(Items), which this frame deliberately keeps at zero.
func (f *findAllFrame) GetItemCount() int { return f.ItemCount }

// activate jumps to row idx's match and closes the menu. VMenu's own Enter and
// click paths read Items to find the action, so the frame runs them instead.
func (f *findAllFrame) activate(idx int) {
	if idx < 0 || idx >= len(f.spans) {
		return
	}
	s := f.spans[idx]
	// vtui pops the menu after the key handler returns, so the jump is posted
	// for after the frame stack has settled.
	vtui.FrameManager.PostTask(func() {
		f.ev.selectFoundPattern(s.Off, s.Len)
	})
	f.SetExitCode(idx)
}

// ProcessMouse re-implements VMenu's hover and click handling against the
// span slice, for the same reason as activate.
func (f *findAllFrame) ProcessMouse(e *vtinput.InputEvent) bool {
	if f.IsDisabled() || e.Type != vtinput.MouseEventType {
		return false
	}
	if f.HandleMouseScroll(e) {
		return true
	}
	if (e.MouseEventFlags & vtinput.MouseMoved) != 0 {
		x1, _, x2, _ := f.GetPosition()
		if mx := int(e.MouseX); mx <= x1 || mx >= x2 {
			return false
		}
		idx := f.GetClickIndex(int(e.MouseY))
		if idx == -1 {
			return false
		}
		f.SetSelectPos(idx)
		return true
	}
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		if idx := f.GetClickIndex(int(e.MouseY)); idx != -1 {
			f.SetSelectPos(idx)
			f.activate(idx)
			return true
		}
	}
	return false
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
		x1, y1, x2, y2 := f.GetPosition()
		r = clampMenuRect([4]int{x1, y1, x2, y2}, w, h)
	}
	f.SetPosition(r[0], r[1], r[2], r[3])
	f.SetSelectPos(f.SelectPos) // re-clamp TopPos to the new height
}

// findAllMaxItemWidth caps the stored display text so a single minified
// megabyte-long line cannot bloat the menu.
const findAllMaxItemWidth = 512

// findAllMaxLineBytes caps how much of a line is read to fill one row. It is
// generous against findAllMaxItemWidth — sixteen bytes per column — because
// the cap that matters is on the display width, and this one only has to keep
// a single enormous line from being read in full for every paint of it.
const findAllMaxLineBytes = 16 * findAllMaxItemWidth

// lineHead reads the line [lineStart, lineEnd) up to findAllMaxLineBytes. A
// line cut there is cut on a character boundary and ends in an ellipsis, so
// the text is valid UTF-8 and says that it is not all of the line: the found
// lines dump writes it into a buffer the user can save.
func (ev *EditorView) lineHead(lineStart, lineEnd int) string {
	n := lineEnd - lineStart
	if n <= findAllMaxLineBytes {
		return string(ev.bufferRange(lineStart, n))
	}
	head := ev.bufferRange(lineStart, findAllMaxLineBytes)
	for i := len(head) - 1; i >= 0 && i >= len(head)-utf8.UTFMax; i-- {
		if utf8.RuneStart(head[i]) {
			if !utf8.FullRune(head[i:]) {
				head = head[:i]
			}
			break
		}
	}
	return string(head) + "…"
}

// findAllWidthSample is how many occurrences are measured to pick the menu
// width. The list itself is unbounded, and the longest of twenty million
// matching lines cannot be found without resolving all of them, so the width
// hugs the longest line in the first sample instead. Lists shorter than this
// are still sized exactly; longer ones get F5 to zoom.
const findAllWidthSample = 1000

// findAllDumpMaxLines caps the F4 dump. The list renders lazily, but the dump
// materializes real text into a new editor, and dumping every matching line of
// a file that matches on all 21M of them would be a second copy of the file.
// A var so the test can reach the cap without a file that large.
var findAllDumpMaxLines = 100000

// firstMatchPerLine keeps the first occurrence on each line, so the list shows
// a line once however many times it matches. Two consecutive matches sit on
// different lines exactly when a '\n' separates them, so the whole list costs
// one forward pass over the bytes between the first and the last match: the
// spans are ordered and non-overlapping, so the gaps scanned are disjoint.
// Resolving each match against the line index instead would be a locked
// binary search per occurrence, seconds of them at the counts this path
// exists for. The filtering is done in place. Runs on the collecting
// goroutine, off the UI thread.
func firstMatchPerLine(ctx context.Context, data []byte, spans []matchSpan) []matchSpan {
	if len(spans) == 0 {
		return spans
	}
	kept := spans[:1]
	off := min(max(spans[0].Off, 0), len(data))
	for i, s := range spans[1:] {
		if ctx != nil && i%1024 == 0 && ctx.Err() != nil {
			return kept
		}
		end := min(max(s.Off, off), len(data))
		if bytes.IndexByte(data[off:end], '\n') >= 0 {
			kept = append(kept, s)
		}
		off = end
	}
	return kept
}

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
	// pattern (K U+212A matches "k"), hence CutPrefix per match. The string
	// is a view, not a copy: on a mapped file, copying here would cost the
	// whole file's size in heap for the most ordinary search there is.
	text := bytesToString(data)
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

// errSearchBuffer marks a collection that failed to read the text, as opposed
// to one that failed to understand the pattern: the two are different dialogs.
var errSearchBuffer = errors.New("cannot read the buffer to search it")

// findAllWindow is how much of the buffer one collection pass holds at a time.
// Large enough that the reads behind it are sequential reads rather than a
// stream of small ones, small enough to be nothing on the heap. A var so the
// tests can shrink it and put a match on every seam.
var findAllWindow = 4 << 20

// findAllSampleTrust is how far the match density measured over a stretch of
// the file is extrapolated past it when sizing the result: a 4 MB window is
// read as saying something about the next 64 MB, not about the next 8 GB.
const findAllSampleTrust = 16

// collectMatchSpans finds the first occurrence of pattern on every line of the
// buffer, and how many occurrences there are, reading the buffer a window at a time.
//
// Handing the whole buffer to the matcher is what made Find All unusable on a
// large file. On a mapped file it faults every page of the file into residency
// — 8 GB of it, on a machine with 16, while the line index is scanning the
// same file — and the page-at-a-time faulting is a quarter of the speed of
// reading it. On a file that has been edited there is no window to hand out at
// all, so the buffer is assembled into the heap instead: another whole copy.
// A window costs 4 MB either way.
//
// Consecutive windows overlap, so no match can fall between two of them, and
// the scan never looks back before the end of the last match it reported, so
// none is reported twice.
//
// pt and readFromFile are the buffer and the file as they were on the UI
// thread when the search was asked for, captured there for the same reason
// StartIndexing captures them: a save landing mid-scan swaps and closes both
// underneath a task goroutine reading them.
func (ev *EditorView) collectMatchSpans(ctx *vtui.TaskContext, session int, pt *piecetable.PieceTable,
	readFromFile fileChunkReader, pattern string, caseSensitive, useRegex, wholeWord bool) ([]matchSpan, int, error) {

	// The regex engine matches against one contiguous buffer and its patterns
	// have no bounded length, so a window cannot stand in for the file. That
	// path still assembles, and still pays for it.
	if useRegex || wholeWord {
		data, err := ev.searchBuffer(ctx, session)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %w", errSearchBuffer, err)
		}
		spans, err := findAllMatchSpans(ctx, data, pattern, caseSensitive, useRegex, wholeWord)
		if err != nil {
			return nil, 0, err
		}
		return firstMatchPerLine(ctx, data, spans), len(spans), nil
	}

	size := pt.Size()
	if size == 0 || pattern == "" {
		return nil, 0, nil
	}

	// A folded match can be longer in bytes than the pattern it matched, so
	// the tail repeated between windows is measured in the longest a match
	// could be, not in the length of the pattern.
	overlap := max(4*len(pattern), 64)
	buf := make([]byte, min(findAllWindow+overlap, size))

	var spans []matchSpan
	occurrences := 0
	// Where the last match ended, and whether a newline has been seen since
	// it — which is how a line is listed once however many times it matches,
	// without asking the index per occurrence.
	lastEnd := -1
	sawNewline := false

	for pos := 0; pos < size; {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		win, err := ev.readSearchWindow(ctx, readFromFile, pt, buf, pos, min(len(buf), size-pos))
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %w", errSearchBuffer, err)
		}

		from := 0
		if lastEnd > pos {
			from = min(lastEnd-pos, len(win))
		}
		found, err := findAllMatchSpans(ctx, win[from:], pattern, caseSensitive, false, false)
		if err != nil {
			return nil, 0, err
		}

		// What has been scanned says how thickly matching lines occur, which
		// is enough to ask for the result ahead of time instead of letting
		// append copy it into place a dozen times over. The density is that
		// of spans actually kept, not of raw occurrences: a minified line
		// matching a million times keeps one span, and sizing by occurrences
		// would reserve the million. A sample is only trusted so far, though:
		// a dense header extrapolated over 8 GB asked for more spans than the
		// machine had memory for, so the reservation covers at most
		// findAllSampleTrust times the bytes measured, and is measured again,
		// over more of the file, when it fills. Until there is a sample of
		// kept spans, append grows the slice by itself.
		if len(spans) > 0 && len(spans)+len(found) > cap(spans) {
			scanned := float64(pos + len(win))
			perByte := float64(len(spans)) / scanned
			estimate := perByte * min(float64(size), scanned*findAllSampleTrust) * 1.1
			if estimate > float64(cap(spans)) && estimate < float64(math.MaxInt32) {
				grown := make([]matchSpan, len(spans), int(estimate))
				copy(grown, spans)
				spans = grown
			}
		}

		for _, m := range found {
			abs := pos + from + m.Off
			occurrences++
			// A match opens a new listed line when it is the first ever, or a
			// newline separates it from the previous match — seen either in
			// an earlier window or in the bytes in hand since that match;
			// anything before those was checked when the window holding it
			// was done with.
			if len(spans) == 0 || sawNewline ||
				bytes.IndexByte(win[max(lastEnd-pos, 0):abs-pos], '\n') >= 0 {
				spans = append(spans, matchSpan{abs, m.Len})
			}
			sawNewline = false
			lastEnd = abs + m.Len
		}

		// Whatever follows the last match in this window decides whether the
		// next one starts on a new line — and a window with no match in it
		// still has newlines to report, or a match two windows on would be
		// counted against the line of the one before.
		if lastEnd >= 0 && !sawNewline {
			if tail := max(lastEnd-pos, 0); tail <= len(win) {
				sawNewline = bytes.IndexByte(win[tail:], '\n') >= 0
			}
		}

		if len(win) < len(buf) || pos+len(win) >= size {
			break // that was the last window
		}
		pos += len(win) - overlap
	}
	return spans, occurrences, nil
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
		// One view of the buffer for the whole pass: the file may be reloaded
		// or the buffer swapped underneath a search of an 8 GB file, and half
		// a scan of each is not a search of either.
		pt := ev.pt
		readFromFile := ev.chunkReader()

		runSearchWithProgress(pattern, func(ctx *vtui.TaskContext, dlg *vtui.Window) {
			defer ev.guardMapping("collecting occurrences")()

			// One span per matching line, plus the total occurrence count,
			// come out of the same pass, so that opening the menu stays
			// O(one screenful) however many occurrences there are.
			spans, occurrences, err := ev.collectMatchSpans(ctx, session, pt, readFromFile, pattern, caseSensitive, useRegex, wholeWord)
			if errors.Is(err, errSearchBuffer) {
				if ctx.Err() != nil {
					return // canceled; the dialog is already closing
				}
				ctx.RunOnUI(func() {
					dlg.Close()
					vtui.ShowMessage(" Error ", "Failed to read file buffer.", []string{"&Ok"})
				})
				return
			}

			// The list is a window onto these offsets and asks the index for
			// the line of each row as it paints it, so it cannot open before
			// the index can answer.
			indexed := true
			if err == nil && len(spans) > 0 {
				indexed = ev.awaitIndexForResults(ctx, spans[len(spans)-1].Off)
			}

			ctx.RunOnUI(func() {
				// Closing the dialog cancels the task via OnResult; read the
				// state before Close so normal completions still deliver.
				canceled := ctx.Err() != nil
				dlg.Close()
				if canceled || !indexed {
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
				ev.showFindAllMenu(pattern, spans, occurrences)
			})
		})
	})
}

func (ev *EditorView) showFindAllMenu(pattern string, spans []matchSpan, occurrences int) {
	// Sizing the menu resolves a sample of rows, which reads the buffer here
	// on the UI thread, mapping and all.
	defer ev.guardMapping("sizing the occurrences list")()

	// Every row is resolved against the index as it is painted, so the index
	// has to reach the last occurrence before the list opens. The task waited
	// for a running scan to get there; this covers the case where there was no
	// scan to wait for, by counting the remainder here — the same gap-filling
	// a search match gets in selectFoundPattern.
	if len(spans) > 0 {
		ev.ensureIndexedTo(spans[len(spans)-1].Off)
	}

	menuTitle := " " + fmt.Sprintf(Msg("Search.AllStatistics"), occurrences, len(spans)) + " "
	menu := vtui.NewVMenu(menuTitle)
	// The menu holds no items: it is a window onto the spans, and everything
	// VMenu would read out of Items the frame answers from them instead.
	menu.ItemCount = len(spans)
	menu.IsSelectable = func(i int) bool { return i >= 0 && i < len(spans) }

	frame := &findAllFrame{
		VMenu:      menu,
		ev:         ev,
		spans:      spans,
		bottomHint: Msg("Search.AllBottomHint"),
		// The last occurrence sits on the highest line number in the list, so
		// one lookup fixes the width of the line column for good.
		lineW: len(strconv.Itoa(ev.li.GetLineAtOffset(spans[len(spans)-1].Off) + 1)),
		colW:  1,
	}

	// Sizing needs real rows, so a bounded sample of the head is resolved
	// here; every other row is resolved when it is painted.
	maxDisplayW := 0
	for i := 0; i < min(len(spans), findAllWidthSample); i++ {
		r := frame.resolveRow(i)
		maxDisplayW = max(maxDisplayW, vtui.StringWidth(r.text))
		frame.colW = max(frame.colW, len(strconv.Itoa(r.match.Col)))
	}
	// Measured, not counted: '│' is East-Asian-Ambiguous and renders two
	// cells wide under CJK locales, where a lineW+colW+3 guess would paint
	// the highlight two columns off. The numbers formatted here do not matter,
	// only the widths they are padded to.
	accentW := vtui.StringWidth(frame.prefixFor(editorMatch{}))
	frame.accentW = accentW

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
	h := len(spans) + 2
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
	menu.SetSelectPos(0) // AddItem would have done this for a normal menu
	frame.normalRect = [4]int{x, y, x + w - 1, y + h - 1}

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
				if idx := menu.SelectPos; idx >= 0 && idx < len(spans) {
					s := spans[idx]
					ev.selectFoundPattern(s.Off, s.Len)
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
			case vtinput.VK_RETURN:
				// VMenu's own Enter handling reads Items to find the action,
				// and this menu has none.
				frame.activate(menu.SelectPos)
				return true
			case vtinput.VK_F4:
				menu.Close()
				vtui.FrameManager.PostTask(func() {
					ev.openFoundLinesEditor(pattern, spans)
				})
				return true
			case vtinput.VK_F5:
				scrW := vtui.FrameManager.GetScreenSize()
				scrH := vtui.FrameManager.GetScreenHeight()
				var r [4]int
				if !frame.zoomed {
					fx1, fy1, fx2, fy2 := frame.GetPosition()
					frame.normalRect = [4]int{fx1, fy1, fx2, fy2}
					r = zoomRect(scrW, scrH)
					frame.zoomed = true
				} else {
					r = clampMenuRect(frame.normalRect, scrW, scrH)
					frame.zoomed = false
				}
				frame.SetPosition(r[0], r[1], r[2], r[3])
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

	vtui.FrameManager.PushMenu(frame)
}

// openFoundLinesEditor dumps the matching lines (the spans hold one match
// per line), each prefixed with its 1-based line number, into a fresh editor
// (Far's F4 in the find-all list). Unlike the list, this one materializes
// text, so it stops after findAllDumpMaxLines and says how much it left out.
func (ev *EditorView) openFoundLinesEditor(pattern string, spans []matchSpan) {
	if len(spans) == 0 {
		return
	}
	// The dump copies the matching lines out of the buffer, so it reads
	// through the mapping too.
	defer ev.guardMapping("dumping the matching lines")()

	lineW := len(strconv.Itoa(ev.li.GetLineAtOffset(spans[len(spans)-1].Off) + 1))
	size := ev.pt.Size()

	var b strings.Builder
	lines, i, prevLine := 0, 0, -1
	for ; i < len(spans) && lines < findAllDumpMaxLines; i++ {
		line := ev.li.GetLineAtOffset(spans[i].Off)
		if line == prevLine {
			continue // a still-indexing file can report a zero-length line
		}
		prevLine = line
		lineStart := min(max(ev.li.GetLineOffset(line), 0), size)
		lineEnd := min(lineStart+max(ev.getLineLength(line), 0), size)
		raw := strings.TrimRight(ev.lineHead(lineStart, lineEnd), "\r\n")
		fmt.Fprintf(&b, "%*d: %s\n", lineW, line+1, raw)
		lines++
	}
	if left := len(spans) - i; left > 0 {
		fmt.Fprintf(&b, Msg("Search.AllEditorMore")+"\n", left)
	}

	editor := NewEditorView(piecetable.New([]byte(b.String())), nil, "")
	editor.DisplayTitle = fmt.Sprintf(Msg("Search.AllEditorTitle"), pattern)
	editor.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
	editor.StartIndexing()
	vtui.FrameManager.AddScreen(editor)
}
