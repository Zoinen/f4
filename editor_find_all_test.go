package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// --- findAllMatchSpans (pure) ---

func TestFindAllMatchSpans_Plain(t *testing.T) {
	spans, err := findAllMatchSpans(context.Background(), []byte("unit one\nno match\nUnited"), "unit", true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 || spans[0].Off != 0 || spans[0].Len != 4 {
		t.Errorf("expected one span at 0 len 4, got %v", spans)
	}
}

func TestFindAllMatchSpans_CaseInsensitive(t *testing.T) {
	spans, err := findAllMatchSpans(context.Background(), []byte("Unit one\nunited"), "unit", false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || spans[0].Off != 0 || spans[1].Off != 9 {
		t.Errorf("expected spans at 0 and 9, got %v", spans)
	}
}

func TestFindAllMatchSpans_CaseInsensitiveUnicodeOffsets(t *testing.T) {
	// İ (U+0130) grows from 2 to 3 bytes under ToLower, so a lowered-copy
	// search would report offsets shifted past every such character. The
	// spans must index the original buffer.
	data := []byte("İİİ unit")
	spans, err := findAllMatchSpans(context.Background(), data, "unit", false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	wantOff := len("İİİ ")
	if len(spans) != 1 || spans[0].Off != wantOff || spans[0].Len != 4 {
		t.Errorf("expected one span at %d len 4, got %v", wantOff, spans)
	}
}

func TestFindAllMatchSpans_CaseInsensitiveNonASCIIPattern(t *testing.T) {
	// A non-ASCII pattern takes the (?i) regex route; folding must still
	// find both cases and report offsets into the original buffer.
	data := []byte("Ünit one\nünited")
	spans, err := findAllMatchSpans(context.Background(), data, "ünit", false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	second := len("Ünit one\n")
	if len(spans) != 2 || spans[0].Off != 0 || spans[1].Off != second {
		t.Errorf("expected spans at 0 and %d, got %v", second, spans)
	}
}

func TestFindAllMatchSpans_FoldedMatchLength(t *testing.T) {
	// K (U+212A, 3 bytes) case-folds to "k": the span must cover the
	// folded character's real byte width, not the pattern's 1 byte.
	data := []byte("\u212A unit k")
	spans, err := findAllMatchSpans(context.Background(), data, "k", false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	second := len("\u212A unit ")
	if len(spans) != 2 || spans[0].Off != 0 || spans[0].Len != 3 ||
		spans[1].Off != second || spans[1].Len != 1 {
		t.Errorf("expected spans {0,3} and {%d,1}, got %v", second, spans)
	}
}

func TestFindAllMatchSpans_Regex(t *testing.T) {
	spans, err := findAllMatchSpans(context.Background(), []byte("ab12cd345"), `\d+`, true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || spans[0].Off != 2 || spans[0].Len != 2 || spans[1].Off != 6 || spans[1].Len != 3 {
		t.Errorf("expected [2,2] and [6,3], got %v", spans)
	}
}

func TestFindAllMatchSpans_RegexError(t *testing.T) {
	_, err := findAllMatchSpans(context.Background(), []byte("abc"), `(`, true, true, false)
	if err == nil {
		t.Error("expected compile error for '('")
	}
}

func TestFindAllMatchSpans_WholeWord(t *testing.T) {
	spans, err := findAllMatchSpans(context.Background(), []byte("unit units unit"), "unit", true, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || spans[0].Off != 0 || spans[1].Off != 11 {
		t.Errorf("expected whole-word spans at 0 and 11, got %v", spans)
	}
}

func TestFindAllMatchSpans_MultiplePerLine(t *testing.T) {
	spans, err := findAllMatchSpans(context.Background(), []byte("a unit b unit\nunit"), "unit", true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 3 {
		t.Errorf("expected 3 spans, got %v", spans)
	}
}

func TestFindAllMatchSpans_ZeroLengthRegex(t *testing.T) {
	done := make(chan struct{})
	var spans []matchSpan
	var err error
	go func() {
		spans, err = findAllMatchSpans(context.Background(), []byte("abc\nabc"), "x*", true, true, false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("zero-length regex hung")
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 0 {
		t.Errorf("zero-length matches must be dropped, got %v", spans)
	}
}

func TestFindAllMatchSpans_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	spans, err := findAllMatchSpans(ctx, []byte("abc abc abc"), "abc", true, false, false)
	if err == nil {
		t.Error("expected the canceled context to surface as an error")
	}
	if len(spans) != 0 {
		t.Errorf("canceled scan should return no spans, got %v", spans)
	}
}

// --- menu behavior ---

// pumpFindAll executes queued UI tasks until cond is satisfied or the
// timeout hits.
func pumpFindAll(t *testing.T, cond func() bool) {
	t.Helper()
	timeout := time.After(1 * time.Second)
	for {
		if cond() {
			return
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("timed out waiting for find-all UI")
		}
	}
}

// pumpFindAllFor drains queued UI tasks for the given duration, failing the
// test as soon as bad() reports a forbidden state.
func pumpFindAllFor(t *testing.T, d time.Duration, bad func() (string, bool)) {
	t.Helper()
	deadline := time.After(d)
	for {
		if msg, isBad := bad(); isBad {
			t.Fatal(msg)
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			return
		}
	}
}

// pumpUntilSearchDialog drains UI tasks until the " Searching... " progress
// dialog is the top frame and returns it. Pumping (rather than assuming the
// next queued task is the search setup) keeps the test immune to leftover
// tasks queued by earlier tests: vtui's task queue outlives FrameManager.Init.
func pumpUntilSearchDialog(t *testing.T) *vtui.Window {
	t.Helper()
	var dlg *vtui.Window
	pumpFindAll(t, func() bool {
		w, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
		if ok && w.GetTitle() == Msg("Search.Searching") {
			dlg = w
		}
		return dlg != nil
	})
	return dlg
}

// openFindAllMenu runs FindAll and returns the occurrences frame once it
// reaches the top of the frame stack.
func openFindAllMenu(t *testing.T, ev *EditorView, pattern string, caseSensitive, useRegex, wholeWord bool) *findAllFrame {
	t.Helper()
	ev.FindAll(pattern, caseSensitive, useRegex, wholeWord)
	var frame *findAllFrame
	pumpFindAll(t, func() bool {
		f, ok := vtui.FrameManager.GetTopFrame().(*findAllFrame)
		if ok {
			frame = f
		}
		return ok
	})
	return frame
}

func newFindAllEditorSized(t *testing.T, content string, w, h int) *EditorView {
	t.Helper()
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(w, h)
	vtui.FrameManager.Init(scr)
	pt := piecetable.New([]byte(content))
	ev := NewEditorView(pt, nil, "test.txt")
	t.Cleanup(func() { ev.Close() })
	ev.SetPosition(0, 0, w, h-1)
	return ev
}

func newFindAllEditor(t *testing.T, content string) *EditorView {
	t.Helper()
	return newFindAllEditorSized(t, content, 80, 25)
}

func keyEvent(vk uint16, ctrlState vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vk,
		ControlKeyState: ctrlState,
	}
}

func TestEditorFindAll_MenuContents(t *testing.T) {
	ev := newFindAllEditor(t, "Unit 15 The Avenue\nno match here\nUnited Kingdom")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	if got := frame.GetItemCount(); got != 2 {
		t.Fatalf("expected 2 items, got %d", got)
	}
	if !strings.Contains(frame.GetTitle(), "2") {
		t.Errorf("title should mention occurrence count, got %q", frame.GetTitle())
	}
	if frame.Items[0].AccentPrefix != "1│1│ " {
		t.Errorf("unexpected accent prefix %q", frame.Items[0].AccentPrefix)
	}
	if frame.Items[1].AccentPrefix != "3│1│ " {
		t.Errorf("unexpected accent prefix %q", frame.Items[1].AccentPrefix)
	}
	if frame.Items[0].Text != "Unit 15 The Avenue" {
		t.Errorf("unexpected item text %q", frame.Items[0].Text)
	}
}

func TestEditorFindAll_MenuJump(t *testing.T) {
	ev := newFindAllEditor(t, "Unit 15 The Avenue\nno match here\nUnited Kingdom")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	frame.SetSelectPos(1)
	frame.ProcessKey(keyEvent(vtinput.VK_RETURN, 0))
	pumpFindAll(t, func() bool { return ev.selActive })

	wantOff := len("Unit 15 The Avenue\nno match here\n")
	if ev.selAnchorOffset != wantOff {
		t.Errorf("expected selection anchor %d, got %d", wantOff, ev.selAnchorOffset)
	}
	cursorOff := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	if cursorOff != wantOff+4 {
		t.Errorf("expected cursor at %d, got %d", wantOff+4, cursorOff)
	}
	if !frame.IsDone() {
		t.Error("Enter should close the occurrences menu")
	}
}

func TestEditorFindAll_CtrlEnterKeepsMenuOpen(t *testing.T) {
	ev := newFindAllEditor(t, "Unit 15 The Avenue\nno match here\nUnited Kingdom")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	frame.SetSelectPos(1)
	frame.ProcessKey(keyEvent(vtinput.VK_RETURN, vtinput.LeftCtrlPressed))

	if !ev.selActive {
		t.Error("Ctrl+Enter should select the occurrence")
	}
	wantOff := len("Unit 15 The Avenue\nno match here\n")
	if ev.selAnchorOffset != wantOff {
		t.Errorf("expected selection anchor %d, got %d", wantOff, ev.selAnchorOffset)
	}
	if frame.IsDone() {
		t.Error("Ctrl+Enter must keep the menu open")
	}
	if vtui.FrameManager.GetTopFrame() != vtui.Frame(frame) {
		t.Error("occurrences menu should still be the top frame")
	}
}

func TestEditorFindAll_CtrlUpDownScrollsEditor(t *testing.T) {
	content := strings.Repeat("filler\n", 60) + "needle"
	ev := newFindAllEditor(t, content)
	frame := openFindAllMenu(t, ev, "needle", false, false, false)

	frame.ProcessKey(keyEvent(vtinput.VK_DOWN, vtinput.LeftCtrlPressed))
	if ev.ScrollTopRow != 1 {
		t.Errorf("Ctrl+Down should scroll editor to row 1, got %d", ev.ScrollTopRow)
	}
	frame.ProcessKey(keyEvent(vtinput.VK_UP, vtinput.LeftCtrlPressed))
	frame.ProcessKey(keyEvent(vtinput.VK_UP, vtinput.LeftCtrlPressed))
	if ev.ScrollTopRow != 0 {
		t.Errorf("Ctrl+Up should clamp at 0, got %d", ev.ScrollTopRow)
	}
	if frame.IsDone() {
		t.Error("Ctrl+Up/Down must not close the menu")
	}
	// Menu selection must not have moved.
	if frame.SelectPos != 0 {
		t.Errorf("menu selection moved to %d", frame.SelectPos)
	}
}

func TestEditorFindAll_F4OpensNewEditor(t *testing.T) {
	ev := newFindAllEditor(t, "Unit 15 The Avenue\nno match here\nUnited Kingdom\nunit again")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	frame.ProcessKey(keyEvent(vtinput.VK_F4, 0))
	var newEd *EditorView
	pumpFindAll(t, func() bool {
		ed, ok := vtui.FrameManager.GetTopFrame().(*EditorView)
		if ok && ed != ev {
			newEd = ed
		}
		return newEd != nil
	})
	defer newEd.Close()

	data, err := newEd.pt.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	want := "1: Unit 15 The Avenue\n3: United Kingdom\n4: unit again\n"
	if string(data) != want {
		t.Errorf("new editor content mismatch:\n got %q\nwant %q", string(data), want)
	}
	if !strings.Contains(newEd.DisplayTitle, "unit") {
		t.Errorf("new editor title should mention the pattern, got %q", newEd.DisplayTitle)
	}
	if !frame.IsDone() {
		t.Error("F4 should close the occurrences menu")
	}
}

func TestEditorFindAll_F5Zoom(t *testing.T) {
	ev := newFindAllEditor(t, "Unit 15 The Avenue\nno match here\nUnited Kingdom")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	x1, y1, x2, y2 := frame.VMenu.GetPosition()
	frame.ProcessKey(keyEvent(vtinput.VK_F5, 0))
	zx1, zy1, zx2, zy2 := frame.VMenu.GetPosition()
	if zx2-zx1 <= x2-x1 || zy2-zy1 <= y2-y1 {
		t.Errorf("F5 should zoom the menu: was (%d,%d)-(%d,%d), got (%d,%d)-(%d,%d)",
			x1, y1, x2, y2, zx1, zy1, zx2, zy2)
	}
	frame.ProcessKey(keyEvent(vtinput.VK_F5, 0))
	rx1, ry1, rx2, ry2 := frame.VMenu.GetPosition()
	if rx1 != x1 || ry1 != y1 || rx2 != x2 || ry2 != y2 {
		t.Error("second F5 should restore the original size")
	}
}

func TestEditorFindAll_AmpersandEscaping(t *testing.T) {
	ev := newFindAllEditor(t, "a & unit b")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	if !strings.Contains(frame.Items[0].Text, "&&") {
		t.Errorf("literal '&' must be doubled for vtui, got %q", frame.Items[0].Text)
	}
}

func TestEditorFindAll_TabsBecomeSpaces(t *testing.T) {
	ev := newFindAllEditor(t, "a\tunit\tb")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	if frame.Items[0].Text != "a unit b" {
		t.Errorf("tabs should render as single spaces, got %q", frame.Items[0].Text)
	}
}

func TestEditorFindAll_NotFound(t *testing.T) {
	ev := newFindAllEditor(t, "nothing to see")
	ev.FindAll("unit", false, false, false)

	pumpFindAll(t, func() bool {
		f := vtui.FrameManager.GetTopFrame()
		if _, ok := f.(*findAllFrame); ok {
			t.Fatal("no occurrences menu expected for a miss")
		}
		return f != nil && f.GetType() == vtui.TypeDialog
	})
	if ev.selActive {
		t.Error("a miss must not create a selection")
	}
}

// --- review-fix regressions ---

func TestRunSearchWithProgress_CloseCancels(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	done := make(chan struct{})
	dlg, _ := runSearchWithProgress("x", func(ctx *vtui.TaskContext, _ *vtui.Window) {
		<-ctx.Done()
		close(done)
	})

	// Closing the dialog (what Esc does) must cancel the worker even though
	// the Cancel button was never clicked.
	dlg.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("closing the progress dialog did not cancel the search task")
	}
}

func TestEditorFindAll_CancelSuppressesResults(t *testing.T) {
	ev := newFindAllEditor(t, "unit unit unit")
	ev.FindAll("unit", false, false, false)

	dlg := pumpUntilSearchDialog(t)
	dlg.Close() // Esc path: close without touching the Cancel button

	pumpFindAllFor(t, 300*time.Millisecond, func() (string, bool) {
		if _, isMenu := vtui.FrameManager.GetTopFrame().(*findAllFrame); isMenu {
			return "canceled Find All must not open the occurrences menu", true
		}
		return "", false
	})
}

func TestEditorFindAll_CancelSuppressesRegexError(t *testing.T) {
	ev := newFindAllEditor(t, "some text")
	ev.FindAll("(", false, true, false)

	dlg := pumpUntilSearchDialog(t)
	dlg.Close()

	pumpFindAllFor(t, 300*time.Millisecond, func() (string, bool) {
		if f := vtui.FrameManager.GetTopFrame(); f != nil && f.GetTitle() == " Error " {
			return "canceled Find All must not pop the regex error dialog", true
		}
		return "", false
	})
}

func TestEditorFindAll_EditSessionGuard(t *testing.T) {
	ev := newFindAllEditor(t, "unit unit unit")
	ev.FindAll("unit", false, false, false)

	pumpUntilSearchDialog(t)
	// Simulate an edit landing while the scan runs: results must be dropped.
	ev.editSession++

	pumpFindAllFor(t, 300*time.Millisecond, func() (string, bool) {
		if _, isMenu := vtui.FrameManager.GetTopFrame().(*findAllFrame); isMenu {
			return "stale Find All results must not open the occurrences menu", true
		}
		return "", false
	})
}

func TestEditorFindAll_HighlightMatchesDisplayBytes(t *testing.T) {
	// One line with a control character before the match and enough padding
	// to force the sanitize+truncate rebuild; one clean long line where the
	// rebuild keeps the bytes identical.
	content := "\x01needle" + strings.Repeat("x", 600) + "\nneedle" + strings.Repeat("y", 600)
	ev := newFindAllEditor(t, content)
	frame := openFindAllMenu(t, ev, "needle", false, false, false)

	if got := frame.GetItemCount(); got != 2 {
		t.Fatalf("expected 2 items, got %d", got)
	}
	// The invariant behind the highlight overpaint: whenever a row claims a
	// highlight, its byte range in the display string must hold the match.
	for i, r := range frame.rows {
		if r.cellWidth == 0 {
			continue
		}
		if got := frame.displays[i][r.byteStart:r.byteEnd]; got != "needle" {
			t.Errorf("row %d highlight bytes = %q, want \"needle\"", i, got)
		}
	}
	// The control-free line must keep its highlight.
	if frame.rows[1].cellWidth == 0 {
		t.Error("control-free long line should still highlight its match")
	}
}

func TestEditorFindAll_AccentWidthMeasured(t *testing.T) {
	ev := newFindAllEditor(t, "unit\n\n\n\n\n\n\n\n\n\nunit at line eleven")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	if want := vtui.StringWidth(frame.Items[0].AccentPrefix); frame.accentW != want {
		t.Errorf("accentW = %d, want measured width %d", frame.accentW, want)
	}
}

func TestEditorFindAll_ResizeConsole(t *testing.T) {
	ev := newFindAllEditor(t, "Unit 15 The Avenue\nno match here\nUnited Kingdom")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	assertWithin := func(w, h int, when string) {
		t.Helper()
		x1, y1, x2, y2 := frame.VMenu.GetPosition()
		if x1 < 0 || y1 < 0 || x2 >= w || y2 >= h || x1 > x2 || y1 > y2 {
			t.Errorf("%s: menu rect (%d,%d)-(%d,%d) outside %dx%d screen", when, x1, y1, x2, y2, w, h)
		}
	}

	frame.ResizeConsole(40, 12)
	assertWithin(40, 12, "after shrink")

	frame.ProcessKey(keyEvent(vtinput.VK_F5, 0)) // zoom on the small screen
	frame.ResizeConsole(30, 10)
	assertWithin(30, 10, "zoomed after shrink")

	frame.ProcessKey(keyEvent(vtinput.VK_F5, 0)) // un-zoom
	assertWithin(30, 10, "restored after shrink")
}

func TestEditorFindAll_NarrowTerminalKeepsText(t *testing.T) {
	ev := newFindAllEditorSized(t, "unit and more text here\nunit", 24, 10)
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	for i := range frame.Items {
		if frame.Items[i].Text == "" {
			t.Errorf("item %d text blanked out on a narrow terminal", i)
		}
	}
}
