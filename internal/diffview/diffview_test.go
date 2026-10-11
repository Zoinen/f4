package diffview

import (
	"testing"

	"github.com/unxed/f4/internal/textdiff"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func newTestDiffView(t *testing.T, left, right []string) *DiffView {
	t.Helper()
	dv, err := NewDiffView("left.txt", "right.txt", left, right)
	if err != nil {
		t.Fatalf("NewDiffView: %v", err)
	}
	return dv
}

func TestNewDiffViewJumpsToFirstDifference(t *testing.T) {
	left := []string{"a", "b", "same", "c"}
	right := []string{"a", "x", "same", "c"}
	dv := newTestDiffView(t, left, right)

	if len(dv.rows) == 0 {
		t.Fatal("expected rows to be populated")
	}
	if dv.rows[dv.cursor].Left.Kind == textdiff.RowEqual {
		t.Fatalf("cursor row %d should not be RowEqual, got %#v", dv.cursor, dv.rows[dv.cursor])
	}
	if dv.topPos != dv.cursor {
		t.Fatalf("expected topPos to start at the first difference (%d), got %d", dv.cursor, dv.topPos)
	}
}

func TestNewDiffViewIdenticalFilesOpenAtTop(t *testing.T) {
	lines := []string{"a", "b", "c"}
	dv := newTestDiffView(t, lines, lines)
	if dv.topPos != 0 || dv.cursor != 0 {
		t.Fatalf("expected identical files to open at the top, got topPos=%d cursor=%d", dv.topPos, dv.cursor)
	}
	for _, r := range dv.rows {
		if r.Left.Kind != textdiff.RowEqual {
			t.Fatalf("expected only RowEqual rows for identical inputs, got %#v", r)
		}
	}
}

func TestDiffViewTooLarge(t *testing.T) {
	big := make([]string, textdiff.MaxLines)
	for i := range big {
		big[i] = "x"
	}
	if _, err := NewDiffView("a", "b", big, []string{"y"}); err != textdiff.ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

// key sends a synthetic key-down event through ProcessKey, the same way a
// real terminal input event would arrive.
func key(dv *DiffView, vk uint16, ctrl bool) bool {
	state := vtinput.ControlKeyState(0)
	if ctrl {
		state = vtinput.LeftCtrlPressed
	}
	return dv.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vk,
		ControlKeyState: state,
	})
}

func TestDiffViewEscapeCloses(t *testing.T) {
	dv := newTestDiffView(t, []string{"a"}, []string{"b"})
	if dv.IsDone() {
		t.Fatal("expected the view not to be done before Escape")
	}
	if !key(dv, vtinput.VK_ESCAPE, false) {
		t.Fatal("expected Escape to be handled")
	}
	if !dv.IsDone() {
		t.Fatal("expected Escape to close the view")
	}
}

func TestDiffViewScrollClampsToContent(t *testing.T) {
	var left, right []string
	for i := 0; i < 5; i++ {
		left = append(left, "line")
		right = append(right, "line")
	}
	dv := newTestDiffView(t, left, right)
	dv.SetPosition(0, 0, 20, 3) // 4 rows tall -> 2 content rows (minus 2 border rows)

	// Scrolling up past the top must clamp at 0.
	key(dv, vtinput.VK_UP, false)
	key(dv, vtinput.VK_UP, false)
	if dv.topPos != 0 {
		t.Fatalf("expected topPos clamped to 0, got %d", dv.topPos)
	}

	// Scrolling down past the end must clamp at len(rows)-viewHeight.
	for i := 0; i < 20; i++ {
		key(dv, vtinput.VK_DOWN, false)
	}
	maxTop := len(dv.rows) - dv.viewHeight()
	if maxTop < 0 {
		maxTop = 0
	}
	if dv.topPos != maxTop {
		t.Fatalf("expected topPos clamped to %d, got %d", maxTop, dv.topPos)
	}
}

func TestDiffViewJumpToDifferenceWraps(t *testing.T) {
	left := []string{"a", "b1", "c", "d", "e2"}
	right := []string{"a", "b2", "c", "d", "e1"}
	dv := newTestDiffView(t, left, right)
	dv.SetPosition(0, 0, 20, 6)

	first := dv.cursor
	key(dv, vtinput.VK_DOWN, true) // Ctrl+Down: next difference
	second := dv.cursor
	if second == first {
		t.Fatalf("expected cursor to move to a different row, stayed at %d", first)
	}
	if dv.rows[second].Left.Kind == textdiff.RowEqual {
		t.Fatalf("row %d should be a difference, got %#v", second, dv.rows[second])
	}

	key(dv, vtinput.VK_UP, true) // Ctrl+Up: back to the previous difference
	if dv.cursor != first {
		t.Fatalf("expected Ctrl+Up to return to row %d, got %d", first, dv.cursor)
	}
}

// TestDiffViewShowDoesNotPanic exercises the full paint path against a real
// (silent) ScreenBuf, the way internal/viewer's own tests do -- a plain smoke
// test for slice bounds and nil derefs that the logic-only tests above
// cannot see, since they never call Show.
func TestDiffViewShowDoesNotPanic(t *testing.T) {
	theme.SetDefaultF4Palette()

	left := []string{"a", "b", "c", "d", "e", "f"}
	right := []string{"a", "x", "c", "y", "z", "f"}
	dv := newTestDiffView(t, left, right)
	dv.SetPosition(0, 0, 39, 9)

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 10)

	dv.Show(scr)
	// Scroll around and repaint, including past both ends, to exercise the
	// clamping paths inside Show itself.
	dv.topPos = -5
	dv.Show(scr)
	dv.topPos = len(dv.rows) + 5
	dv.Show(scr)
}

// TestDiffViewShowZeroSizeDoesNotPanic covers the frame being painted before
// ResizeConsole/SetPosition ever ran (X1..Y2 all zero), which is exactly the
// state NewDiffView leaves a view in.
func TestDiffViewShowZeroSizeDoesNotPanic(t *testing.T) {
	theme.SetDefaultF4Palette()
	dv := newTestDiffView(t, []string{"a"}, []string{"b"})
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(1, 1)
	dv.Show(scr)
}

func TestDiffViewFrameMetadata(t *testing.T) {
	dv := newTestDiffView(t, []string{"a"}, []string{"b"})

	if got, want := dv.GetType(), vtui.TypeUser+9; got != want {
		t.Fatalf("GetType() = %v, want %v", got, want)
	}
	if got, want := dv.GetTitle(), "Compare: left.txt <-> right.txt"; got != want {
		t.Fatalf("GetTitle() = %q, want %q", got, want)
	}
}

// TestDiffViewResizeConsole checks the arithmetic ResizeConsole wires into
// SetPosition (full width minus one, full height minus the key bar row),
// without hard-coding vtui.FrameManager's workspace-tab inset -- that value
// is FrameManager's own business, DiffView just has to pass it through as Y1.
func TestDiffViewResizeConsole(t *testing.T) {
	dv := newTestDiffView(t, []string{"a"}, []string{"b"})
	dv.ResizeConsole(80, 24)

	wantY1 := vtui.FrameManager.WorkspaceTopInset()
	if dv.X1 != 0 || dv.Y1 != wantY1 || dv.X2 != 79 || dv.Y2 != 22 {
		t.Fatalf("ResizeConsole(80, 24) placed the frame at (%d,%d)-(%d,%d), want (0,%d)-(79,22)",
			dv.X1, dv.Y1, dv.X2, dv.Y2, wantY1)
	}
}

func TestDiffViewGetKeyLabels(t *testing.T) {
	dv := newTestDiffView(t, []string{"a"}, []string{"b"})
	labels := dv.GetKeyLabels()
	for i, label := range labels.Normal {
		if i == 9 {
			if label == "" {
				t.Fatal("expected the F10 slot to carry the Close label")
			}
			continue
		}
		if label != "" {
			t.Fatalf("expected key-bar slot %d to be empty, got %q", i, label)
		}
	}
}

// TestRowAttr locks down which tint each row kind gets: a swapped
// deleted/inserted pair (red for additions, green for removals) would be a
// real, user-visible bug that Show's smoke tests above cannot catch, since
// they never inspect the attribute a row was actually painted with.
func TestRowAttr(t *testing.T) {
	const base uint64 = 0x1234

	tests := []struct {
		name string
		kind textdiff.RowKind
		want uint64
	}{
		{"equal falls through to the base attribute", textdiff.RowEqual, base},
		{"changed tints olive", textdiff.RowChanged, vtui.SetRGBBack(base, 0x5A5A23)},
		{"deleted tints red", textdiff.RowDeleted, vtui.SetRGBBack(base, 0x5A2323)},
		{"inserted tints green", textdiff.RowInserted, vtui.SetRGBBack(base, 0x235A23)},
		{"filler falls through to the base attribute", textdiff.RowFiller, base},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rowAttr(base, tt.kind); got != tt.want {
				t.Fatalf("rowAttr(base, %v) = %#x, want %#x", tt.kind, got, tt.want)
			}
		})
	}
}

func TestDiffViewProcessKeyIgnoresKeyUpEvents(t *testing.T) {
	dv := newTestDiffView(t, []string{"a", "b"}, []string{"a", "c"})
	dv.SetPosition(0, 0, 20, 5)
	before := dv.topPos

	if dv.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        false,
		VirtualKeyCode: vtinput.VK_DOWN,
	}) {
		t.Fatal("expected a key-up event to be reported as unhandled")
	}
	if dv.topPos != before {
		t.Fatalf("expected topPos unchanged by a key-up event, got %d (was %d)", dv.topPos, before)
	}
}

func TestDiffViewProcessKeyUnhandledKeyIsNoop(t *testing.T) {
	dv := newTestDiffView(t, []string{"a", "b"}, []string{"a", "c"})
	dv.SetPosition(0, 0, 20, 5)
	before := dv.topPos

	if key(dv, vtinput.VK_SPACE, false) {
		t.Fatal("expected an unrelated key to be reported as unhandled")
	}
	if dv.topPos != before {
		t.Fatalf("expected topPos unchanged by an unhandled key, got %d (was %d)", dv.topPos, before)
	}
}

// TestDiffViewProcessKeyPageAndHomeEnd covers PgUp/PgDn/Home/End, none of
// which TestDiffViewScrollClampsToContent above exercises (it only drives
// VK_UP/VK_DOWN).
func TestDiffViewProcessKeyPageAndHomeEnd(t *testing.T) {
	var left, right []string
	for i := 0; i < 20; i++ {
		left = append(left, "line")
		right = append(right, "line")
	}
	dv := newTestDiffView(t, left, right)
	dv.SetPosition(0, 0, 20, 6) // 5 content rows tall
	height := dv.viewHeight()
	maxTop := len(dv.rows) - height

	tests := []struct {
		name  string
		vk    uint16
		setup func()
		want  int
	}{
		{"PageDown scrolls by a full view height", vtinput.VK_NEXT, func() { dv.topPos = 0 }, height},
		{"PageDown clamps at the bottom", vtinput.VK_NEXT, func() { dv.topPos = maxTop }, maxTop},
		{"PageUp scrolls back by a full view height", vtinput.VK_PRIOR, func() { dv.topPos = height }, 0},
		{"PageUp clamps at the top", vtinput.VK_PRIOR, func() { dv.topPos = 0 }, 0},
		{"Home jumps to the top", vtinput.VK_HOME, func() { dv.topPos = maxTop }, 0},
		{"End jumps to the bottom", vtinput.VK_END, func() { dv.topPos = 0 }, maxTop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			if !key(dv, tt.vk, false) {
				t.Fatal("expected the key to be handled")
			}
			if dv.topPos != tt.want {
				t.Fatalf("topPos = %d, want %d", dv.topPos, tt.want)
			}
		})
	}
}

// TestDiffViewProcessKeyMaxTopClampsWhenContentFitsView covers the view
// being taller than the diff itself, where maxTop would go negative and
// must clamp to 0 -- otherwise PgDn/End could scroll the content clean off
// the top of a short file.
func TestDiffViewProcessKeyMaxTopClampsWhenContentFitsView(t *testing.T) {
	dv := newTestDiffView(t, []string{"a", "b"}, []string{"a", "c"})
	dv.SetPosition(0, 0, 20, 10) // 9 content rows, far more than the 2 rows of diff

	if !key(dv, vtinput.VK_NEXT, false) { // PgDn
		t.Fatal("expected PgDn to be handled")
	}
	if dv.topPos != 0 {
		t.Fatalf("expected topPos to stay clamped at 0 when the view is taller than the content, got %d", dv.topPos)
	}
}

func TestDiffViewJumpToDifferenceNoRows(t *testing.T) {
	dv := newTestDiffView(t, nil, nil)
	dv.SetPosition(0, 0, 20, 5)
	if len(dv.rows) != 0 {
		t.Fatalf("expected an empty diff of two empty files, got %d rows", len(dv.rows))
	}

	key(dv, vtinput.VK_DOWN, true) // Ctrl+Down on an empty diff must not panic or move anything.
	if dv.cursor != 0 || dv.topPos != 0 {
		t.Fatalf("expected cursor/topPos to stay at 0 on an empty diff, got cursor=%d topPos=%d", dv.cursor, dv.topPos)
	}
}

// TestDiffViewJumpToDifferenceWrapsAroundBothEnds exercises the wrap-around
// promised by jumpToDifference's doc comment in both directions: past the
// last difference back to the first, and past the first back to the last.
func TestDiffViewJumpToDifferenceWrapsAroundBothEnds(t *testing.T) {
	left := []string{"x1", "same", "x2"}
	right := []string{"y1", "same", "y2"}

	t.Run("Ctrl+Down wraps from the last difference to the first", func(t *testing.T) {
		dv := newTestDiffView(t, left, right)
		dv.SetPosition(0, 0, 20, 6)
		if dv.cursor != 0 {
			t.Fatalf("expected construction to land on the first difference (row 0), got %d", dv.cursor)
		}

		key(dv, vtinput.VK_DOWN, true) // the only other difference is row 2
		if dv.cursor != 2 {
			t.Fatalf("expected Ctrl+Down to land on row 2, got %d", dv.cursor)
		}

		key(dv, vtinput.VK_DOWN, true) // past the end, wraps back to row 0
		if dv.cursor != 0 {
			t.Fatalf("expected Ctrl+Down to wrap back to row 0, got %d", dv.cursor)
		}
	})

	t.Run("Ctrl+Up wraps from the first difference to the last", func(t *testing.T) {
		dv := newTestDiffView(t, left, right)
		dv.SetPosition(0, 0, 20, 6)

		key(dv, vtinput.VK_UP, true) // past the start, wraps to the last difference, row 2
		if dv.cursor != 2 {
			t.Fatalf("expected Ctrl+Up to wrap to row 2, got %d", dv.cursor)
		}
	})
}

// TestCenterOnClampsToZeroWhenViewTallerThanContent covers centerOn when the
// whole diff already fits the view (maxTop would go negative): it must not
// leave topPos at whatever centering math it computed, or the content would
// scroll away from the top for no reason.
func TestCenterOnClampsToZeroWhenViewTallerThanContent(t *testing.T) {
	dv := newTestDiffView(t, []string{"a", "b", "c"}, []string{"a", "b", "c"})
	dv.SetPosition(0, 0, 20, 12) // 11 content rows, far more than the 3 rows of diff
	dv.topPos = 5                // any nonzero value the clamp must override

	dv.centerOn(1)

	if dv.topPos != 0 {
		t.Fatalf("expected centerOn to clamp topPos to 0 when the view is taller than the content, got %d", dv.topPos)
	}
}

// TestDiffViewShowNarrowWidthDoesNotPaint is a regression test for the
// leftWidth/rightWidth <= 0 guard in Show: at this width splitX lands right
// on x1, so each pane's inner width comes out to zero. Without the guard,
// Show would hand that non-positive width straight to vtui.TruncateString
// and risk a slice-bounds panic instead of just skipping the row.
func TestDiffViewShowNarrowWidthDoesNotPaint(t *testing.T) {
	theme.SetDefaultF4Palette()
	dv := newTestDiffView(t, []string{"a"}, []string{"b"})
	dv.SetPosition(0, 0, 2, 5)

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(3, 6)
	dv.Show(scr)
}

// TestDiffViewShowScrollBranches covers the topPos-clamping branches inside
// Show that TestDiffViewShowDoesNotPanic above cannot reach: that smoke test
// keeps the view taller than the content (maxTop negative), so the
// topPos>maxTop and topPos<0 branches never run. Here the content is taller
// than the view instead, so both are reachable and their result is checked
// rather than only "did not panic".
func TestDiffViewShowScrollBranches(t *testing.T) {
	theme.SetDefaultF4Palette()
	var left, right []string
	for i := 0; i < 20; i++ {
		left = append(left, "line")
		right = append(right, "line")
	}
	dv := newTestDiffView(t, left, right)
	dv.SetPosition(0, 0, 39, 5) // 4 content rows, shorter than the 20 rows of diff
	maxTop := len(dv.rows) - dv.viewHeight()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 6)

	dv.topPos = maxTop + 100
	dv.Show(scr)
	if dv.topPos != maxTop {
		t.Fatalf("expected Show to clamp topPos down to %d, got %d", maxTop, dv.topPos)
	}

	dv.topPos = -100
	dv.Show(scr)
	if dv.topPos != 0 {
		t.Fatalf("expected Show to clamp topPos up to 0, got %d", dv.topPos)
	}
}

// wheelEvent sends a synthetic mouse-wheel event through ProcessMouse.
func wheelEvent(dv *DiffView, dir int) bool {
	return dv.ProcessMouse(&vtinput.InputEvent{
		Type:           vtinput.MouseEventType,
		WheelDirection: dir,
	})
}

func TestDiffViewProcessMouseWheel(t *testing.T) {
	var left, right []string
	for i := 0; i < 20; i++ {
		left = append(left, "line")
		right = append(right, "line")
	}
	dv := newTestDiffView(t, left, right)
	dv.SetPosition(0, 0, 20, 6) // 5 content rows tall
	maxTop := len(dv.rows) - dv.viewHeight()

	if wheelEvent(dv, 0) {
		t.Fatal("expected a zero WheelDirection to be reported as unhandled")
	}
	if dv.topPos != 0 {
		t.Fatalf("expected topPos unchanged by a no-op wheel event, got %d", dv.topPos)
	}

	dv.topPos = 10
	if !wheelEvent(dv, -1) { // wheel down
		t.Fatal("expected wheel-down to be handled")
	}
	if want := 13; dv.topPos != want {
		t.Fatalf("expected wheel-down to scroll by 3 lines to %d, got %d", want, dv.topPos)
	}

	dv.topPos = maxTop - 1
	if !wheelEvent(dv, -1) { // wheel down past the bottom clamps
		t.Fatal("expected wheel-down to be handled")
	}
	if dv.topPos != maxTop {
		t.Fatalf("expected wheel-down to clamp at %d, got %d", maxTop, dv.topPos)
	}

	dv.topPos = 2
	if !wheelEvent(dv, 1) { // wheel up past the top clamps
		t.Fatal("expected wheel-up to be handled")
	}
	if dv.topPos != 0 {
		t.Fatalf("expected wheel-up to clamp at 0, got %d", dv.topPos)
	}
}
