package piecetable

import (
	"strings"
	"sync"
	"testing"
)

// TestPieceTable_ReadsWhileEditing is the race this lock exists for: the editor
// scans the buffer from background goroutines — a search assembling what it
// scans, the line indexer walking the text — while the UI thread edits the same
// table. Run this with -race; without the lock it reports a write to pt.pieces
// racing with the reads below.
func TestPieceTable_ReadsWhileEditing(t *testing.T) {
	pt := New([]byte(strings.Repeat("a line of text\n", 2000)))

	var wg sync.WaitGroup
	done := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				size := pt.Size()
				if size > 32 {
					_, _ = pt.GetRange(size/2, 16)
					_, _ = pt.View(size/2, 16)
				}
				_ = pt.GetState()
				_ = pt.ForEachRange(func([]byte) error { return nil })
			}
		}()
	}

	for i := 0; i < 300; i++ {
		pt.Insert(10, []byte("edit "))
		if pt.Size() > 100 {
			pt.Delete(20, 5)
		}
	}
	close(done)
	wg.Wait()

	if pt.Size() <= 0 {
		t.Fatalf("size = %d after the edits", pt.Size())
	}
}
