// Package diffview shows two files side by side with their line-level
// differences highlighted -- f4's answer to Total Commander's "Compare by
// content". This first version is deliberately basic: two files (not
// directories), plain line diffing (no syntax highlighting, that stays the
// editor's job), no in-place editing. See internal/textdiff for the actual
// comparison algorithm; this package only lays the result out on screen.
package diffview

import (
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/textdiff"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/wheel"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// DiffView is a read-only vtui.Frame, opened with vtui.FrameManager.AddScreen
// the same way the viewer is: its own workspace, not a dialog. Esc and F10
// close it.
type DiffView struct {
	vtui.BaseFrame

	LeftTitle, RightTitle string
	rows                  []textdiff.Row

	// topPos is the first row shown. Both panes always scroll together --
	// that is the point of a side-by-side diff -- so there is exactly one
	// scroll position for the whole view, not one per pane.
	topPos int
	// cursor is the row Next/Prev-difference navigation moves from. It is
	// not painted as a selection: the view is text to read, not a list to
	// pick from. Keeping it lets repeated jumps move on from where the user
	// last landed instead of restarting from the top every time.
	cursor int

	// wheelCoast is what a fast wheel spin leaves behind: rows the diff
	// still owes the scroll position (see internal/wheel).
	wheelCoast wheel.Coast
}

// NewDiffView builds a view from the two files' content, already split into
// lines. It returns textdiff.ErrTooLarge unchanged when the two files
// together exceed textdiff.MaxLines -- the caller decides how to tell the
// user (a message box, in f4's case).
func NewDiffView(leftTitle, rightTitle string, left, right []string) (*DiffView, error) {
	ops, err := textdiff.Diff(left, right)
	if err != nil {
		return nil, err
	}
	dv := &DiffView{
		LeftTitle:  leftTitle,
		RightTitle: rightTitle,
		rows:       textdiff.Rows(left, right, ops),
	}
	dv.jumpToFirstDifference()
	return dv, nil
}

// jumpToFirstDifference scrolls straight to the first differing row, if any,
// rather than opening on a possibly long unchanged header. Identical files
// simply open at the top. This runs at construction time, before the frame
// has a real size (ResizeConsole has not been called yet), so it sets topPos
// directly rather than through centerOn, which needs a real viewport height
// to mean anything.
func (dv *DiffView) jumpToFirstDifference() {
	for i, r := range dv.rows {
		if r.Left.Kind != textdiff.RowEqual {
			dv.cursor = i
			dv.topPos = i
			return
		}
	}
}

func (dv *DiffView) viewHeight() int {
	h := dv.Y2 - dv.Y1 - 1 // minus top and bottom border rows
	if h < 0 {
		return 0
	}
	return h
}

// centerOn scrolls so that row i is roughly in the middle of the view,
// clamped to the ends of the row list.
func (dv *DiffView) centerOn(i int) {
	h := dv.viewHeight()
	top := i - h/2
	maxTop := len(dv.rows) - h
	if maxTop < 0 {
		maxTop = 0
	}
	if top > maxTop {
		top = maxTop
	}
	if top < 0 {
		top = 0
	}
	dv.topPos = top
}

// GetType identifies DiffView on the frame stack (e.g. for macro/dispatch
// routing that switches on frame type), the same way the viewer, editor and
// panels frame each claim their own vtui.TypeUser+N slot.
func (dv *DiffView) GetType() vtui.FrameType { return vtui.TypeUser + 9 }

func (dv *DiffView) GetTitle() string {
	return "Compare: " + dv.LeftTitle + " <-> " + dv.RightTitle
}

func (dv *DiffView) ResizeConsole(w, h int) {
	top := vtui.FrameManager.WorkspaceTopInset()
	dv.SetPosition(0, top, w-1, h-2)
}

func (dv *DiffView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{"", "", "", "", "", "", "", "", "", i18n.Msg("DiffView.Close")},
	}
}

// ProcessKey handles scrolling and closing. Everything else (mouse wheel
// aside, which HandleMouseScroll-style wiring is a natural follow-up) is out
// of scope for this first version.
func (dv *DiffView) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	h := dv.viewHeight()
	maxTop := len(dv.rows) - h
	if maxTop < 0 {
		maxTop = 0
	}
	clampTop := func() {
		if dv.topPos > maxTop {
			dv.topPos = maxTop
		}
		if dv.topPos < 0 {
			dv.topPos = 0
		}
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		dv.SetExitCode(-1)
		return true
	case vtinput.VK_UP:
		if ctrl {
			dv.jumpToDifference(-1)
			return true
		}
		dv.topPos--
		clampTop()
		return true
	case vtinput.VK_DOWN:
		if ctrl {
			dv.jumpToDifference(1)
			return true
		}
		dv.topPos++
		clampTop()
		return true
	case vtinput.VK_PRIOR: // PgUp
		dv.topPos -= max(h, 1)
		clampTop()
		return true
	case vtinput.VK_NEXT: // PgDn
		dv.topPos += max(h, 1)
		clampTop()
		return true
	case vtinput.VK_HOME:
		dv.topPos = 0
		return true
	case vtinput.VK_END:
		dv.topPos = maxTop
		return true
	}
	return false
}

// jumpToDifference moves dv.cursor to the next (dir>0) or previous (dir<0)
// row whose left side is not RowEqual, scrolling it into view. It wraps
// around, the same way most "find next" navigation in f4 does.
func (dv *DiffView) jumpToDifference(dir int) {
	if len(dv.rows) == 0 {
		return
	}
	i := dv.cursor
	for step := 0; step < len(dv.rows); step++ {
		i += dir
		if i < 0 {
			i = len(dv.rows) - 1
		}
		if i >= len(dv.rows) {
			i = 0
		}
		if dv.rows[i].Left.Kind != textdiff.RowEqual {
			dv.cursor = i
			dv.centerOn(i)
			return
		}
	}
}

// Show paints both panes as two adjacent bordered boxes, the same visual
// language as the two file panels: a shared border column down the middle,
// a title on each top border, and one row per diff line colored by kind.
func (dv *DiffView) Show(scr *vtui.ScreenBuf) {
	x1, y1, x2, y2 := dv.X1, dv.Y1, dv.X2, dv.Y2
	if x2 <= x1 || y2 <= y1 {
		return
	}
	splitX := x1 + (x2-x1)/2

	boxAttr := vtui.Palette[theme.ColPanelBox]
	textAttr := vtui.Palette[theme.ColViewerText]

	p := vtui.NewPainter(scr)
	p.Fill(x1, y1, x2, y2, ' ', textAttr)
	p.DrawBox(x1, y1, splitX, y2, boxAttr, vtui.SingleBox)
	p.DrawBox(splitX, y1, x2, y2, boxAttr, vtui.SingleBox)
	p.DrawTitle(x1, y1, splitX, dv.LeftTitle, boxAttr)
	p.DrawTitle(splitX, y1, x2, dv.RightTitle, boxAttr)

	leftInnerX1, leftInnerX2 := x1+1, splitX-1
	rightInnerX1, rightInnerX2 := splitX+1, x2-1
	leftWidth := leftInnerX2 - leftInnerX1 + 1
	rightWidth := rightInnerX2 - rightInnerX1 + 1
	if leftWidth <= 0 || rightWidth <= 0 {
		return
	}

	height := dv.viewHeight()
	if maxTop := len(dv.rows) - height; maxTop < 0 {
		dv.topPos = 0
	} else if dv.topPos > maxTop {
		dv.topPos = maxTop
	} else if dv.topPos < 0 {
		dv.topPos = 0
	}
	for i := 0; i < height; i++ {
		idx := dv.topPos + i
		if idx < 0 || idx >= len(dv.rows) {
			break
		}
		row := dv.rows[idx]
		y := y1 + 1 + i

		leftAttr := rowAttr(textAttr, row.Left.Kind)
		rightAttr := rowAttr(textAttr, row.Right.Kind)
		if idx == dv.cursor {
			leftAttr = vtui.InvertColors(leftAttr)
			rightAttr = vtui.InvertColors(rightAttr)
		}

		p.Fill(leftInnerX1, y, leftInnerX2, y, ' ', leftAttr)
		p.Fill(rightInnerX1, y, rightInnerX2, y, ' ', rightAttr)
		if row.Left.Kind != textdiff.RowFiller {
			p.DrawString(leftInnerX1, y, vtui.TruncateString(row.Left.Text, leftWidth, ""), leftAttr)
		}
		if row.Right.Kind != textdiff.RowFiller {
			p.DrawString(rightInnerX1, y, vtui.TruncateString(row.Right.Text, rightWidth, ""), rightAttr)
		}
	}
}

// rowAttr tints the base text attribute's background according to how a
// side of a row relates to the other file. The tint is a fixed first pass,
// deliberately not theme-configurable yet -- see the discussion in f4#613.
func rowAttr(base uint64, kind textdiff.RowKind) uint64 {
	switch kind {
	case textdiff.RowChanged:
		return vtui.SetRGBBack(base, 0x5A5A23) // muted olive: changed on both sides
	case textdiff.RowDeleted:
		return vtui.SetRGBBack(base, 0x5A2323) // muted red: only on the left
	case textdiff.RowInserted:
		return vtui.SetRGBBack(base, 0x235A23) // muted green: only on the right
	default:
		return base
	}
}

// ProcessMouse gives the view mouse wheel scrolling, the one mouse gesture a
// read-only comparison genuinely needs; clicking a row does nothing yet.
func (dv *DiffView) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.WheelDirection == 0 {
		return false
	}
	direction := 1
	if e.WheelDirection > 0 {
		direction = -1
	}
	// A spin faster than one notch per spin window queues extra lines the
	// diff keeps scrolling on its own (see internal/wheel).
	dv.wheelCoast.Notch(direction, dv.scrollBy)
	dv.scrollBy(direction * 3)
	return true
}

// scrollBy moves the first shown row by step rows, positive down the diff,
// and reports whether anything moved so a coast stops at an end of the
// comparison instead of spinning in place.
func (dv *DiffView) scrollBy(step int) bool {
	before := dv.topPos
	dv.topPos += step
	h := dv.viewHeight()
	maxTop := len(dv.rows) - h
	if maxTop < 0 {
		maxTop = 0
	}
	if dv.topPos > maxTop {
		dv.topPos = maxTop
	}
	if dv.topPos < 0 {
		dv.topPos = 0
	}
	return dv.topPos != before
}
