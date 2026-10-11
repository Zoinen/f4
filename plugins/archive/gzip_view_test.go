package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/zstd"
	"github.com/unxed/xz"
)

// A many-member tar.gz inside a zip is read through the checkpointed view:
// members near the end, near the start and in the middle all come out right
// and out of order (f4#1678).
func TestArchiveVFSNestedTarGzipMembersOutOfOrder(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewSource(5)) // #nosec G404 -- a fixed seed makes the test data reproducible; no security decision uses it.
	const members = 24
	contents := make([][]byte, members)
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for i := range contents {
		size := 200<<10 + rng.Intn(400<<10)
		b := make([]byte, size)
		for j := range b {
			b[j] = "abcdefgh\n"[rng.Intn(9)]
		}
		contents[i] = b
		if err := tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("dir/file%02d.txt", i), Mode: 0o600, Size: int64(size)}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var packed bytes.Buffer
	zw := gzip.NewWriter(&packed)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	outer, outerPath := openOuterArchive(t, map[string][]byte{"big.tar.gz": packed.Bytes()})
	bigPath := outer.Join(outerPath, "big.tar.gz")
	big, err := NewArchiveVFSContext(ctx, outer, bigPath)
	if err != nil {
		t.Fatalf("open tar.gz inside zip: %v", err)
	}
	t.Cleanup(func() { _ = big.Close() })
	requireReaderBacked(t, big, "big.tar.gz")

	for _, i := range []int{members - 1, 3, 12, 0, members - 2, 7} {
		member := big.Join(bigPath, fmt.Sprintf("dir/file%02d.txt", i))
		if got := readArchiveMember(t, big, member); !bytes.Equal(got, contents[i]) {
			t.Fatalf("member %d differs (%d bytes, want %d)", i, len(got), len(contents[i]))
		}
	}
}

func TestTarNameOf(t *testing.T) {
	for in, want := range map[string]string{
		"a.tar.gz": "a.tar", "A.TAR.GZ": "A.TAR", "a.tgz": "a.tar", "a.gz": "a.tar", "a.bin": "a.bin.tar",
		"a.tar.zst": "a.tar", "a.tzst": "a.tar", "a.zst": "a.tar",
		"a.tar.bz2": "a.tar", "a.tbz2": "a.tar", "a.tbz": "a.tar", "a.bz2": "a.tar",
		"a.tar.xz": "a.tar", "a.txz": "a.tar", "a.xz": "a.tar",
	} {
		if got := tarNameOf(in); got != want {
			t.Errorf("tarNameOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenGzipTarViewDeclinesOtherData(t *testing.T) {
	ctx := context.Background()
	for name, data := range map[string][]byte{
		"tiny":     []byte("x"),
		"not gzip": bytes.Repeat([]byte("plain text, not a gzip file "), 20),
		"gzip of text": func() []byte {
			var b bytes.Buffer
			zw := gzip.NewWriter(&b)
			_, _ = zw.Write(bytes.Repeat([]byte("just text, no tar header here "), 40))
			_ = zw.Close()
			return b.Bytes()
		}(),
	} {
		if view, _ := openGzipTarView(ctx, bytes.NewReader(data), int64(len(data)), "x.gz"); view != nil {
			t.Errorf("%s: a view was opened", name)
		}
	}
}

// The same view serves a zstd'd TAR of several frames.
func TestArchiveVFSNestedTarZstdMembersOutOfOrder(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewSource(11)) // #nosec G404 -- a fixed seed makes the test data reproducible; no security decision uses it.
	const members = 20
	contents := make([][]byte, members)
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for i := range contents {
		b := make([]byte, 150<<10+rng.Intn(300<<10))
		for j := range b {
			b[j] = "abcdefgh\n"[rng.Intn(9)]
		}
		contents[i] = b
		if err := tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("d/f%02d.txt", i), Mode: 0o600, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	// One frame per 512 KiB of the tar.
	var packed bytes.Buffer
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	for data := raw.Bytes(); len(data) > 0; {
		n := 512 << 10
		if n > len(data) {
			n = len(data)
		}
		packed.Write(enc.EncodeAll(data[:n], nil))
		data = data[n:]
	}

	outer, outerPath := openOuterArchive(t, map[string][]byte{"big.tar.zst": packed.Bytes()})
	bigPath := outer.Join(outerPath, "big.tar.zst")
	big, err := NewArchiveVFSContext(ctx, outer, bigPath)
	if err != nil {
		t.Fatalf("open tar.zst inside zip: %v", err)
	}
	t.Cleanup(func() { _ = big.Close() })
	requireReaderBacked(t, big, "big.tar.zst")
	for _, i := range []int{members - 1, 2, 9, 0, members - 3} {
		member := big.Join(bigPath, fmt.Sprintf("d/f%02d.txt", i))
		if got := readArchiveMember(t, big, member); !bytes.Equal(got, contents[i]) {
			t.Fatalf("member %d differs (%d bytes, want %d)", i, len(got), len(contents[i]))
		}
	}
}

// The same view serves an xz'd TAR of several blocks.
func TestArchiveVFSNestedTarXzMembersOutOfOrder(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewSource(13)) // #nosec G404 -- a fixed seed makes the test data reproducible; no security decision uses it.
	const members = 16
	contents := make([][]byte, members)
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for i := range contents {
		b := make([]byte, 150<<10+rng.Intn(300<<10))
		for j := range b {
			b[j] = "abcdefgh\n"[rng.Intn(9)]
		}
		contents[i] = b
		if err := tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("d/f%02d.txt", i), Mode: 0o600, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var packed bytes.Buffer
	cfg := xz.WriterConfig{BlockSize: 512 << 10}
	xw, err := cfg.NewWriter(&packed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := xw.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := xw.Close(); err != nil {
		t.Fatal(err)
	}

	outer, outerPath := openOuterArchive(t, map[string][]byte{"big.tar.xz": packed.Bytes()})
	bigPath := outer.Join(outerPath, "big.tar.xz")
	big, err := NewArchiveVFSContext(ctx, outer, bigPath)
	if err != nil {
		t.Fatalf("open tar.xz inside zip: %v", err)
	}
	t.Cleanup(func() { _ = big.Close() })
	requireReaderBacked(t, big, "big.tar.xz")
	for _, i := range []int{members - 1, 2, 9, 0, members - 3} {
		member := big.Join(bigPath, fmt.Sprintf("d/f%02d.txt", i))
		if got := readArchiveMember(t, big, member); !bytes.Equal(got, contents[i]) {
			t.Fatalf("member %d differs (%d bytes, want %d)", i, len(got), len(contents[i]))
		}
	}
}

// The same view serves a bzip2'd TAR of many blocks.
func TestArchiveVFSNestedTarBzipMembersOutOfOrder(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewSource(17)) // #nosec G404 -- a fixed seed makes the test data reproducible; no security decision uses it.
	const members = 14
	contents := make([][]byte, members)
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for i := range contents {
		b := make([]byte, 150<<10+rng.Intn(300<<10))
		for j := range b {
			b[j] = "abcdefgh\n"[rng.Intn(9)]
		}
		contents[i] = b
		if err := tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("d/f%02d.txt", i), Mode: 0o600, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var packed bytes.Buffer
	bw, err := bzip2.NewWriter(&packed, &bzip2.WriterConfig{Level: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bw.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := bw.Close(); err != nil {
		t.Fatal(err)
	}

	outer, outerPath := openOuterArchive(t, map[string][]byte{"big.tar.bz2": packed.Bytes()})
	bigPath := outer.Join(outerPath, "big.tar.bz2")
	big, err := NewArchiveVFSContext(ctx, outer, bigPath)
	if err != nil {
		t.Fatalf("open tar.bz2 inside zip: %v", err)
	}
	t.Cleanup(func() { _ = big.Close() })
	requireReaderBacked(t, big, "big.tar.bz2")
	for _, i := range []int{members - 1, 2, 9, 0, members - 3} {
		member := big.Join(bigPath, fmt.Sprintf("d/f%02d.txt", i))
		if got := readArchiveMember(t, big, member); !bytes.Equal(got, contents[i]) {
			t.Fatalf("member %d differs (%d bytes, want %d)", i, len(got), len(contents[i]))
		}
	}
}

// Data that is not the format a view is for is declined, whichever view is
// asked, so the caller falls back to the generic path.
func TestCompressedTarViewsDeclineOtherData(t *testing.T) {
	ctx := context.Background()
	text := bytes.Repeat([]byte("plain text, not any archive at all "), 20)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(bytes.Repeat([]byte("just text, no tar header here "), 40))
	_ = zw.Close()

	opens := map[string]func(context.Context, io.ReaderAt, int64, string) (*gzipTarView, string){
		"gzip": openGzipTarView, "zstd": openZstdTarView, "xz": openXzTarView, "bzip2": openBzipTarView,
	}
	inputs := map[string][]byte{
		"tiny":             []byte("x"),
		"plain text":       text,
		"gzip of text":     gz.Bytes(),
		"zstd magic only":  append([]byte{0x28, 0xb5, 0x2f, 0xfd}, text...),
		"xz magic only":    append([]byte{0xfd, '7', 'z', 'X', 'Z', 0}, text...),
		"bzip2 magic only": append([]byte("BZh9"), text...),
	}
	for viewName, open := range opens {
		for inName, data := range inputs {
			if view, _ := open(ctx, bytes.NewReader(data), int64(len(data)), "x.bin"); view != nil {
				t.Errorf("%s view opened %s", viewName, inName)
			}
		}
	}
}
