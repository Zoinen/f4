package fusefs

import (
	"sync"
	"testing"
)

func newWriterTestBridge() *bridge {
	return &bridge{writers: make(map[string]*writeHandle)}
}

func TestWriterTableSharesOneHandlePerPath(t *testing.T) {
	b := newWriterTestBridge()

	first, created := b.acquireWriter("/a.txt")
	if !created {
		t.Fatal("the first writer of a file has to be told it is the first")
	}
	second, created := b.acquireWriter("/a.txt")
	if created {
		t.Fatal("a second writer of the same file must not be told to stage its own copy")
	}
	if first != second {
		t.Fatal("two writers of one file got two handles; the last close would silently win")
	}
	if got := b.openWriters(); got != 1 {
		t.Fatalf("openWriters = %d, want 1", got)
	}

	if last := b.releaseWriter(second); last {
		t.Fatal("committing while another writer is still open would truncate its work")
	}
	if b.writerFor("/a.txt") == nil {
		t.Fatal("the handle disappeared while a writer still held it")
	}
	if last := b.releaseWriter(first); !last {
		t.Fatal("the last release has to report itself, or the file is never committed")
	}
	if b.writerFor("/a.txt") != nil || b.openWriters() != 0 {
		t.Fatal("the handle outlived its last writer")
	}
}

func TestWriterTableKeepsPathsApart(t *testing.T) {
	b := newWriterTestBridge()
	a, _ := b.acquireWriter("/a.txt")
	c, _ := b.acquireWriter("/b.txt")
	if a == c {
		t.Fatal("two different files share one handle")
	}
	if b.openWriters() != 2 {
		t.Fatalf("openWriters = %d, want 2", b.openWriters())
	}
	b.releaseWriter(a)
	if b.writerFor("/b.txt") == nil {
		t.Fatal("closing one file took the other one down with it")
	}
}

func TestWriterTableUnderConcurrentOpens(t *testing.T) {
	b := newWriterTestBridge()
	const n = 64
	var wg sync.WaitGroup
	handles := make([]*writeHandle, n)
	creations := make([]bool, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			handles[i], creations[i] = b.acquireWriter("/busy.bin")
		}(i)
	}
	wg.Wait()

	created := 0
	for i := range creations {
		if creations[i] {
			created++
		}
		if handles[i] != handles[0] {
			t.Fatal("concurrent opens produced more than one handle for one file")
		}
	}
	if created != 1 {
		t.Fatalf("%d openers were told to stage a copy, want exactly 1", created)
	}

	for i := 0; i < n; i++ {
		last := b.releaseWriter(handles[i])
		if last != (i == n-1) {
			t.Fatalf("release %d reported last=%v", i, last)
		}
	}
	if b.openWriters() != 0 {
		t.Fatal("the table did not empty")
	}
}
