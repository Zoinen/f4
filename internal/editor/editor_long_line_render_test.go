package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

// With wrapping off a fragment is the whole logical line, and a log line of
// tens of kilobytes was segmented into grapheme clusters and turned into cells
// in full on every frame it was on screen — for a viewport that shows a couple
// of hundred columns of it. That was the stutter of scrolling a log.
func TestFillCells_StopsAtTheRightEdgeOfTheViewport(t *testing.T) {
	ev := &EditorView{TabSize: 8}
	const width = 200
	line := []byte(strings.Repeat("x", 1<<20))

	start := time.Now()
	cells := ev.fillCellsWithLinks(nil, line, 0, 0, 0, false, 0, 0, nil, nil, 0, width, false, -1, 0, 0, 0)
	elapsed := time.Since(start)

	if len(cells) != width {
		t.Errorf("built %d cells for a %d column viewport", len(cells), width)
	}
	// Building the megabyte took hundreds of milliseconds; a viewport's worth
	// is microseconds. The bound is loose enough for a loaded machine and
	// still nowhere near the whole line.
	if elapsed > 50*time.Millisecond {
		t.Errorf("building one row of a 1 MB line took %v", elapsed)
	}

	// Without a limit the whole fragment is still built, which is what the
	// callers that hand over a line already cut to size rely on.
	if got := ev.fillCells(nil, []byte("abcde"), 0, 0, 0, false, 0, 0, nil, 0, false, -1, 0, 0, 0); len(got) != 5 {
		t.Errorf("unlimited fill built %d cells, want 5", len(got))
	}
}

// The clip has to count columns the way the renderer counts them. vtui's
// UAX #29 segmentation splits an Indic virama sequence that the editor joins
// into one cluster of one column, so counting with it claimed columns the
// renderer would not paint: the clip came back short and the right of the
// viewport showed background where there was text.
func TestEditorRenderClip_CountsColumnsAsTheRendererDoes(t *testing.T) {
	ev := &EditorView{TabSize: 8}
	const width = 200
	line := strings.Repeat("क्क्क्क्क्क्ष ", 600)

	clipped := editorRenderClip(line, width)
	if cols := editorRenderColumns(clipped); cols < width {
		t.Errorf("the clip covers %d columns, short of the %d the viewport shows", cols, width)
	}

	// What the renderer then builds has to fill the viewport, which is the
	// property the clip exists to preserve.
	cells := ev.fillCellsWithLinks(nil, []byte(line), 0, 0, 0, false, 0, 0, nil, nil, 0, width, false, -1, 0, 0, 0)
	if len(cells) < width {
		t.Errorf("built %d cells for a %d column viewport; the rest of the row is left blank", len(cells), width)
	}
}

func TestEditorRenderClip(t *testing.T) {
	long := strings.Repeat("x", 50000)

	if got := editorRenderClip(long, 200); len(got) >= len(long) {
		t.Errorf("a 50 KB line was not clipped for a 200 column viewport: %d bytes", len(got))
	} else if got != long[:len(got)] {
		t.Error("the clip is not a prefix of the line")
	} else if cols := clusterColumnsForTest(got); cols < 200 {
		t.Errorf("the clip covers %d columns, short of the 200 asked for", cols)
	}

	// Nothing to gain, and nothing may be lost.
	if got := editorRenderClip("short line", 200); got != "short line" {
		t.Errorf("a short line was clipped to %q", got)
	}
	if got := editorRenderClip(long, 0); got != long {
		t.Error("an unlimited request clipped the line")
	}

	// A line laid out by the bidi algorithm is reordered as a whole: the
	// reordering of a prefix is not the prefix of the reordering, so it is
	// left alone however long it is.
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	defer func() { vtui.DefaultBidiMode = oldMode }()
	rtl := strings.Repeat("שלום ", 10000)
	if got := editorRenderClip(rtl, 200); got != rtl {
		t.Errorf("a right-to-left line was clipped to %d of %d bytes", len(got), len(rtl))
	}
}

func clusterColumnsForTest(s string) int {
	cols := 0
	vtui.ForEachCluster(s, func(_ string, width, _ int) { cols += width })
	return cols
}

// A tab is counted as one column, not its expansion, so the clip can only err
// towards taking more of the line than the viewport needs.
func TestEditorRenderColumns_CountsATabAsOneColumn(t *testing.T) {
	if got := editorRenderColumns("	a"); got != 2 {
		t.Errorf("columns of a tab and a letter = %d, want 2", got)
	}
	// A zero-width cluster occupies no column and must not be counted as one.
	if got := editorRenderColumns("ab"); got != 2 {
		t.Errorf("columns of two letters = %d, want 2", got)
	}
}

// The first guess at how many bytes cover the viewport is deliberately
// generous, but text that is mostly combining marks can still fall short of
// it, so the window doubles until the columns are covered.
func TestEditorRenderClip_GrowsTheWindowUntilTheColumnsAreCovered(t *testing.T) {
	const width = 64
	// Each visible letter carries five combining acutes: twenty-odd bytes per
	// column, far past the four the first guess allows for.
	line := strings.Repeat("á́́́́", 4000)

	clipped := editorRenderClip(line, width)
	if cols := editorRenderColumns(clipped); cols < width {
		t.Errorf("the clip covers %d columns, short of %d", cols, width)
	}
	if len(clipped) >= len(line) {
		t.Error("the whole line was taken when a prefix would do")
	}
}

// A line shorter than the viewport has nothing to clip, and a fragment the
// caller has already cut to size must come back whole.
func TestEditorRenderClip_LeavesWhatAlreadyFits(t *testing.T) {
	short := strings.Repeat("x", 40)
	if got := editorRenderClip(short, 200); got != short {
		t.Errorf("a line inside the viewport was clipped to %d bytes", len(got))
	}
}

// Text that never reaches the column budget however far the window grows --
// carriage returns, which the renderer drops rather than draws -- must come
// back whole rather than loop.
func TestEditorRenderClip_GivesUpOnTextWithNoColumns(t *testing.T) {
	line := strings.Repeat("", 15000)
	if got := editorRenderClip(line, 200); got != line {
		t.Errorf("a line with no columns was clipped to %d of %d bytes", len(got), len(line))
	}
}
