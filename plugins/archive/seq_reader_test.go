package archive

import (
	"bytes"
	"io"
	"io/fs"
	"testing"
)

// countingFile is a member that can only be read forward, like a compressed
// TAR entry, and counts how many times it was opened.
type countingFile struct {
	r *bytes.Reader
}

func (f *countingFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *countingFile) Close() error               { return nil }
func (f *countingFile) Stat() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

type seekableFile struct{ *bytes.Reader }

func (f seekableFile) Close() error               { return nil }
func (f seekableFile) Stat() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

func TestSeqMemberReaderContinuesForwardAndReopensBackward(t *testing.T) {
	data := make([]byte, 1000)
	for i := range data {
		data[i] = byte(i)
	}
	opens := 0
	r := &seqMemberReader{}
	r.setOpen(func() (fs.File, error) {
		opens++
		return &countingFile{r: bytes.NewReader(data)}, nil
	})
	t.Cleanup(r.Close)

	read := func(off int64, n int) []byte {
		t.Helper()
		p := make([]byte, n)
		got, err := r.ReadAt(p, off)
		if err != nil {
			t.Fatalf("ReadAt(%d, %d): %v", off, n, err)
		}
		return p[:got]
	}

	// Chunked in-order reads, then one that skips ahead: a single open.
	for off := int64(0); off < 300; off += 100 {
		if got := read(off, 100); !bytes.Equal(got, data[off:off+100]) {
			t.Fatalf("chunk at %d differs", off)
		}
	}
	if got := read(700, 100); !bytes.Equal(got, data[700:800]) {
		t.Fatal("read after a skip differs")
	}
	if opens != 1 {
		t.Fatalf("member opened %d times for forward reads, want 1", opens)
	}

	// Going back reopens once and is still right.
	if got := read(50, 20); !bytes.Equal(got, data[50:70]) {
		t.Fatal("read after going back differs")
	}
	if opens != 2 {
		t.Fatalf("member opened %d times after a backward read, want 2", opens)
	}

	// The end of the member reads short with io.EOF, and a later read still works.
	p := make([]byte, 100)
	n, err := r.ReadAt(p, 950)
	if n != 50 || err != io.EOF {
		t.Fatalf("ReadAt at the tail = %d, %v; want 50, EOF", n, err)
	}
	if got := read(0, 10); !bytes.Equal(got, data[:10]) {
		t.Fatal("read after EOF differs")
	}
}

func TestSeqMemberReaderDoesNotKeepSeekableHandles(t *testing.T) {
	data := []byte("0123456789")
	opens := 0
	r := &seqMemberReader{}
	r.setOpen(func() (fs.File, error) {
		opens++
		return seekableFile{bytes.NewReader(data)}, nil
	})
	for _, off := range []int64{0, 5, 2} {
		p := make([]byte, 3)
		if _, err := r.ReadAt(p, off); err != nil || !bytes.Equal(p, data[off:off+3]) {
			t.Fatalf("ReadAt(%d) = %q, %v", off, p, err)
		}
	}
	if opens != 3 {
		t.Fatalf("seekable member opened %d times, want one per read", opens)
	}
	if r.file != nil {
		t.Fatal("a seekable handle was kept")
	}
}

func TestSeqMemberReaderOpenAndSkipErrors(t *testing.T) {
	r := &seqMemberReader{}
	r.setOpen(func() (fs.File, error) { return nil, fs.ErrNotExist })
	if _, err := r.ReadAt(make([]byte, 1), 0); err != fs.ErrNotExist {
		t.Fatalf("open error = %v, want ErrNotExist", err)
	}

	short := &seqMemberReader{}
	short.setOpen(func() (fs.File, error) { return &countingFile{r: bytes.NewReader([]byte("abc"))}, nil })
	if _, err := short.ReadAt(make([]byte, 1), 10); err == nil {
		t.Fatal("reading past the end of a short member succeeded")
	}
}
