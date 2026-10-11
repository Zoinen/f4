package panel

import (
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func headerZoneFixture(mode ViewMode) *FileSystemPanel {
	fp := newPanelScrollTestFixture(mode, 1)
	fp.Frame = vtui.NewBorderedFrame(0, 0, 79, 11, vtui.SingleBox, "")
	fp.SortMode = SortName
	fp.SortReverse = false
	fp.Resize(80, 12)
	return fp
}

func headerClick(fp *FileSystemPanel, x int) bool {
	return fp.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: testutil.Int16(x), MouseY: testutil.Int16(fp.Table.Y1),
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})
}

// A click in the blank part of a header sorts nothing: only the label does
// (f4#1769).
func TestHeaderSortZoneIsTheLabelNotTheWholeColumn(t *testing.T) {
	fp := headerZoneFixture(ViewModeDetailed)
	startX := fp.Table.X1
	nameStart, nameEnd := fp.headerLabelSpan(0)
	if nameStart != 0 || nameEnd != runewidth.StringWidth("Name ↑") {
		t.Fatalf("Name label span = [%d,%d), want it to cover %q", nameStart, nameEnd, fp.Table.Columns[0].Title)
	}
	if _, ok := fp.headerSortModeAt(startX+nameEnd-1, fp.Table.Y1); !ok {
		t.Fatal("the last cell of the Name label does not sort")
	}
	if mode, ok := fp.headerSortModeAt(startX+nameEnd, fp.Table.Y1); ok {
		t.Fatalf("the blank cell right after the Name label sorts by %v", mode)
	}
	if mode, ok := fp.headerSortModeAt(startX+fp.Table.Columns[0].Width-1, fp.Table.Y1); ok {
		t.Fatalf("the far end of the Name header sorts by %v", mode)
	}

	// A click on the blank part changes neither the mode nor the order.
	headerClick(fp, startX+nameEnd+2)
	if fp.SortMode != SortName || fp.SortReverse {
		t.Fatalf("blank-header click changed the sort: mode=%v reverse=%v", fp.SortMode, fp.SortReverse)
	}
}

// The Size title is right-aligned: its zone sits at the right end of the
// column, not at the left where the padding is.
func TestHeaderSortZoneFollowsRightAlignedTitle(t *testing.T) {
	fp := headerZoneFixture(ViewModeDetailed)
	fp.SortMode = SortName
	fp.updateSortColumnTitles()
	sizeColumn := 1
	columnX := fp.Table.X1 + fp.Table.Columns[0].Width + 1
	start, end := fp.headerLabelSpan(sizeColumn)
	width := fp.Table.Columns[sizeColumn].Width
	if end != width || start <= 0 {
		t.Fatalf("Size label span = [%d,%d) in a %d-wide column, want it flush right", start, end, width)
	}
	if _, ok := fp.headerSortModeAt(columnX, fp.Table.Y1); ok {
		t.Fatal("the padding in front of the right-aligned Size title sorts")
	}
	if mode, ok := fp.headerSortModeAt(columnX+start, fp.Table.Y1); !ok || mode != SortSize {
		t.Fatalf("the first cell of the Size title maps to %v,%v; want SortSize,true", mode, ok)
	}
	if mode, ok := fp.headerSortModeAt(columnX+end-1, fp.Table.Y1); !ok || mode != SortSize {
		t.Fatalf("the last cell of the Size title maps to %v,%v; want SortSize,true", mode, ok)
	}
}

// With the active mode hidden, "Name" and "[Extension]↓" are two separate
// targets and the gap between them is neither: a miss near Name no longer
// switches the mode away from the one the user picked.
func TestHeaderSortZoneWithHiddenModeLabel(t *testing.T) {
	fp := headerZoneFixture(ViewModeDetailed)
	fp.SortMode = SortExt
	fp.updateSortColumnTitles()
	if !fp.sortModeIsHidden() {
		t.Fatal("Extension should be a hidden sort mode in the detailed view")
	}
	x0 := fp.Table.X1
	width := fp.Table.Columns[0].Width
	right := hiddenSortColumnTitle(fp.SortMode, fp.SortIsAscending(), width)
	labelX := x0 + width - runewidth.StringWidth(right)

	if mode, ok := fp.headerSortModeAt(x0, fp.Table.Y1); !ok || mode != SortName {
		t.Fatalf("the Name title maps to %v,%v; want SortName,true", mode, ok)
	}
	if mode, ok := fp.headerSortModeAt(labelX, fp.Table.Y1); !ok || mode != SortExt {
		t.Fatalf("the hidden mode label maps to %v,%v; want SortExt,true", mode, ok)
	}
	gapX := x0 + runewidth.StringWidth("Name") + 1
	if gapX >= labelX {
		t.Fatalf("test layout has no gap between the title and the label (gap %d, label %d)", gapX, labelX)
	}
	if mode, ok := fp.headerSortModeAt(gapX, fp.Table.Y1); ok {
		t.Fatalf("the gap between Name and the mode label sorts by %v", mode)
	}
	headerClick(fp, gapX)
	if fp.SortMode != SortExt || fp.SortReverse {
		t.Fatalf("a click in the gap changed the sort: mode=%v reverse=%v", fp.SortMode, fp.SortReverse)
	}
}
