package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	t.Cleanup(swapFrameManager(t))
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

// paintFindAll renders the frame and returns the text of each row inside the
// border. The list holds no items, so painting is the only place its contents
// exist; asserting on the screen is asserting on the list.
func paintFindAll(t *testing.T, frame *findAllFrame) []string {
	t.Helper()
	w, h := vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(w, h)
	frame.Show(scr)

	x1, y1, x2, y2 := frame.GetPosition()
	rows := make([]string, 0, max(y2-y1-1, 0))
	for y := y1 + 1; y < y2; y++ {
		rows = append(rows, strings.TrimRight(vtui.ScreenRow(scr, y, x1+2, x2-1), " "))
	}
	return rows
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
	// The list holds no menu items at all: rows are resolved as they are
	// painted, which is what keeps a million occurrences openable.
	if len(frame.Items) != 0 {
		t.Errorf("menu materialized %d items, want none", len(frame.Items))
	}
	if got := paintFindAll(t, frame); got[0] != "1│1│ Unit 15 The Avenue" || got[1] != "3│1│ United Kingdom" {
		t.Errorf("unexpected painted rows %q", got[:2])
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

	x1, y1, x2, y2 := frame.GetPosition()
	frame.ProcessKey(keyEvent(vtinput.VK_F5, 0))
	zx1, zy1, zx2, zy2 := frame.GetPosition()
	if zx2-zx1 <= x2-x1 || zy2-zy1 <= y2-y1 {
		t.Errorf("F5 should zoom the menu: was (%d,%d)-(%d,%d), got (%d,%d)-(%d,%d)",
			x1, y1, x2, y2, zx1, zy1, zx2, zy2)
	}
	frame.ProcessKey(keyEvent(vtinput.VK_F5, 0))
	rx1, ry1, rx2, ry2 := frame.GetPosition()
	if rx1 != x1 || ry1 != y1 || rx2 != x2 || ry2 != y2 {
		t.Error("second F5 should restore the original size")
	}
}

func TestEditorFindAll_AmpersandRendersLiterally(t *testing.T) {
	// Rows are painted as plain strings, not as vtui control text, so a '&'
	// in the file reaches the screen as itself and eats no following letter.
	ev := newFindAllEditor(t, "a & unit b")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	if got := paintFindAll(t, frame)[0]; !strings.HasSuffix(got, "a & unit b") {
		t.Errorf("literal '&' should survive to the screen, got %q", got)
	}
}

func TestEditorFindAll_TabsBecomeSpaces(t *testing.T) {
	ev := newFindAllEditor(t, "a\tunit\tb")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	if got := paintFindAll(t, frame)[0]; !strings.HasSuffix(got, "a unit b") {
		t.Errorf("tabs should render as single spaces, got %q", got)
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
		return f != nil && f.GetTitle() == Msg("Search.Title")
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
	for i := 0; i < frame.GetItemCount(); i++ {
		r := frame.resolveRow(i)
		if r.byteEnd == r.byteStart {
			continue
		}
		if got := r.text[r.byteStart:r.byteEnd]; got != "needle" {
			t.Errorf("row %d highlight bytes = %q, want \"needle\"", i, got)
		}
	}
	// The control-free line must keep its highlight.
	if r := frame.resolveRow(1); r.byteEnd == r.byteStart {
		t.Error("control-free long line should still highlight its match")
	}
}

func TestEditorFindAll_AccentWidthMeasured(t *testing.T) {
	ev := newFindAllEditor(t, "unit\n\n\n\n\n\n\n\n\n\nunit at line eleven")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	paintFindAll(t, frame)
	if want := vtui.StringWidth(frame.prefixFor(frame.resolveRow(0).match)); frame.accentW != want {
		t.Errorf("accentW = %d, want measured width %d", frame.accentW, want)
	}
}

func TestEditorFindAll_ResizeConsole(t *testing.T) {
	ev := newFindAllEditor(t, "Unit 15 The Avenue\nno match here\nUnited Kingdom")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	assertWithin := func(w, h int, when string) {
		t.Helper()
		x1, y1, x2, y2 := frame.GetPosition()
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

	for i, row := range paintFindAll(t, frame)[:frame.GetItemCount()] {
		if strings.TrimSpace(row) == "" {
			t.Errorf("item %d text blanked out on a narrow terminal", i)
		}
	}
}

// --- the list is a window onto the spans, not a copy of them ---

func TestFirstMatchPerLine(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		pattern string
		want    []int // offsets of the kept occurrences
	}{
		{"one per line", "aa\naa\naa\n", "aa", []int{0, 3, 6}},
		{"several per line", "aa aa aa\nbb\naa aa\n", "aa", []int{0, 12}},
		{"all on one line", "aa aa aa aa", "aa", []int{0}},
		{"single match", "xx\nyy aa\n", "aa", []int{6}},
		{"match ends a line", "aa\nbb aa\naa cc\n", "aa", []int{0, 6, 9}},
		{"nothing", "xx", "aa", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spans, err := findAllMatchSpans(context.Background(), []byte(c.data), c.pattern, true, false, false)
			if err != nil {
				t.Fatal(err)
			}
			got := firstMatchPerLine(context.Background(), []byte(c.data), spans)
			if len(got) != len(c.want) {
				t.Fatalf("firstMatchPerLine = %v, want offsets %v", got, c.want)
			}
			for i, s := range got {
				if s.Off != c.want[i] {
					t.Errorf("kept %d = %+v, want offset %d", i, s, c.want[i])
				}
			}
		})
	}
}

// TestEditorFindAll_OneItemPerLine: a line that matches three times is listed
// once, at its first occurrence; the title still counts every occurrence.
func TestEditorFindAll_OneItemPerLine(t *testing.T) {
	for _, regex := range []bool{false, true} {
		ev := newFindAllEditor(t, "unit unit unit\nnone\nunit unit")
		frame := openFindAllMenu(t, ev, "unit", false, regex, false)
		if got := frame.GetItemCount(); got != 2 {
			t.Fatalf("regex=%v: expected 2 items, got %d", regex, got)
		}
		if !strings.Contains(frame.GetTitle(), "5") || !strings.Contains(frame.GetTitle(), "2") {
			t.Errorf("regex=%v: title should read 5 occurrences on 2 lines, got %q", regex, frame.GetTitle())
		}
		if got := paintFindAll(t, frame); got[0] != "1│1│ unit unit unit" || got[1] != "3│1│ unit unit" {
			t.Errorf("regex=%v: unexpected painted rows %q", regex, got[:2])
		}
		frame.Close()
	}
}

func TestEditorFindAll_LargeListOpensWithoutMaterializing(t *testing.T) {
	// Half a million occurrences. Materializing one menu item each cost ~2 µs
	// and ~210 bytes, so this used to be a second of frozen UI and 100 MB; it
	// is now bounded by the width sample.
	const n = 500_000
	ev := newFindAllEditor(t, strings.Repeat("aaa bbbbb\n", n))
	data := []byte(strings.Repeat("aaa bbbbb\n", n))
	spans, err := findAllMatchSpans(context.Background(), data, "aaa", true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != n {
		t.Fatalf("expected %d occurrences, got %d", n, len(spans))
	}

	start := time.Now()
	ev.showFindAllMenu("aaa", spans, len(spans))
	elapsed := time.Since(start)

	frame, ok := vtui.FrameManager.GetTopFrame().(*findAllFrame)
	if !ok {
		t.Fatal("occurrences menu did not open")
	}
	defer frame.Close()
	// Under the race detector every access is instrumented, so this measures
	// the instrumentation, not the lazy open. The assertions below prove the
	// same property structurally: nothing is materialized, yet the count is
	// right.
	if !raceEnabled && elapsed > 200*time.Millisecond {
		t.Errorf("opening a %d-occurrence list took %v; it should not scale with the list", n, elapsed)
	}
	if len(frame.Items) != 0 {
		t.Errorf("menu materialized %d items, want none", len(frame.Items))
	}
	if got := frame.GetItemCount(); got != n {
		t.Errorf("item count = %d, want %d", got, n)
	}
	if !strings.Contains(frame.GetTitle(), "500000") {
		t.Errorf("title should report the true total, got %q", frame.GetTitle())
	}

	// The last occurrence must paint as cheaply as the first.
	frame.SetSelectPos(n - 1)
	start = time.Now()
	rows := paintFindAll(t, frame)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("painting the end of the list took %v", elapsed)
	}
	last := rows[len(rows)-1]
	if want := "500000│1│ aaa bbbbb"; last != want {
		t.Errorf("last row = %q, want %q", last, want)
	}
}

func TestEditorFindAll_ColumnsCountRunesNotBytes(t *testing.T) {
	ev := newFindAllEditor(t, "ёжикёжик unit\nunit")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	if got := frame.resolveRow(0).match.Col; got != 10 {
		t.Errorf("column = %d, want 10 (runes, not bytes)", got)
	}
}

func TestEditorFindAll_MouseClickJumps(t *testing.T) {
	ev := newFindAllEditor(t, "Unit 15 The Avenue\nno match here\nUnited Kingdom")
	frame := openFindAllMenu(t, ev, "unit", false, false, false)

	x1, y1, _, _ := frame.GetPosition()
	click := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      testInt16(x1 + 2),
		MouseY:      testInt16(y1 + 2), // second row
	}
	if !frame.ProcessMouse(click) {
		t.Fatal("click on an occurrence was not handled")
	}
	pumpFindAll(t, func() bool { return ev.selActive })

	wantOff := len("Unit 15 The Avenue\nno match here\n")
	if ev.selAnchorOffset != wantOff {
		t.Errorf("clicked occurrence anchor = %d, want %d", ev.selAnchorOffset, wantOff)
	}
	if !frame.IsDone() {
		t.Error("clicking an occurrence should close the menu")
	}
}

func TestEditorFindAll_F4DumpCapped(t *testing.T) {
	old := findAllDumpMaxLines
	findAllDumpMaxLines = 2
	defer func() { findAllDumpMaxLines = old }()

	ev := newFindAllEditor(t, "unit one\nunit two\nunit three\nunit four")
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
	want := "1: unit one\n2: unit two\n" + fmt.Sprintf(Msg("Search.AllEditorMore"), 2) + "\n"
	if string(data) != want {
		t.Errorf("dump mismatch:\n got %q\nwant %q", string(data), want)
	}
}

// openMappedFindAllEditor opens a real file the way showEditor opens a mapped
// one: the index is empty on arrival and the background scan fills it.
func openMappedFindAllEditor(t *testing.T, content string) *EditorView {
	t.Helper()
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "occurrences.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	ev := openMappedEditor(t, dir, path)
	t.Cleanup(func() { ev.Close() })
	ev.SetPosition(0, 0, 80, 24)
	return ev
}

// findAllCorpus is big enough that the scan of it is still running when the
// search over it finishes, which is the case the waiting exists for.
func findAllCorpus(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	if _, err := sb.WriteString("Unit 15 The Avenue\nno match here\nUnited Kingdom\n"); err != nil {
		t.Fatal(err)
	}
	for sb.Len() < 16<<20 {
		if _, err := sb.WriteString("filler line with nothing of interest in it\n"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sb.WriteString("unit again\n"); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// TestEditorFindAll_WaitsForTheLineIndex is the bug that opening a file
// without indexing it first introduced: the list resolves the line of every
// row against the live index, so a list opened while the scan was still
// running reported every occurrence on line 1, with the whole file as the text
// of the line.
func TestEditorFindAll_WaitsForTheLineIndex(t *testing.T) {
	content := findAllCorpus(t)
	ev := openMappedFindAllEditor(t, content)
	ev.StartIndexing() // as showEditor does, and then straight into the search

	frame := openFindAllMenu(t, ev, "unit", false, false, false)
	if frame == nil {
		t.Fatal("no occurrences frame appeared")
	}
	if !ev.indexIsComplete() {
		t.Errorf("the list opened against an index that is still %v", ev.indexStatus.Phase)
	}

	rows := paintFindAll(t, frame)
	if len(rows) < 3 {
		t.Fatalf("painted %d rows, want at least 3", len(rows))
	}
	lastLine := strings.Count(content, "\n")
	want := []string{
		"     1│1│ Unit 15 The Avenue",
		"     3│1│ United Kingdom",
		fmt.Sprintf("%6d│1│ unit again", lastLine),
	}
	for i, w := range want {
		if got := strings.TrimRight(rows[i], " "); got != w {
			t.Errorf("row %d = %q, want %q", i, got, w)
		}
	}
}

// TestEditorFindAll_ResolvesWithNoScanRunning covers the other half: there is
// not always a scan to wait for — one cancelled by an edit leaves the index
// short until the resume fires — and a list must not open on offsets the index
// cannot place either way.
func TestEditorFindAll_ResolvesWithNoScanRunning(t *testing.T) {
	content := "Unit 15 The Avenue\nno match here\nUnited Kingdom\n" +
		strings.Repeat("filler\n", 50) + "unit again\n"
	ev := openMappedFindAllEditor(t, content)
	if ev.li.LineCount() != 1 {
		t.Fatalf("precondition: index holds %d lines, want an empty one", ev.li.LineCount())
	}

	frame := openFindAllMenu(t, ev, "unit", false, false, false)
	if frame == nil {
		t.Fatal("no occurrences frame appeared")
	}
	rows := paintFindAll(t, frame)
	want := []string{" 1│1│ Unit 15 The Avenue", " 3│1│ United Kingdom", "54│1│ unit again"}
	for i, w := range want {
		if got := strings.TrimRight(rows[i], " "); got != w {
			t.Errorf("row %d = %q, want %q", i, got, w)
		}
	}
}

// TestFindAllMatchSpans_FoldedScanReadsTheBufferInPlace: a case-insensitive
// collection hands the buffer to strcase as a string, which used to mean a
// copy of the whole file on the most ordinary search there is — 8 GB of heap
// for the 8 GB file, on top of the mapping it was copied from. The matches
// here are sparse, so anything approaching the corpus size is that copy and
// not the spans.
func TestFindAllMatchSpans_FoldedScanReadsTheBufferInPlace(t *testing.T) {
	const gap = 4096
	corpus := bytes.Repeat(append([]byte("Needle"), bytes.Repeat([]byte("x"), gap)...), 2000)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	spans, err := findAllMatchSpans(context.Background(), corpus, "needle", false, false, false)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2000 {
		t.Fatalf("found %d occurrences, want 2000", len(spans))
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > uint64(len(corpus))/4 {
		t.Errorf("collecting allocated %d bytes over a %d byte buffer", allocated, len(corpus))
	}
}

// taskContextForTest is the handle the collection expects; the search dialog
// supplies one in the app.
func taskContextForTest(t *testing.T) *vtui.TaskContext {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &vtui.TaskContext{Context: ctx, Cancel: cancel}
}

// findAllSeamCorpus is built so that matches land on, before and after every
// seam for a range of window sizes, and so that folding changes a match's byte
// length (K U+212A folds to "k") right where a window might be cut.
func findAllSeamCorpus(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	for i := 0; sb.Len() < 40000; i++ {
		switch i % 7 {
		case 0:
			if _, err := sb.WriteString("needle in a line\n"); err != nil {
				t.Fatal(err)
			}
		case 1:
			if _, err := sb.WriteString("NEEDLE shouting\n"); err != nil {
				t.Fatal(err)
			}
		case 2:
			if _, err := sb.WriteString("nothing here at all, just filler to move the seam along\n"); err != nil {
				t.Fatal(err)
			}
		case 3:
			if _, err := sb.WriteString("two needle and needle on one line\n"); err != nil {
				t.Fatal(err)
			}
		case 4:
			if _, err := sb.WriteString("Kneedle after a kelvin sign\n"); err != nil {
				t.Fatal(err)
			}
		case 5:
			if _, err := sb.WriteString("needle"); err != nil {
				t.Fatal(err)
			}
		default:
			if _, err := sb.WriteString("tail\n"); err != nil {
				t.Fatal(err)
			}
		}
	}
	return sb.String()
}

// TestCollectMatchSpans_WindowsMatchTheWholeBuffer is what makes reading the
// file in windows safe to do: for every window size, the occurrences and the
// line count must be exactly what one pass over the whole buffer produces,
// including for matches that straddle a seam.
func TestCollectMatchSpans_WindowsMatchTheWholeBuffer(t *testing.T) {
	content := findAllSeamCorpus(t)
	data := []byte(content)

	for _, caseSensitive := range []bool{true, false} {
		want, err := findAllMatchSpans(context.Background(), data, "needle", caseSensitive, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(want) == 0 {
			t.Fatal("corpus matches nothing")
		}
		wantOccurrences := len(want)
		want = firstMatchPerLine(context.Background(), data, want)

		for _, window := range []int{7, 64, 1000, 4096, len(data) * 2} {
			ev := newFindAllEditor(t, content)
			restore := findAllWindow
			findAllWindow = window
			got, gotOccurrences, err := ev.collectMatchSpans(taskContextForTest(t), ev.editSession, ev.pt, ev.chunkReader(),
				"needle", caseSensitive, false, false)
			findAllWindow = restore
			if err != nil {
				t.Fatalf("window %d: %v", window, err)
			}

			if len(got) != len(want) {
				t.Fatalf("window %d, caseSensitive=%v: %d lines, want %d",
					window, caseSensitive, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("window %d, caseSensitive=%v: occurrence %d = %+v, want %+v",
						window, caseSensitive, i, got[i], want[i])
				}
			}
			if gotOccurrences != wantOccurrences {
				t.Errorf("window %d, caseSensitive=%v: %d occurrences, want %d",
					window, caseSensitive, gotOccurrences, wantOccurrences)
			}
		}
	}
}

// TestCollectMatchSpans_NewlineInMatchlessWindow: the newline that separates
// two matches can sit in a window holding neither of them. It still has to be
// seen, or the second match is counted on the first one's line.
func TestCollectMatchSpans_NewlineInMatchlessWindow(t *testing.T) {
	content := "X" + strings.Repeat("a", 200) + "\n" + strings.Repeat("b", 200) + "X\n"
	ev := newFindAllEditor(t, content)
	restore := findAllWindow
	findAllWindow = 64
	defer func() { findAllWindow = restore }()

	spans, occurrences, err := ev.collectMatchSpans(taskContextForTest(t), ev.editSession, ev.pt, ev.chunkReader(),
		"X", true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if occurrences != 2 {
		t.Fatalf("found %d occurrences, want 2", occurrences)
	}
	if len(spans) != 2 {
		t.Errorf("listed %d matching lines, want 2", len(spans))
	}
}

// TestLineHead_CutsOnACharacterBoundary: a line longer than the cap is cut on
// a rune boundary and marked, so the dump it is written into stays UTF-8.
func TestLineHead_CutsOnACharacterBoundary(t *testing.T) {
	// The euro sign straddles the cap: two of its three bytes fit.
	content := strings.Repeat("a", findAllMaxLineBytes-2) + "€" + strings.Repeat("b", 100) + "\n"
	ev := newFindAllEditor(t, content)
	head := ev.lineHead(0, len(content)-1)
	if !utf8.ValidString(head) {
		t.Fatalf("head is not valid UTF-8")
	}
	if want := strings.Repeat("a", findAllMaxLineBytes-2) + "…"; head != want {
		t.Errorf("head = %q..., want %d a's and an ellipsis", head[max(len(head)-8, 0):], findAllMaxLineBytes-2)
	}
	if short := ev.lineHead(0, 5); short != "aaaaa" {
		t.Errorf("short line = %q, want aaaaa", short)
	}
}

// TestCollectMatchSpans_ReadsTheFileNotTheMapping: the collection is why Find
// All on a large file used to fault the whole thing into residency. It should
// now ask the file for windows, as the line index does.
func TestCollectMatchSpans_ReadsTheFileNotTheMapping(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "occurrences.txt")
	content := strings.Repeat("a line with needle in it\n", 20000)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	ev := openMappedEditor(t, dir, path)
	t.Cleanup(func() { ev.Close() })

	counter := &countingReadAtCloser{ReadAtCloser: ev.file}
	ev.file = counter

	spans, lines, err := ev.collectMatchSpans(taskContextForTest(t), ev.editSession, ev.pt, ev.chunkReader(), "needle", true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 20000 {
		t.Errorf("found %d occurrences, want 20000", len(spans))
	}
	if lines != 20000 {
		t.Errorf("counted %d matching lines, want 20000", lines)
	}
	if calls, read := counter.counted(); calls == 0 || read < int64(len(content)) {
		t.Errorf("the collection read %d bytes in %d calls; it walked the mapping instead", read, calls)
	}
}
