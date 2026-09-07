package textlayout

import (
	"errors"
	"testing"

	"github.com/unxed/f4/piecetable"
)

type mappingSource struct {
	data      []byte
	available int
	err       error
}

func (b *mappingSource) Size() int { return len(b.data) }
func (b *mappingSource) Read(off, length int) ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	if off+length > b.available {
		return nil, piecetable.ErrLoading
	}
	return b.data[off : off+length], nil
}

func TestOffsetMappingWaitsForPrecedingRowsAndReportsFailure(t *testing.T) {
	b := &mappingSource{data: []byte("abcdefgh\nxyz"), available: 0}
	pt := piecetable.NewWithBuffer(b)
	li := piecetable.NewLineIndex()
	li.AppendOffsets([]int{9}, pt.Size())
	engine := NewWrapEngine(pt, li)
	engine.SetWidth(2)
	if got := engine.AdvanceToOffset(10, 64); got.Ready || got.Err != nil {
		t.Fatalf("pending mapping: %+v", got)
	}
	failure := errors.New("read failed")
	b.err = failure
	if got := engine.AdvanceToOffset(10, 64); got.Ready || !errors.Is(got.Err, failure) {
		t.Fatalf("failed mapping: %+v", got)
	}
	b.err = nil
	b.available = len(b.data)
	if got := engine.AdvanceToOffset(10, 64); !got.Ready || got.Row != 4 || got.Err != nil {
		t.Fatalf("ready mapping: %+v", got)
	}
}

func TestOffsetMappingRequiresFollowingFragmentAtWrapBoundary(t *testing.T) {
	pt := piecetable.New([]byte("abcdefgh"))
	li := piecetable.NewLineIndex()
	engine := NewWrapEngine(pt, li)
	engine.SetWidth(4)
	if got := engine.AdvanceToOffset(4, 4); got.Ready {
		t.Fatalf("unfinished next fragment mapped as previous: %+v", got)
	}
	if got := engine.AdvanceToOffset(4, 8); !got.Ready || got.Row != 1 {
		t.Fatalf("boundary mapping: %+v", got)
	}
	if got := engine.AdvanceToOffset(8, 8); !got.Ready || got.Row != 1 {
		t.Fatalf("EOF mapping: %+v", got)
	}
}
