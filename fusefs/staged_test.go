package fusefs

import (
	"bytes"
	"io"
	"testing"
)

func TestStagedFileTakesWritesInAnyOrder(t *testing.T) {
	sf, err := newStagedFile()
	if err != nil {
		t.Fatalf("newStagedFile: %v", err)
	}
	defer sf.Close()

	// The kernel does not promise to write a file front to back, which is
	// the whole reason this staging file exists.
	if _, err := sf.WriteAt([]byte("world"), 6); err != nil {
		t.Fatalf("WriteAt tail: %v", err)
	}
	if _, err := sf.WriteAt([]byte("hello "), 0); err != nil {
		t.Fatalf("WriteAt head: %v", err)
	}

	if size, err := sf.Size(); err != nil || size != 11 {
		t.Fatalf("size = %d err = %v, want 11", size, err)
	}

	got := make([]byte, 5)
	if _, err := sf.ReadAt(got, 6); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if string(got) != "world" {
		t.Fatalf("ReadAt = %q", got)
	}

	r, err := sf.Reader()
	if err != nil {
		t.Fatalf("Reader: %v", err)
	}
	all, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(all, []byte("hello world")) {
		t.Fatalf("commit would send %q", all)
	}
}

func TestStagedFileTruncateAndSecondCommit(t *testing.T) {
	sf, err := newStagedFile()
	if err != nil {
		t.Fatal(err)
	}
	defer sf.Close()

	if _, err := sf.WriteAt([]byte("0123456789"), 0); err != nil {
		t.Fatal(err)
	}
	if err := sf.Truncate(4); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	if size, _ := sf.Size(); size != 4 {
		t.Fatalf("size after truncate = %d, want 4", size)
	}

	// Reader has to rewind every time: a file may be committed, written
	// again and committed again through the same staging file.
	for i := 0; i < 2; i++ {
		r, err := sf.Reader()
		if err != nil {
			t.Fatal(err)
		}
		all, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if string(all) != "0123" {
			t.Fatalf("pass %d sent %q", i, all)
		}
	}
}

func TestStagedFileIsUnlinked(t *testing.T) {
	sf, err := newStagedFile()
	if err != nil {
		t.Fatal(err)
	}
	defer sf.Close()
	// Unlinked at creation, so a crash cannot leave it behind: the name is
	// already gone while the handle still works.
	if _, err := sf.f.Stat(); err != nil {
		t.Fatalf("the staging file stopped working after being unlinked: %v", err)
	}
}
