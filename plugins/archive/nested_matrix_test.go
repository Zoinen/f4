package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"testing"

	"github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/zstd"
	"github.com/unxed/xz"
)

// nestedCodec is one compressed-TAR flavour of the nesting chain.
type nestedCodec struct {
	ext      string
	compress func(t *testing.T, tarData []byte) []byte
}

var nestedCodecs = []nestedCodec{
	{".tar.gz", func(t *testing.T, tarData []byte) []byte {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		mustWrite(t, zw, tarData)
		return buf.Bytes()
	}},
	{".tar.bz2", func(t *testing.T, tarData []byte) []byte {
		var buf bytes.Buffer
		zw, err := bzip2.NewWriter(&buf, &bzip2.WriterConfig{Level: 1})
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, zw, tarData)
		return buf.Bytes()
	}},
	{".tar.xz", func(t *testing.T, tarData []byte) []byte {
		var buf bytes.Buffer
		zw, err := xz.NewWriter(&buf)
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, zw, tarData)
		return buf.Bytes()
	}},
	{".tar.zst", func(t *testing.T, tarData []byte) []byte {
		var buf bytes.Buffer
		zw, err := zstd.NewWriter(&buf)
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, zw, tarData)
		return buf.Bytes()
	}},
}

type closeWriter interface {
	Write([]byte) (int, error)
	Close() error
}

func mustWrite(t *testing.T, w closeWriter, data []byte) {
	t.Helper()
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func plainTar(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The chain zip > tar.X > zip > tar.Y > file, for every pair of codecs, opens
// layer by layer through the reader-backed path: no layer is copied to a
// temporary file before the next one can read it (f4#1678).
func TestArchiveVFSReaderBackedNestedCodecMatrix(t *testing.T) {
	ctx := context.Background()
	leaf := []byte("leaf of a four-layer chain")
	for _, first := range nestedCodecs {
		for _, second := range nestedCodecs {
			t.Run(first.ext[1:]+" in zip, "+second.ext[1:]+" in zip", func(t *testing.T) {
				innermost := second.compress(t, plainTar(t, "leaf.txt", leaf))
				innerZip := zipBytes(t, "c"+second.ext, innermost)
				middle := first.compress(t, plainTar(t, "b.zip", innerZip))
				outer, outerPath := openOuterArchive(t, map[string][]byte{"a" + first.ext: middle})

				aPath := outer.Join(outerPath, "a"+first.ext)
				a, err := NewArchiveVFSContext(ctx, outer, aPath)
				if err != nil {
					t.Fatalf("open %s: %v", first.ext, err)
				}
				t.Cleanup(func() { _ = a.Close() })
				requireReaderBacked(t, a, "a"+first.ext)

				bPath := a.Join(aPath, "b.zip")
				b, err := NewArchiveVFSContext(ctx, a, bPath)
				if err != nil {
					t.Fatalf("open b.zip: %v", err)
				}
				t.Cleanup(func() { _ = b.Close() })
				requireReaderBacked(t, b, "b.zip")

				cPath := b.Join(bPath, "c"+second.ext)
				c, err := NewArchiveVFSContext(ctx, b, cPath)
				if err != nil {
					t.Fatalf("open c%s: %v", second.ext, err)
				}
				t.Cleanup(func() { _ = c.Close() })
				requireReaderBacked(t, c, "c"+second.ext)

				got := readArchiveMember(t, c, c.Join(cPath, "leaf.txt"))
				if !bytes.Equal(got, leaf) {
					t.Fatalf("leaf = %q, want %q", got, leaf)
				}
			})
		}
	}
}
