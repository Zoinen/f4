package main

import (
	"bytes"
	"sort"
	"strings"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Multi-caret editing. Every key that acts on the text through more than one
// caret goes through one primitive: build the whole set of replacements
// against the text as it stands, then apply them.
//
// Building first is what keeps the offsets comparable. A caret's offset means
// nothing once a neighbour has inserted or removed bytes before it, so the
// edits are described up front, applied from the last one backwards — which
// never moves the text the earlier ones describe — and the carets are then
// placed by arithmetic rather than by re-deriving them from the buffer.

// caretEdit replaces del bytes at off with ins, on behalf of one caret. A
// caret that has nothing to do this round still gets an entry, with neither a
// deletion nor an insertion, so that it survives the edit and is carried along
// by whatever the other carets did before it.
type caretEdit struct {
	off int
	del int
	ins []byte
	// after puts the caret past the inserted text (typing) rather than at
	// the edit site (deleting).
	after bool
	// primary marks the caret that stays the primary one afterwards.
	primary bool
}

// caretSpan is one caret and whatever it has selected, which for the primary
// caret lives in selActive/selAnchorOffset and for the others in the caret
// itself. Everything that edits works from these rather than from bare
// offsets, so a selection is replaced by what is typed over it.
type caretSpan struct {
	off      int
	selStart int
	selEnd   int
	primary  bool
}

func (c caretSpan) hasSel() bool { return c.selEnd > c.selStart }

// caretSpans returns every caret with its selection, in ascending order and
// without duplicates.
func (ev *EditorView) caretSpans() []caretSpan {
	primaryOff := ev.caretOffset()
	spans := make([]caretSpan, 0, len(ev.extraCursors)+1)
	primary := caretSpan{off: primaryOff, selStart: primaryOff, selEnd: primaryOff, primary: true}
	if ev.selActive && !ev.rectSelActive {
		primary.selStart, primary.selEnd = ev.getSelectionRange()
	}
	spans = append(spans, primary)

	size := ev.pt.Size()
	for _, caret := range ev.extraCursors {
		if caret.off < 0 || caret.off > size || caret.off == primaryOff {
			continue
		}
		start, end := caret.selRange()
		spans = append(spans, caretSpan{off: caret.off, selStart: start, selEnd: end})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].off < spans[j].off })
	return spans
}

// buildCaretEdits calls build for every caret, in ascending order.
func (ev *EditorView) buildCaretEdits(build func(c caretSpan) caretEdit) []caretEdit {
	spans := ev.caretSpans()
	edits := make([]caretEdit, 0, len(spans))
	for _, span := range spans {
		edit := build(span)
		edit.primary = span.primary
		edits = append(edits, edit)
	}
	return edits
}

// applyCaretEdits performs the whole set as one undoable change and leaves a
// caret at each site. It reports whether the buffer changed.
func (ev *EditorView) applyCaretEdits(edits []caretEdit, op undoOpType) bool {
	if len(edits) == 0 {
		return false
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].off < edits[j].off })

	// Two carets close enough for their deletions to meet: the later edit
	// keeps what it claimed and the earlier one gives up the overlap, so no
	// byte is deleted twice.
	for i := 0; i+1 < len(edits); i++ {
		if end := edits[i].off + edits[i].del; end > edits[i+1].off {
			edits[i].del = edits[i+1].off - edits[i].off
			if edits[i].del < 0 {
				edits[i].del = 0
			}
		}
	}

	changed := false
	for _, edit := range edits {
		if edit.del > 0 || len(edit.ins) > 0 {
			changed = true
			break
		}
	}
	if !changed {
		return false
	}

	ev.noteBufferEdit()
	ev.saveUndo(op)
	ev.lastOp = op
	ev.modified = true

	first := edits[0].off
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		if edit.del > 0 {
			ev.pt.Delete(edit.off, edit.del)
			ev.li.UpdateAfterDelete(edit.off, edit.del)
		}
		if len(edit.ins) > 0 {
			ev.pt.Insert(edit.off, edit.ins)
			ev.li.UpdateAfterInsert(edit.off, edit.ins)
		}
	}

	firstLine := ev.li.GetLineAtOffset(first)
	ev.invalidateStates(firstLine)
	ev.engine.InvalidateFrom(firstLine)

	// Each caret lands at its own site, shifted by everything the edits
	// before it added or removed.
	offsets := make([]int, len(edits))
	primary := 0
	shift := 0
	for i, edit := range edits {
		off := edit.off + shift
		if edit.after {
			off += len(edit.ins)
		}
		offsets[i] = off
		if edit.primary {
			primary = i
		}
		shift += len(edit.ins) - edit.del
	}
	ev.setCaretOffsets(offsets, primary)
	return true
}

// setCaretOffsets rebuilds the caret set from absolute offsets, keeping the
// one at index primary as the primary caret. Carets that met in the middle of
// a deletion arrive here as duplicates and leave as one.
func (ev *EditorView) setCaretOffsets(offsets []int, primary int) {
	if len(offsets) == 0 {
		return
	}
	if primary < 0 || primary >= len(offsets) {
		primary = 0
	}
	size := ev.pt.Size()
	clamp := func(off int) int {
		if off < 0 {
			return 0
		}
		if off > size {
			return size
		}
		return off
	}

	// Every selection has just been replaced by what was typed or deleted,
	// so the carets come out of an edit as bare carets.
	ev.selActive = false
	ev.rectSelActive = false

	primaryOff := clamp(offsets[primary])
	ev.CursorLine = ev.li.GetLineAtOffset(primaryOff)
	ev.CursorPos = primaryOff - ev.li.GetLineOffset(ev.CursorLine)
	ev.CursorVirtualSpaces = 0

	extras := ev.extraCursors[:0]
	for i, off := range offsets {
		if i == primary {
			continue
		}
		extras = append(extras, extraCaret{off: clamp(off)})
	}
	ev.extraCursors = extras
	ev.normalizeExtraCarets()
	// An edit puts every caret where its own change left it, so each one
	// takes the column it landed on.
	for i := range ev.extraCursors {
		ev.extraCursors[i].desiredCol = ev.visualColAt(ev.extraCursors[i].off)
	}

	ev.updateDesiredVisualCol()
	ev.ensureCursorVisible()
}

// processMultiCursorKey handles the keys that act through every caret and
// reports whether it took the key. Anything it declines collapses the set and
// is then handled the ordinary single-caret way.
func (ev *EditorView) processMultiCursorKey(e *vtinput.InputEvent) bool {
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	shift := e.ControlKeyState&vtinput.ShiftPressed != 0

	// A suggestion is a single caret's idea of what comes next, so it has no
	// place in a multi-caret edit.
	ev.acMatches = nil

	switch e.VirtualKeyCode {
	case vtinput.VK_LEFT, vtinput.VK_RIGHT:
		// Ctrl jumps by words, which is still the primary caret's alone.
		if ctrl || alt {
			return false
		}
		delta := 1
		if e.VirtualKeyCode == vtinput.VK_LEFT {
			delta = -1
		}
		return ev.multiMoveHorizontal(delta, shift)
	case vtinput.VK_UP, vtinput.VK_DOWN:
		// Ctrl+Up and Ctrl+Down scroll the text under the carets and leave
		// them where they are; multiCursorKeepsSet lets that through.
		if ctrl || alt {
			return false
		}
		delta := 1
		if e.VirtualKeyCode == vtinput.VK_UP {
			delta = -1
		}
		return ev.multiMoveVertical(delta, shift)
	case vtinput.VK_HOME, vtinput.VK_END:
		// Ctrl+Home and Ctrl+End go to one end of the file, which is one
		// place and therefore one caret.
		if ctrl || alt {
			return false
		}
		return ev.multiMoveLineEdge(e.VirtualKeyCode == vtinput.VK_END, shift)
	case vtinput.VK_BACK:
		if ctrl || alt {
			return false
		}
		return ev.multiDeleteBackward()
	case vtinput.VK_DELETE:
		// Shift+Del is Cut and Ctrl+Del deletes a word: both are still
		// single-caret operations.
		if ctrl || alt || shift {
			return false
		}
		return ev.multiDeleteForward()
	case vtinput.VK_RETURN:
		if ctrl || alt {
			return false
		}
		return ev.multiInsertNewline()
	case vtinput.VK_TAB:
		// Shift+Tab unindents, which is not a per-caret insertion.
		if ctrl || alt || shift {
			return false
		}
		return ev.multiInsertTab()
	}

	if e.Char != 0 && e.Char >= 32 && !ctrl && !alt {
		return ev.multiInsertText([]byte(string(e.Char)))
	}
	return false
}

// multiInsertText types the same bytes at every caret.
func (ev *EditorView) multiInsertText(data []byte) bool {
	return ev.applyCaretEdits(ev.buildCaretEdits(func(c caretSpan) caretEdit {
		edit := caretEdit{off: c.off, ins: data, after: true}
		switch {
		case c.hasSel():
			// Typing over a selection replaces it, at every caret that
			// has one.
			edit.off, edit.del = c.selStart, c.selEnd-c.selStart
		case ev.overtype:
			edit.del = ev.overtypeWidthAt(c.off)
		}
		return edit
	}), opTyping)
}

// multiInsertNewline splits the line at every caret, each one taking the
// indentation of the line it was on when auto-indent is on.
func (ev *EditorView) multiInsertNewline() bool {
	return ev.applyCaretEdits(ev.buildCaretEdits(func(c caretSpan) caretEdit {
		data := []byte("\n")
		if ev.AutoIndent {
			data = append(data, ev.lineIndentAt(c.off)...)
		}
		edit := caretEdit{off: c.off, ins: data, after: true}
		if c.hasSel() {
			edit.off, edit.del = c.selStart, c.selEnd-c.selStart
		}
		return edit
	}), opOther)
}

// multiInsertTab inserts a tab at every caret, expanded to the next tab stop
// of that caret's own column when tabs are expanded to spaces.
func (ev *EditorView) multiInsertTab() bool {
	tabSize := ev.TabSize
	if tabSize <= 0 {
		tabSize = 8
	}
	return ev.applyCaretEdits(ev.buildCaretEdits(func(c caretSpan) caretEdit {
		off := c.off
		if c.hasSel() {
			off = c.selStart
		}
		data := []byte("\t")
		if ev.ExpandTabs > 0 {
			_, vCol := ev.engine.LogicalToVisual(off)
			data = []byte(strings.Repeat(" ", tabSize-(vCol%tabSize)))
		}
		edit := caretEdit{off: off, ins: data, after: true}
		if c.hasSel() {
			edit.del = c.selEnd - c.selStart
		}
		return edit
	}), opTyping)
}

// multiDeleteBackward is Backspace at every caret. A caret at the very start
// of the buffer has nothing to delete and simply stays where it is.
func (ev *EditorView) multiDeleteBackward() bool {
	return ev.applyCaretEdits(ev.buildCaretEdits(func(c caretSpan) caretEdit {
		// With a selection, Backspace removes exactly that and no more.
		if c.hasSel() {
			return caretEdit{off: c.selStart, del: c.selEnd - c.selStart}
		}
		off := c.off
		if off <= 0 {
			return caretEdit{off: off}
		}
		line := ev.li.GetLineAtOffset(off)
		lineStart := ev.li.GetLineOffset(line)
		start := off - 1
		if off == lineStart {
			// Joining with the line above takes the whole terminator, so a
			// CRLF file does not keep a stray carriage return.
			if off >= 2 {
				if prefix, err := ev.pt.GetRange(off-2, 2); err == nil &&
					len(prefix) == 2 && prefix[0] == '\r' && prefix[1] == '\n' {
					start = off - 2
				}
			}
		} else {
			start = lineStart + ev.previousDeletionBoundaryInLine(lineStart, off-lineStart)
		}
		if start < 0 || start >= off {
			return caretEdit{off: off}
		}
		return caretEdit{off: start, del: off - start}
	}), opOther)
}

// multiDeleteForward is Del at every caret, following the single-caret rule:
// inside a line it removes one grapheme, at the end of one it removes the
// single byte that starts the line break.
func (ev *EditorView) multiDeleteForward() bool {
	size := ev.pt.Size()
	return ev.applyCaretEdits(ev.buildCaretEdits(func(c caretSpan) caretEdit {
		if c.hasSel() {
			return caretEdit{off: c.selStart, del: c.selEnd - c.selStart}
		}
		off := c.off
		if off >= size {
			return caretEdit{off: off}
		}
		line := ev.li.GetLineAtOffset(off)
		lineStart := ev.li.GetLineOffset(line)
		lineLen := ev.getLineLength(line)
		end := off + 1
		if pos := off - lineStart; pos < lineLen {
			end = lineStart + ev.nextGraphemeBoundaryInLine(lineStart, lineLen, pos)
		}
		if end <= off {
			return caretEdit{off: off}
		}
		return caretEdit{off: off, del: end - off}
	}), opOther)
}

// overtypeWidthAt is how much the character under a caret takes, for overtype
// mode. At the end of a line there is nothing to type over: overtype never
// eats the line break.
func (ev *EditorView) overtypeWidthAt(off int) int {
	line := ev.li.GetLineAtOffset(off)
	lineStart := ev.li.GetLineOffset(line)
	lineLen := ev.getLineLength(line)
	pos := off - lineStart
	if pos < 0 || pos >= lineLen {
		return 0
	}
	next := ev.nextGraphemeBoundaryInLine(lineStart, lineLen, pos)
	if next <= pos {
		return 0
	}
	return next - pos
}

// lineIndentAt returns the leading whitespace of the line the offset is on,
// which is what a new line started there inherits.
func (ev *EditorView) lineIndentAt(off int) []byte {
	var indent []byte
	for _, r := range ev.getLogicalLineRunes(ev.li.GetLineAtOffset(off)) {
		if r != ' ' && r != '\t' {
			break
		}
		indent = append(indent, []byte(string(r))...)
	}
	return indent
}

// multiCursorKeepsSet reports the keys that leave the caret set alone because
// they move the viewport rather than the text or the carets. Everything else
// the multi-caret handler declines collapses the set on its way through.
func multiCursorKeepsSet(e *vtinput.InputEvent) bool {
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	shift := e.ControlKeyState&vtinput.ShiftPressed != 0
	if !ctrl || alt || shift {
		return false
	}
	return e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_DOWN
}

// moveCarets moves every caret through move, which is handed each caret's
// offset and the column it aims for and returns where it goes.
//
// keepDesired says whether the carets keep aiming for the column they had:
// moving between lines does, so a short line on the way past does not cost a
// caret its place, while moving along a line takes the column it lands on.
func (ev *EditorView) moveCarets(move func(off, desiredCol int) int, keepDesired, selecting bool) bool {
	if len(ev.extraCursors) == 0 {
		return false
	}
	ev.rectSelActive = false
	ev.CursorVirtualSpaces = 0

	// Holding Shift means each caret leaves an anchor behind the first time
	// it moves and drags its own selection from there; letting go of the
	// text means letting go of the selections.
	primaryOff := ev.caretOffset()
	if selecting {
		if !ev.selActive {
			ev.selActive = true
			ev.selAnchorOffset = primaryOff
		}
	} else {
		ev.selActive = false
	}

	primary := move(primaryOff, ev.DesiredVisualCol)
	extras := make([]extraCaret, 0, len(ev.extraCursors))
	for _, caret := range ev.extraCursors {
		off := move(caret.off, caret.desiredCol)
		desired := caret.desiredCol
		if !keepDesired {
			desired = ev.visualColAt(off)
		}
		moved := extraCaret{off: off, desiredCol: desired}
		if selecting {
			moved.anchor = caret.anchor
			moved.hasSel = caret.hasSel
			if !moved.hasSel {
				moved.anchor, moved.hasSel = caret.off, true
			}
			moved.hasSel = moved.anchor != off
		}
		extras = append(extras, moved)
	}

	ev.CursorLine = ev.li.GetLineAtOffset(primary)
	ev.CursorPos = primary - ev.li.GetLineOffset(ev.CursorLine)
	if !keepDesired {
		ev.updateDesiredVisualCol()
	}
	if ev.selActive && ev.selAnchorOffset == primary {
		ev.selActive = false
	}
	ev.extraCursors = extras
	ev.normalizeExtraCarets()
	ev.ensureCursorVisible()
	return true
}

// multiMoveHorizontal moves every caret one grapheme along the text, stepping
// between lines at the ends the way the single caret does.
func (ev *EditorView) multiMoveHorizontal(delta int, selecting bool) bool {
	return ev.moveCarets(func(off, _ int) int {
		if delta < 0 {
			return ev.offsetBeforeCaret(off)
		}
		return ev.offsetAfterCaret(off)
	}, false, selecting)
}

// multiMoveVertical moves every caret one visual row up or down. A caret
// already at the edge of the text stays where it is instead of dragging the
// rest of the set out of shape.
func (ev *EditorView) multiMoveVertical(delta int, selecting bool) bool {
	total := ev.engine.GetTotalVisualRows()
	return ev.moveCarets(func(off, desired int) int {
		vRow, _ := ev.engine.LogicalToVisual(off)
		target := vRow + delta
		if target < 0 || target >= total {
			return off
		}
		return ev.snapMouseOffsetToClusterBoundary(ev.engine.VisualToLogical(target, desired))
	}, true, selecting)
}

// multiMoveLineEdge sends every caret to the start or the end of its own line.
func (ev *EditorView) multiMoveLineEdge(toEnd, selecting bool) bool {
	return ev.moveCarets(func(off, _ int) int {
		line := ev.li.GetLineAtOffset(off)
		if toEnd {
			return ev.li.GetLineOffset(line) + ev.getLineLength(line)
		}
		return ev.li.GetLineOffset(line)
	}, false, selecting)
}

// offsetAfterCaret is one grapheme forward, stepping over the line break at
// the end of a line whether it is one byte or two.
func (ev *EditorView) offsetAfterCaret(off int) int {
	size := ev.pt.Size()
	if off >= size {
		return size
	}
	line := ev.li.GetLineAtOffset(off)
	lineStart := ev.li.GetLineOffset(line)
	lineLen := ev.getLineLength(line)
	if pos := off - lineStart; pos < lineLen {
		return lineStart + ev.nextGraphemeBoundaryInLine(lineStart, lineLen, pos)
	}
	if line+1 < ev.li.LineCount() {
		return ev.li.GetLineOffset(line + 1)
	}
	return size
}

// offsetBeforeCaret is one grapheme back, landing at the end of the previous
// line rather than inside its line break.
func (ev *EditorView) offsetBeforeCaret(off int) int {
	if off <= 0 {
		return 0
	}
	line := ev.li.GetLineAtOffset(off)
	lineStart := ev.li.GetLineOffset(line)
	if off > lineStart {
		return lineStart + ev.previousGraphemeBoundaryInLine(lineStart, off-lineStart)
	}
	if line <= 0 {
		return 0
	}
	return ev.li.GetLineOffset(line-1) + ev.getLineLength(line-1)
}

// visualColAt is the column an offset is painted at.
func (ev *EditorView) visualColAt(off int) int {
	_, col := ev.engine.LogicalToVisual(off)
	return col
}

// sortExtraCarets puts the set back in offset order.
func (ev *EditorView) sortExtraCarets() {
	sort.Slice(ev.extraCursors, func(i, j int) bool {
		return ev.extraCursors[i].off < ev.extraCursors[j].off
	})
}

// normalizeExtraCarets sorts the set and drops the carets that have met: two
// that moved onto the same offset, or one that has arrived where the primary
// caret already is, are one caret now.
func (ev *EditorView) normalizeExtraCarets() {
	ev.sortExtraCarets()
	primary := ev.caretOffset()
	kept := ev.extraCursors[:0]
	for _, caret := range ev.extraCursors {
		if caret.off == primary {
			// The primary caret keeps the ground; if the one that walked
			// into it was selecting, the selection carries on from the
			// anchor that is farther away.
			if caret.hasSel {
				if !ev.selActive || farther(primary, ev.selAnchorOffset, caret.anchor) {
					ev.selAnchorOffset = caret.anchor
				}
				ev.selActive = true
			}
			continue
		}
		if n := len(kept); n > 0 && kept[n-1].off == caret.off {
			if caret.hasSel && (!kept[n-1].hasSel || farther(caret.off, kept[n-1].anchor, caret.anchor)) {
				kept[n-1].anchor, kept[n-1].hasSel = caret.anchor, true
			}
			continue
		}
		kept = append(kept, caret)
	}
	ev.extraCursors = kept
	if ev.selActive && ev.selAnchorOffset == primary {
		ev.selActive = false
	}
}

// farther reports whether b is the anchor that keeps more text selected from
// off, which is the one a merge has to keep.
func farther(off, a, b int) bool {
	da, db := off-a, off-b
	if da < 0 {
		da = -da
	}
	if db < 0 {
		db = -db
	}
	return db > da
}

// Putting carets on the copies of what is selected.
//
// The search is a plain byte scan over the piece table, done in chunks that
// overlap by the length of the pattern so a copy lying across a chunk boundary
// is still found. Nothing is indexed and nothing is cached: the text can be
// larger than memory, and the answer is only needed one keystroke at a time.

const (
	// editorCaretSearchChunk is how much text is pulled out of the piece
	// table at a time while scanning.
	editorCaretSearchChunk = 1 << 16
	// editorCaretSearchMax is the longest selection worth looking for. Past
	// this it is a passage, not a term.
	editorCaretSearchMax = 4096
	// editorMaxOccurrenceCarets caps how many carets one keystroke may
	// produce, so that selecting a space in a large file cannot fill memory
	// with carets nobody asked for.
	editorMaxOccurrenceCarets = 4096
)

// caretSearchNeedle is the primary caret's selection, which is the text the
// occurrence commands look for.
func (ev *EditorView) caretSearchNeedle() ([]byte, bool) {
	if !ev.selActive || ev.rectSelActive {
		return nil, false
	}
	start, end := ev.getSelectionRange()
	if end <= start || end-start > editorCaretSearchMax {
		return nil, false
	}
	needle, err := ev.pt.GetRange(start, end-start)
	if err != nil || len(needle) != end-start {
		return nil, false
	}
	return needle, true
}

// findBytesIn returns the offset of the first copy of needle that lies wholly
// within [from, limit), or -1.
func (ev *EditorView) findBytesIn(needle []byte, from, limit int) int {
	if len(needle) == 0 {
		return -1
	}
	if from < 0 {
		from = 0
	}
	if size := ev.pt.Size(); limit > size {
		limit = size
	}
	for pos := from; pos+len(needle) <= limit; {
		end := pos + editorCaretSearchChunk
		if end > limit {
			end = limit
		}
		var err error
		ev.caretSearchBuf, err = ev.pt.AppendRange(ev.caretSearchBuf[:0], pos, end-pos)
		if err != nil {
			return -1
		}
		if idx := bytes.Index(ev.caretSearchBuf, needle); idx >= 0 {
			return pos + idx
		}
		if end >= limit {
			break
		}
		// Overlap by one less than the pattern, so a copy sitting across
		// the seam is found by the next round.
		pos = end - (len(needle) - 1)
	}
	return -1
}

// AddCursorAtNextOccurrence puts a caret on the next copy of the selected
// text, wrapping round the end of the file, and makes it the primary caret so
// that the view follows and the key can be pressed again to walk on.
//
// With nothing selected it selects the word under the caret instead, which is
// the first press of the same key in every editor that has this.
func (ev *EditorView) AddCursorAtNextOccurrence() {
	needle, ok := ev.caretSearchNeedle()
	if !ok {
		ev.clearExtraCursors()
		ev.selectWordUnderCursor()
		vtui.FrameManager.Redraw()
		return
	}

	// Start after the last thing already selected, so repeated presses walk
	// forwards rather than finding the copies already taken.
	from := 0
	for _, span := range ev.caretSpans() {
		if span.selEnd > from {
			from = span.selEnd
		}
	}
	match := ev.findBytesIn(needle, from, ev.pt.Size())
	if match < 0 {
		match = ev.findBytesIn(needle, 0, from)
	}
	if match < 0 {
		return
	}
	for _, span := range ev.caretSpans() {
		if span.selStart == match {
			// Every copy already has a caret on it.
			return
		}
	}

	ev.pushPrimaryToExtras()
	ev.setPrimarySelection(match, match+len(needle))
	ev.normalizeExtraCarets()
	ev.ensureCursorVisible()
	vtui.FrameManager.Redraw()
}

// SelectAllOccurrences puts a caret on every copy of the selected text.
func (ev *EditorView) SelectAllOccurrences() {
	needle, ok := ev.caretSearchNeedle()
	if !ok {
		ev.clearExtraCursors()
		ev.selectWordUnderCursor()
		needle, ok = ev.caretSearchNeedle()
		if !ok {
			vtui.FrameManager.Redraw()
			return
		}
	}

	selStart, _ := ev.getSelectionRange()
	size := ev.pt.Size()
	matches := make([]int, 0, 16)
	for pos := 0; pos+len(needle) <= size && len(matches) < editorMaxOccurrenceCarets; {
		match := ev.findBytesIn(needle, pos, size)
		if match < 0 {
			break
		}
		matches = append(matches, match)
		pos = match + len(needle)
	}
	if len(matches) == 0 {
		return
	}

	// The caret that was already on one of the copies stays the primary one,
	// so the view does not jump away from what the user was looking at.
	primary := 0
	for i, match := range matches {
		if match == selStart {
			primary = i
			break
		}
	}

	ev.extraCursors = ev.extraCursors[:0]
	for i, match := range matches {
		if i == primary {
			continue
		}
		ev.extraCursors = append(ev.extraCursors, extraCaret{
			off:        match + len(needle),
			desiredCol: ev.visualColAt(match + len(needle)),
			anchor:     match,
			hasSel:     true,
		})
	}
	ev.setPrimarySelection(matches[primary], matches[primary]+len(needle))
	ev.normalizeExtraCarets()
	ev.ensureCursorVisible()
	vtui.FrameManager.Redraw()
}

// pushPrimaryToExtras keeps the primary caret, and whatever it has selected,
// as one of the secondary carets.
func (ev *EditorView) pushPrimaryToExtras() {
	off := ev.caretOffset()
	caret := extraCaret{off: off, desiredCol: ev.visualColAt(off)}
	if ev.selActive && !ev.rectSelActive && ev.selAnchorOffset != off {
		caret.anchor, caret.hasSel = ev.selAnchorOffset, true
	}
	ev.extraCursors = append(ev.extraCursors, caret)
}

// setPrimarySelection moves the primary caret to the end of [start, end) and
// selects that range.
func (ev *EditorView) setPrimarySelection(start, end int) {
	ev.CursorLine = ev.li.GetLineAtOffset(end)
	ev.CursorPos = end - ev.li.GetLineOffset(ev.CursorLine)
	ev.CursorVirtualSpaces = 0
	ev.rectSelActive = false
	ev.selAnchorOffset = start
	ev.selActive = end != start
	ev.updateDesiredVisualCol()
}
