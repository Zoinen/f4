package fishplus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// DefaultChunkSize is how much a File asks for in one round trip. Chunks
// are aligned to their own size, which lets the remote dd position itself
// with a single lseek and read whole blocks even where GNU's
// iflag=skip_bytes is missing.
const DefaultChunkSize = 256 * 1024

// DefaultCachedChunks is how many chunks a File keeps around. Viewers jump
// back and forth over a few screens, so a handful of chunks removes most of
// the round trips at a bounded cost.
const DefaultCachedChunks = 8

// MaxReadLen caps a single read request, so a bad offset cannot make the
// remote host stream a whole disk into memory.
const MaxReadLen = 16 << 20

// ReadModes are the read backends the helper can drive, ordered by
// preference: byte exact dd, plain dd on block boundaries, tail piped into
// head, and finally cat, which can only ever deliver a whole file.
var ReadModes = []string{"ddbytes", "dd", "tailc", "cat"}

// ErrClosed is returned by a File that has already been closed.
var ErrClosed = errors.New("fishplus: file is closed")

// ReadMode returns the read backend the helper picked for itself,
// announced as "read:<n>" among the features.
func (f Features) ReadMode() string {
	for name := range f.names {
		if strings.HasPrefix(name, "read:") {
			return strings.TrimPrefix(name, "read:")
		}
	}
	return ""
}

// parseSizeLine picks the "S <size>" line out of a read reply. Anything else
// the remote tools may have printed is ignored.
func parseSizeLine(lines []string) (int64, error) {
	for _, line := range lines {
		if !strings.HasPrefix(line, "S ") {
			continue
		}
		size, err := strconv.ParseInt(strings.TrimSpace(line[2:]), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("fishplus: bad size line %q", line)
		}
		return size, nil
	}
	return 0, errors.New("fishplus: read reply without a size line")
}

// Read fetches a byte range and reports the size the remote host saw at that
// moment. A length of zero means "to the end of the file". The returned
// slice can be shorter than requested when the range runs past the end.
func (c *Client) Read(ctx context.Context, p string, off, length int64) ([]byte, int64, error) {
	if off < 0 || length < 0 {
		return nil, 0, fmt.Errorf("fishplus: read %q: bad range %d+%d", p, off, length)
	}
	if length > MaxReadLen {
		return nil, 0, fmt.Errorf("fishplus: read %q: length %d exceeds %d", p, length, MaxReadLen)
	}
	resp, err := c.sess.ExecPathData(ctx, "read", p,
		strconv.FormatInt(off, 10), strconv.FormatInt(length, 10))
	if err != nil {
		return nil, 0, err
	}
	if err := resp.Err("read " + p); err != nil {
		return nil, 0, err
	}
	size, err := parseSizeLine(resp.Lines)
	if err != nil {
		return nil, 0, err
	}
	return resp.Data, size, nil
}

// ReadFile fetches a whole file in chunks. It is meant for the small files a
// panel opens on the spot; anything large belongs in a File.
func (c *Client) ReadFile(ctx context.Context, p string) ([]byte, error) {
	var out []byte
	for {
		data, size, err := c.Read(ctx, p, int64(len(out)), DefaultChunkSize)
		if err != nil {
			return nil, err
		}
		out = append(out, data...)
		if int64(len(out)) >= size || len(data) == 0 {
			return out, nil
		}
	}
}

// SetReadMode forces a read backend instead of the one picked during the
// handshake. Meant for tests and for troubleshooting an odd host.
func (c *Client) SetReadMode(ctx context.Context, mode string) error {
	resp, err := c.sess.Exec(ctx, "rmode", mode)
	if err != nil {
		return err
	}
	return resp.Err("rmode " + mode)
}

// File is a read only handle on a remote file. It holds no state on the
// remote host: every miss turns into one ranged read, so a broken session
// costs nothing but the cache. The methods take a context because that is
// what f4's vfs.ReadAtCloser looks like.
type File struct {
	c    *Client
	path string

	mu        sync.Mutex
	size      int64
	pos       int64
	chunkSize int64
	maxChunks int
	cache     map[int64][]byte
	order     []int64
	closed    bool
}

// Open stats the file and returns a handle on it.
func (c *Client) Open(ctx context.Context, p string) (*File, error) {
	e, err := c.Stat(ctx, p)
	if err != nil {
		return nil, err
	}
	if e.IsDir() {
		return nil, fmt.Errorf("fishplus: %q is a directory", p)
	}
	return &File{
		c:         c,
		path:      p,
		size:      e.Size,
		chunkSize: DefaultChunkSize,
		maxChunks: DefaultCachedChunks,
		cache:     make(map[int64][]byte),
	}, nil
}

// SetChunkSize changes the transfer granularity. A size that is not a power
// of two still works, it just costs the remote dd its block alignment.
func (f *File) SetChunkSize(n int64) {
	if n <= 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chunkSize = n
	f.cache = make(map[int64][]byte)
	f.order = nil
}

// Size returns the last size the remote host reported.
func (f *File) Size() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.size
}

// Close drops the cache. There is nothing to release on the remote side.
func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.cache = nil
	f.order = nil
	return nil
}

// ReadAt fills p from the given offset, following the io.ReaderAt contract:
// a short read always comes with an error, io.EOF at the end of the file.
func (f *File) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readAt(ctx, p, off)
}

// Read continues where the previous Read stopped.
func (f *File) Read(ctx context.Context, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := f.readAt(ctx, p, f.pos)
	f.pos += int64(n)
	if n > 0 && err == io.EOF {
		return n, nil
	}
	return n, err
}

// Seek moves the offset used by Read. Whence follows io.Seeker.
func (f *File) Seek(off int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = off
	case io.SeekCurrent:
		abs = f.pos + off
	case io.SeekEnd:
		abs = f.size + off
	default:
		return 0, fmt.Errorf("fishplus: bad whence %d", whence)
	}
	if abs < 0 {
		return 0, fmt.Errorf("fishplus: negative offset %d", abs)
	}
	f.pos = abs
	return abs, nil
}

func (f *File) readAt(ctx context.Context, p []byte, off int64) (int, error) {
	if f.closed {
		return 0, ErrClosed
	}
	if off < 0 {
		return 0, fmt.Errorf("fishplus: negative offset %d", off)
	}
	total := 0
	for total < len(p) {
		at := off + int64(total)
		base := at - at%f.chunkSize
		chunk, err := f.chunkAt(ctx, base)
		if err != nil {
			return total, err
		}
		start := at - base
		if start >= int64(len(chunk)) {
			return total, io.EOF
		}
		total += copy(p[total:], chunk[start:])
	}
	return total, nil
}

func (f *File) chunkAt(ctx context.Context, base int64) ([]byte, error) {
	if chunk, ok := f.cache[base]; ok {
		return chunk, nil
	}
	data, size, err := f.c.Read(ctx, f.path, base, f.chunkSize)
	if err != nil {
		return nil, err
	}
	// The file may have grown or shrunk since Open, and a viewer following a
	// log file cares about that more than about the size stat once saw.
	f.size = size
	f.cache[base] = data
	f.order = append(f.order, base)
	for len(f.order) > f.maxChunks {
		delete(f.cache, f.order[0])
		f.order = f.order[1:]
	}
	return data, nil
}
