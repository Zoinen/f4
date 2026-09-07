package textlayout

import (
	"errors"
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/unxed/f4/piecetable"
)

func TestWrappedTabSourceCoordinatesRoundTrip(t *testing.T) {
	pt := piecetable.New([]byte("123\t"))
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	engine := NewWrapEngine(pt, li)
	engine.SetTabSize(4)
	engine.SetWidth(2)
	fragments := engine.GetFragments(0)
	if len(fragments) != 2 || fragments[1].VisualColumnStart != 2 || fragments[1].RuneStart != 2 {
		t.Fatalf("wrong source origins: %+v", fragments)
	}
	for offset := 0; offset <= pt.Size(); offset++ {
		row, col := engine.LogicalToVisual(offset)
		if got := engine.VisualToLogical(row, col); got != offset {
			t.Errorf("offset %d -> (%d,%d) -> %d", offset, row, col, got)
		}
	}
	if row, col := engine.LogicalToVisual(4); row != 1 || col != 2 {
		t.Fatalf("wrapped tab end=(%d,%d), want (1,2)", row, col)
	}
	if got := engine.FragmentColumnToLogical(fragments[1], 2); got != 4 {
		t.Fatalf("source-fragment hit returned %d, want 4", got)
	}
}

func TestProjectionSourceRoundTripsUnicodeAndInvalidBytes(t *testing.T) {
	for _, source := range []string{"ab世\tcé", "12\t3\t", "e\u0301\t世", "a\xff\xfe\tz", "abc def ghi"} {
		for _, width := range []int{1, 2, 4, 7} {
			for _, wrap := range []bool{false, true} {
				t.Run(fmt.Sprintf("%q/%d/%t", source, width, wrap), func(t *testing.T) {
					pt := piecetable.New([]byte(source))
					li := piecetable.NewLineIndex()
					li.Rebuild(pt)
					engine := NewWrapEngine(pt, li)
					engine.SetWidth(width)
					engine.SetTabSize(4)
					engine.ToggleWrap(wrap)
					for offset := 0; ; {
						row, col := engine.LogicalToVisual(offset)
						if got := engine.VisualToLogical(row, col); got != offset {
							t.Fatalf("offset %d -> (%d,%d) -> %d", offset, row, col, got)
						}
						if offset == len(source) {
							break
						}
						_, size := utf8.DecodeRuneInString(source[offset:])
						offset += size
					}
				})
			}
		}
	}
}

type delayedLayoutBuffer struct {
	data    []byte
	ready   bool
	reads   int
	failure error
}

func (b *delayedLayoutBuffer) Size() int { return len(b.data) }
func (b *delayedLayoutBuffer) Read(offset, length int) ([]byte, error) {
	b.reads++
	if b.failure != nil {
		return nil, b.failure
	}
	if !b.ready {
		return nil, piecetable.ErrLoading
	}
	return b.data[offset:min(offset+length, len(b.data))], nil
}

func TestLayoutReadFailureIsNotAnEndlessLoadingPlaceholder(t *testing.T) {
	failure := errors.New("source disconnected")
	buf := &delayedLayoutBuffer{data: []byte("abcdef"), failure: failure}
	engine := NewWrapEngine(piecetable.NewWithBuffer(buf), piecetable.NewLineIndex())
	engine.SetWidth(2)
	engine.GetLogLineAtVisualRow(2)
	if !errors.Is(engine.LastReadError(), failure) {
		t.Fatalf("lost permanent source error: %v", engine.LastReadError())
	}
	buf.failure, buf.ready = nil, true
	if got := engine.GetTotalVisualRows(); got != 3 {
		t.Fatalf("retry rows=%d", got)
	}
	if engine.LastReadError() != nil {
		t.Fatalf("successful data retained error: %v", engine.LastReadError())
	}
}

func TestLoadingFragmentsNeverBecomeAuthoritativeRowCounts(t *testing.T) {
	buf := &delayedLayoutBuffer{data: []byte("abcdefgh\nx\n")}
	pt := piecetable.NewWithBuffer(buf)
	li := piecetable.NewLineIndex()
	li.AppendOffsets([]int{9, 11}, len(buf.data))
	engine := NewWrapEngine(pt, li)
	engine.SetWidth(2)
	if got := engine.GetTotalVisualRows(); got != 0 {
		t.Fatalf("loading data became %d permanent rows", got)
	}
	if _, complete := engine.KnownVisualRows(); complete {
		t.Fatal("loading extent marked complete")
	}
	engine.GetLogLineAtVisualRow(100) // Must stop, not spin on missing data.
	buf.ready = true
	if got := engine.GetTotalVisualRows(); got != 6 {
		t.Fatalf("ready row count=%d, want 6", got)
	}
	if got := engine.GetRowOffset(1); got != 4 {
		t.Fatalf("next logical line kept loading-stub prefix %d, want 4", got)
	}
}

func TestVisualRowLookupDoesNotWrapOffscreenHundredLines(t *testing.T) {
	data := []byte("ab\ncd\nef\ngh\n")
	buf := &delayedLayoutBuffer{data: data, ready: true}
	pt := piecetable.NewWithBuffer(buf)
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	buf.reads = 0
	engine := NewWrapEngine(pt, li)
	engine.SetWidth(2)
	engine.GetLogLineAtVisualRow(0)
	if buf.reads != 1 {
		t.Fatalf("first row read %d logical lines, want only 1", buf.reads)
	}
	if rows, complete := engine.KnownVisualRows(); rows != 1 || complete {
		t.Fatalf("known extent=(%d,%v), want (1,false)", rows, complete)
	}
}
