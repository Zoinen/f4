package archive

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"sync"

	"github.com/unxed/tar"
)

// gzipTarView is the decompressed bytes of a compressed TAR (gzip, zstd) as an
// archives.ReaderAtSeeker. The random-access reader behind it (tar.GzipReaderAt,
// tar.ZstdReaderAt) keeps checkpoints or frame boundaries, so ReadAt and Seek
// anywhere cost at most one checkpoint interval or frame; the tar reader on top
// skips over file data by seeking, so opening one member of a large tar.gz or
// tar.zst no longer decompresses everything before it every time (f4#1678).
type gzipTarView struct {
	g    decompressedReader
	size int64

	mu  sync.Mutex
	pos int64
}

func (v *gzipTarView) Read(p []byte) (int, error) {
	v.mu.Lock()
	pos := v.pos
	v.mu.Unlock()
	n, err := v.g.ReadAt(p, pos)
	v.mu.Lock()
	v.pos = pos + int64(n)
	v.mu.Unlock()
	if n > 0 && err == io.EOF {
		err = nil
	}
	return n, err
}

func (v *gzipTarView) ReadAt(p []byte, off int64) (int, error) { return v.g.ReadAt(p, off) }

func (v *gzipTarView) Seek(offset int64, whence int) (int64, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = v.pos
	case io.SeekEnd:
		base = v.size
	default:
		return 0, errors.New("invalid seek mode")
	}
	next := base + offset
	if next < 0 {
		return 0, errors.New("negative archive position")
	}
	v.pos = next
	return next, nil
}

func (v *gzipTarView) Close() error { return v.g.Close() }

// decompressedReader is what tar.GzipReaderAt and tar.ZstdReaderAt provide.
type decompressedReader interface {
	io.ReaderAt
	io.Closer
	Size() int64
}

// openGzipTarView returns the view and the name to open it under (the
// display name without its gzip suffix), or nil when source is not a gzip whose
// content is a TAR, in which case the caller reads it the ordinary way. It
// scans the stream to the end once, to learn the exact size and to lay the
// checkpoints down; a listing has to read all of it anyway.
func openGzipTarView(ctx context.Context, source io.ReaderAt, size int64, displayName string) (*gzipTarView, string) {
	if size < 18 {
		return nil, ""
	}
	var magic [2]byte
	if _, err := source.ReadAt(magic[:], 0); err != nil || magic[0] != 0x1f || magic[1] != 0x8b {
		return nil, ""
	}
	g, err := tar.NewGzipReaderAt(source, size, 0)
	if err != nil {
		return nil, ""
	}
	return finishTarView(ctx, g, displayName)
}

// openBzipTarView is openGzipTarView for a bzip2'd TAR. The blocks are found by
// scanning the stream once, and each is decoded on its own.
func openBzipTarView(ctx context.Context, source io.ReaderAt, size int64, displayName string) (*gzipTarView, string) {
	if size < 14 {
		return nil, ""
	}
	var magic [3]byte
	if _, err := source.ReadAt(magic[:], 0); err != nil || magic != [3]byte{'B', 'Z', 'h'} {
		return nil, ""
	}
	z, err := tar.NewBzipReaderAt(source, size)
	if err != nil {
		return nil, ""
	}
	return finishTarView(ctx, z, displayName)
}

// openXzTarView is openGzipTarView for an xz'd TAR. The size is known from the
// file's index, so a file of several blocks is entered at the block a read
// falls in.
func openXzTarView(ctx context.Context, source io.ReaderAt, size int64, displayName string) (*gzipTarView, string) {
	if size < 32 {
		return nil, ""
	}
	var magic [6]byte
	if _, err := source.ReadAt(magic[:], 0); err != nil || magic != [6]byte{0xfd, '7', 'z', 'X', 'Z', 0} {
		return nil, ""
	}
	x, err := tar.NewXzReaderAt(source, size)
	if err != nil {
		return nil, ""
	}
	return finishTarView(ctx, x, displayName)
}

// openZstdTarView is openGzipTarView for a zstd'd TAR.
func openZstdTarView(ctx context.Context, source io.ReaderAt, size int64, displayName string) (*gzipTarView, string) {
	if size < 8 {
		return nil, ""
	}
	var magic [4]byte
	if _, err := source.ReadAt(magic[:], 0); err != nil || magic != [4]byte{0x28, 0xb5, 0x2f, 0xfd} {
		return nil, ""
	}
	z, err := tar.NewZstdReaderAt(source, size)
	if err != nil {
		return nil, ""
	}
	return finishTarView(ctx, z, displayName)
}

// finishTarView checks that the decompressed data starts like a TAR and runs
// the stream to its end once, which teaches the reader the exact size.
func finishTarView(ctx context.Context, g decompressedReader, displayName string) (*gzipTarView, string) {
	var head [512]byte
	if n, err := g.ReadAt(head[:], 0); n < len(head) || (err != nil && err != io.EOF) || string(head[257:262]) != "ustar" {
		_ = g.Close()
		return nil, ""
	}
	// Reading at the far end runs the scan to the end.
	var probe [1]byte
	if _, err := g.ReadAt(probe[:], math.MaxInt64-1); err != io.EOF || ctx.Err() != nil {
		_ = g.Close()
		return nil, ""
	}
	total := g.Size()
	if total < 0 {
		_ = g.Close()
		return nil, ""
	}
	return &gzipTarView{g: g, size: total}, tarNameOf(displayName)
}

// tarNameOf maps a gzip'd tar's name to the name of the tar inside it.
func tarNameOf(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tgz"):
		return name[:len(name)-4] + ".tar"
	case strings.HasSuffix(lower, ".tar.gz"):
		return name[:len(name)-3]
	case strings.HasSuffix(lower, ".tar.zst"):
		return name[:len(name)-4]
	case strings.HasSuffix(lower, ".tzst"):
		return name[:len(name)-5] + ".tar"
	case strings.HasSuffix(lower, ".tar.xz"):
		return name[:len(name)-3]
	case strings.HasSuffix(lower, ".txz"):
		return name[:len(name)-4] + ".tar"
	case strings.HasSuffix(lower, ".xz"):
		return name[:len(name)-3] + ".tar"
	case strings.HasSuffix(lower, ".tar.bz2"):
		return name[:len(name)-4]
	case strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".tbz"):
		return name[:strings.LastIndex(name, ".")] + ".tar"
	case strings.HasSuffix(lower, ".bz2"):
		return name[:len(name)-4] + ".tar"
	case strings.HasSuffix(lower, ".gz"):
		return name[:len(name)-3] + ".tar"
	case strings.HasSuffix(lower, ".zst"):
		return name[:len(name)-4] + ".tar"
	}
	return name + ".tar"
}
