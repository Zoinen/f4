package editor

// The Markdown split preview (f4#1625, step 5): the editor keeps the left
// half of the workspace and a formatted view of the text it holds fills the
// right half, rebuilt shortly after typing stops and scrolled to where the
// cursor is. It is the same vtui Markdown viewer the F3 view and the
// Shift+F3 snapshot use; nothing is parsed here.

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/unxed/f4/internal/mdmath"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// mdSplitDelay is how long the text must sit still before the preview is
// rebuilt: typing does not reparse the document per keystroke.
const mdSplitDelay = 300 * time.Millisecond

// mdSplitMaxSize is the largest text the split preview formats, the same
// limit as the F3 view (internal/app/markdown_view.go).
const mdSplitMaxSize = 4 << 20

type mdSplitState struct {
	view  *vtui.HelpView
	timer *time.Timer
	// prepLine[i] is the line of the editor's text that line i of the text
	// the preview was parsed from (formulas rewritten) came from; topicLine[i]
	// is the line of that text that row i of the parsed topic came from.
	prepLine  []int
	topicLine []int
}

// MarkdownSplitActive reports whether the split preview is on.
func (ev *EditorView) MarkdownSplitActive() bool { return ev.mdSplit != nil }

// ToggleMarkdownSplit switches the preview beside the editor on or off.
func (ev *EditorView) ToggleMarkdownSplit() {
	if ev.mdSplit != nil {
		if ev.mdSplit.timer != nil {
			ev.mdSplit.timer.Stop()
		}
		ev.mdSplit = nil
	} else {
		ev.mdSplit = &mdSplitState{}
		ev.refreshMarkdownSplit()
	}
	if ev.lastW > 0 && ev.lastH > 0 {
		ev.ResizeConsole(ev.lastW, ev.lastH)
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

// layoutMarkdownSplit narrows the editor to the left half and puts the
// preview in the right one. It runs at the end of ResizeConsole.
func (ev *EditorView) layoutMarkdownSplit(w int) {
	if ev.mdSplit == nil || w < 20 {
		return
	}
	x1, y1, x2, y2 := ev.GetPosition()
	mid := w / 2
	ev.SetPosition(x1, y1, mid-1, y2)
	if ev.mdSplit.view != nil {
		ev.mdSplit.view.SetPosition(mid, y1, x2, y2)
	}
}

// refreshMarkdownSplit rebuilds the preview from the buffer as it is now and
// scrolls it to the cursor's relative place in the text.
func (ev *EditorView) refreshMarkdownSplit() {
	st := ev.mdSplit
	if st == nil || ev.Pt == nil {
		return
	}
	text := ev.GetText()
	if len(text) > mdSplitMaxSize {
		st.view = nil
		return
	}
	name := filepath.Base(ev.FilePath)
	prepared, prepLine := mdmath.PrepareMapped(strings.ReplaceAll(text, "\r\n", "\n"))
	topic, topicLine := vtui.ParseMarkdownTopicMap(name, prepared)
	engine := vtui.NewHelpEngine(nil)
	engine.AddTopic(topic)
	view := vtui.NewHelpView(engine, name)
	view.Modal = false
	view.ShowClose = false
	view.SetTitle(" " + name + " ")
	if ev.lastW > 0 {
		_, y1, _, y2 := ev.GetPosition()
		view.SetPosition(ev.lastW/2, y1, ev.lastW-1, y2)
	}
	st.view, st.prepLine, st.topicLine = view, prepLine, topicLine
	ev.syncMarkdownSplitScroll()
}

// previewRowOfLine is the row of the preview (as it reads in the window,
// long lines broken) that shows editor line: the first row made from the
// text at or after that line.
func (st *mdSplitState) previewRowOfLine(line int) int {
	prep := 0
	for i, orig := range st.prepLine {
		if orig > line {
			break
		}
		prep = i
	}
	topicRow := len(st.topicLine) - 1
	for i, src := range st.topicLine {
		if src >= prep {
			topicRow = i
			break
		}
	}
	lines := st.view.CurrentTopic().Lines
	for row := range lines {
		if src, ok := st.view.SourceRow(row); ok && src >= topicRow {
			// The blank line that separates a block from the one before it
			// belongs to the block; the text is on the line after it.
			for row+1 < len(lines) && lines[row] == "" {
				row++
			}
			return row
		}
	}
	return max(len(lines)-1, 0)
}

// lineOfPreviewRow is the editor line the preview row was made from.
func (st *mdSplitState) lineOfPreviewRow(row int) int {
	topicRow, ok := st.view.SourceRow(row)
	if !ok || topicRow >= len(st.topicLine) {
		return 0
	}
	prep := st.topicLine[topicRow]
	if prep >= len(st.prepLine) {
		return st.prepLine[len(st.prepLine)-1]
	}
	return st.prepLine[prep]
}

// syncMarkdownSplitScroll scrolls the preview to the rows made from the
// cursor's line, using the map the Markdown parser keeps from rows back to
// the text.
func (ev *EditorView) syncMarkdownSplitScroll() {
	st := ev.mdSplit
	if st == nil || st.view == nil || st.view.CurrentTopic() == nil || len(st.prepLine) == 0 {
		return
	}
	st.view.SetScrollTop(st.previewRowOfLine(ev.CursorLine))
}

// scheduleMarkdownSplit restarts the pause timer after a key, so the preview
// follows the text once typing stops.
func (ev *EditorView) scheduleMarkdownSplit() {
	st := ev.mdSplit
	if st == nil {
		return
	}
	if st.timer != nil {
		st.timer.Stop()
	}
	// Read on the calling goroutine, like scheduleIndexResume does.
	uiFrames := vtui.FrameManager
	st.timer = time.AfterFunc(mdSplitDelay, func() {
		uiFrames.PostTask(func() {
			if ev.IsDone() || ev.mdSplit != st {
				return
			}
			ev.refreshMarkdownSplit()
			uiFrames.Redraw()
		})
	})
}

func (ev *EditorView) showMarkdownSplit(scr *vtui.ScreenBuf) {
	if ev.mdSplit != nil && ev.mdSplit.view != nil {
		ev.mdSplit.view.Show(scr)
	}
}

// markdownSplitMouse hands a mouse event on the preview half to the preview.
// A left click on the preview's text moves the editor's cursor to the matching
// place (markdownSplitClick); everything else - the wheel, a row with a link,
// the scroll bar - is the preview's own. A click has to be answered here first:
// the window under the preview would take it as the start of a drag.
func (ev *EditorView) markdownSplitMouse(e *vtinput.InputEvent) bool {
	if ev.mdSplit == nil || ev.mdSplit.view == nil || e.Type != vtinput.MouseEventType {
		return false
	}
	x1, y1, x2, y2 := ev.mdSplit.view.GetPosition()
	mx, my := int(e.MouseX), int(e.MouseY)
	if mx < x1 || mx > x2 || my < y1 || my > y2 {
		return false
	}
	if ev.markdownSplitClick(e) {
		return true
	}
	return ev.mdSplit.view.ProcessMouse(e)
}

// markdownSplitClick puts the cursor where the clicked preview row sits in the
// text, by the map from the preview's rows back to its lines. The preview
// itself stays where it is, so the text does not move under the pointer.
func (ev *EditorView) markdownSplitClick(e *vtinput.InputEvent) bool {
	st := ev.mdSplit
	if st == nil || st.view == nil || ev.Li == nil || !vtui.IsMousePress(e) ||
		e.ButtonState&vtinput.FromLeft1stButtonPressed == 0 {
		return false
	}
	topic := st.view.CurrentTopic()
	if topic == nil || len(topic.Lines) == 0 {
		return false
	}
	tx1, ty1, tx2, ty2 := st.view.TextArea()
	mx, my := int(e.MouseX), int(e.MouseY)
	if mx < tx1 || mx > tx2 || my < ty1 || my > ty2 {
		return false
	}
	scroll := st.view.ScrollTop()
	row := my - ty1
	if row >= topic.StickyRows {
		row += scroll
	}
	row = max(0, min(row, len(topic.Lines)-1))
	for _, link := range topic.Links {
		if link.Line == row {
			return false // a row with a link: the preview follows or selects it
		}
	}
	if len(st.prepLine) == 0 {
		return false
	}
	ev.gotoLinePosition(st.lineOfPreviewRow(row)+1, 1)
	return true
}
