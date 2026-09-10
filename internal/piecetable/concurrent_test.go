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
	Pt := New([]byte(strings.Repeat("a line of text\n", 2000)))

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
				size := Pt.Size()
				if size > 32 {
					_, _ = Pt.GetRange(size/2, 16)
					_, _ = Pt.View(size/2, 16)
				}
				_ = Pt.GetState()
				_ = Pt.ForEachRange(func([]byte) error { return nil })
			}
		}()
	}

	for i := 0; i < 300; i++ {
		Pt.Insert(10, []byte("edit "))
		if Pt.Size() > 100 {
			Pt.Delete(20, 5)
		}
	}
	close(done)
	wg.Wait()

	if Pt.Size() <= 0 {
		t.Fatalf("size = %d after the edits", Pt.Size())
	}
}
