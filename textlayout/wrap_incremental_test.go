package textlayout

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"unicode/utf8"

	"github.com/unxed/f4/piecetable"
)

type measuredLayoutBuffer struct {
	data                  []byte
	reads, bytes, maxRead int
	lastEnd               int
}

func (b *measuredLayoutBuffer) Size() int { return len(b.data) }
func (b *measuredLayoutBuffer) Read(offset, length int) ([]byte, error) {
	b.reads++
	b.bytes += length
	b.maxRead = max(b.maxRead, length)
	b.lastEnd = max(b.lastEnd, offset+length)
	return b.data[offset:min(offset+length, len(b.data))], nil
}

func TestLongLineFirstViewportAndExtentAreIncremental(t *testing.T) {
	for _, newline := range []bool{false, true} {
		t.Run(fmt.Sprintf("newline=%v", newline), func(t *testing.T) {
			data := bytes.Repeat([]byte("a"), 200*1024)
			if newline {
				data = append(data, '\n')
			}
			buf := &measuredLayoutBuffer{data: data}
			pt := piecetable.NewWithBuffer(buf)
			li := piecetable.NewLineIndex()
			if newline {
				li.AppendOffsets([]int{len(data)}, len(data))
			}
			engine := NewWrapEngine(pt, li)
			engine.SetWidth(80)
			line, fragment := engine.GetLogLineAtVisualRow(23)
			if line != 0 || fragment != 23 {
				t.Fatalf("first viewport ends at (%d,%d)", line, fragment)
			}
			if got := engine.GetFragmentsThrough(0, 24); len(got) != 24 {
				t.Fatalf("first viewport laid out %d rows, want 24", len(got))
			}
			if rows, complete := engine.KnownVisualRows(); rows != 24 || complete {
				t.Fatalf("prefix advertised extent=(%d,%v)", rows, complete)
			}
			if buf.maxRead > layoutReadBytes || buf.bytes > 2*layoutReadBytes || buf.lastEnd > 2*layoutReadBytes {
				t.Fatalf("first viewport read the logical line: %+v", *buf)
			}
			for steps := 0; ; steps++ {
				before := engine.fragmentCache[0].pending
				oldOffset := 0
				if before != nil {
					oldOffset = before.offset
				}
				progressed, complete := engine.AdvanceVisualRows(64, 8192)
				if after := engine.fragmentCache[0].pending; after != nil && after.offset-oldOffset > 8192+utf8.UTFMax-1 {
					t.Fatalf("one batch scanned %d bytes", after.offset-oldOffset)
				}
				if complete {
					break
				}
				if !progressed || steps > 30 {
					t.Fatalf("extent stopped at step %d, error=%v", steps, engine.LastReadError())
				}
			}
			wantRows := 200 * 1024 / 80
			if newline {
				wantRows++
			}
			if rows, complete := engine.KnownVisualRows(); rows != wantRows || !complete {
				t.Fatalf("eventual extent=(%d,%v), want (%d,true)", rows, complete, wantRows)
			}
			for _, offset := range []int{65536, 90 * 1024, 200 * 1024} {
				row, column := engine.LogicalToVisual(offset)
				if got := engine.VisualToLogical(row, column); got != offset {
					t.Fatalf("source anchor %d -> (%d,%d) -> %d", offset, row, column, got)
				}
			}
		})
	}
}

func TestIncrementalWrapPreservesUTF8TabsWordsAndCRLFAcrossChunks(t *testing.T) {
	data := append(bytes.Repeat([]byte("a"), layoutReadBytes-1), []byte("世\tword more ")...)
	data = append(data, bytes.Repeat([]byte("é\tfoo bar\xff世"), 1200)...)
	data = append(data, '\r', '\n')
	makeEngine := func() *WrapEngine {
		pt := piecetable.New(data)
		li := piecetable.NewLineIndex()
		li.Rebuild(pt)
		engine := NewWrapEngine(pt, li)
		engine.SetWidth(11)
		engine.SetTabSize(4)
		return engine
	}
	full := makeEngine()
	want := append([]LineFragment(nil), full.GetFragments(0)...)
	for _, budget := range []int{1, 2, 3, 17, layoutReadBytes} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			engine := makeEngine()
			for step := 0; ; step++ {
				progressed, complete := engine.AdvanceVisualRows(2, budget)
				if complete {
					break
				}
				if !progressed || step > len(data)+2 {
					t.Fatalf("no progress at step %d: %v", step, engine.LastReadError())
				}
			}
			got := engine.GetFragments(0)
			if !reflect.DeepEqual(got, want) {
				t.Fatal("chunk boundary changed source fragments or logical tab/rune origins")
			}
			for _, frag := range got {
				if frag.ByteOffsetEnd > len(data)-2 {
					t.Fatal("CRLF became visible content")
				}
				column, runes := 0, 0
				for offset := frag.ByteOffsetStart; offset < frag.ByteOffsetEnd; {
					r, size := utf8.DecodeRune(data[offset:])
					column += engine.runeWidth(r, frag.VisualColumnStart+column)
					runes++
					offset += size
				}
				if column != frag.VisualWidth || runes != utf8.RuneCount(data[frag.ByteOffsetStart:frag.ByteOffsetEnd]) {
					t.Fatalf("invalid fragment metrics: %+v", frag)
				}
				row, col := engine.LogicalToVisual(frag.ByteOffsetStart)
				if engine.VisualToLogical(row, col) != frag.ByteOffsetStart {
					t.Fatalf("fragment anchor %d did not round trip", frag.ByteOffsetStart)
				}
			}
		})
	}
}

func TestUnindexedNewlineDoesNotCompleteTheLayout(t *testing.T) {
	pt := piecetable.New([]byte("first\nsecond\n"))
	li := piecetable.NewLineIndex()
	engine := NewWrapEngine(pt, li)
	if rows := engine.GetTotalVisualRows(); rows != 1 {
		t.Fatalf("unindexed prefix rows=%d", rows)
	}
	if _, complete := engine.KnownVisualRows(); complete {
		t.Fatal("an unindexed newline was treated as EOF")
	}
	if progress, _ := engine.AdvanceVisualRows(64, 65536); progress {
		t.Fatal("an unchanged incomplete index scheduled repeated work")
	}
	li.AppendOffsets([]int{6, 13}, pt.Size())
	engine.InvalidateFrom(0)
	if rows := engine.GetTotalVisualRows(); rows != 3 {
		t.Fatalf("completed index rows=%d", rows)
	}
}

func TestUnwrappedVisiblePrefixDoesNotScanWholeLine(t *testing.T) {
	data := append(bytes.Repeat([]byte("a"), 200*1024), '\n')
	buf := &measuredLayoutBuffer{data: data}
	li := piecetable.NewLineIndex()
	li.AppendOffsets([]int{len(data)}, len(data))
	engine := NewWrapEngine(piecetable.NewWithBuffer(buf), li)
	engine.ToggleWrap(false)
	fragments := engine.GetProjectionFragments(0, 1, 80)
	if len(fragments) != 1 || fragments[0].Loading || fragments[0].ByteOffsetEnd != 80 {
		t.Fatalf("visible prefix not ready: %+v", fragments)
	}
	if buf.bytes > layoutReadBytes || buf.maxRead > layoutReadBytes {
		t.Fatalf("first unwrapped viewport read %d bytes, max read %d", buf.bytes, buf.maxRead)
	}
	if engine.fragmentCache[0].complete() {
		t.Fatal("visible horizontal prefix became complete source layout")
	}
	if row, col := engine.LogicalToVisual(90 * 1024); row != 0 || col != 90*1024 {
		t.Fatalf("source anchor beyond 64KiB became (%d,%d)", row, col)
	}
	if offset := engine.VisualToLogical(0, 90*1024); offset != 90*1024 {
		t.Fatalf("horizontal hit beyond 64KiB became %d", offset)
	}
	full := engine.GetFragments(0)
	if len(full) != 1 || full[0].ByteOffsetEnd != 200*1024 || full[0].VisualWidth != 200*1024 {
		t.Fatalf("explicit full row was truncated: %+v", full)
	}
}
