package fishplus

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// DefaultWriteChunk is how much of a file travels in one raw write request.
// It matches the read chunk, and being a power of two it lets the remote dd
// position itself on a block boundary.
const DefaultWriteChunk = 256 * 1024

// EncodedWriteChunk is the same for the base64 backend, where the remote
// shell reads the payload itself, one byte per read syscall: a smaller chunk
// keeps that loop short at the price of one more round trip.
const EncodedWriteChunk = 64 * 1024

// MaxWriteLen caps a single write request, so a runaway caller cannot make
// the remote host buffer a whole disk.
const MaxWriteLen = 16 << 20

// WriteModes are the write backends the helper can drive, ordered by
// preference: GNU dd stops after exactly n bytes while still reading whole
// blocks, base64 is exact by construction because the shell itself reads it,
// and dd with bs=1 is exact as well but pays a syscall per byte.
var WriteModes = []string{"ddbytes", "b64", "ddbs1"}

// ErrNoWrite is returned when the remote host announced no write backend at
// all. Sending the payload anyway would leave it in the stream and be read
// as the next request.
var ErrNoWrite = errors.New("fishplus: the remote host cannot write files")

// drainedLine is what the helper prints when it consumed the payload of a
// failed write for certain. Without it the two sides no longer agree on how
// much of the stream is gone, and the session cannot be trusted.
const drainedLine = "D"

// WriteMode returns the write backend the helper picked for itself,
// announced as "write:<name>" among the features.
func (f Features) WriteMode() string {
	for name := range f.names {
		if strings.HasPrefix(name, "write:") {
			return strings.TrimPrefix(name, "write:")
		}
	}
	return ""
}

// WriteMode returns the backend the client currently encodes its payload
// for, which is the announced one until wmode changes it.
func (c *Client) WriteMode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeMode
}

// WriteChunk is the payload size the current backend is happiest with.
func (c *Client) WriteChunk() int {
	if c.WriteMode() == "b64" {
		return EncodedWriteChunk
	}
	return DefaultWriteChunk
}

// SetWriteMode forces a write backend instead of the one picked during the
// handshake. Meant for tests and for troubleshooting an odd host.
func (c *Client) SetWriteMode(ctx context.Context, mode string) error {
	resp, err := c.sess.Exec(ctx, "wmode", mode)
	if err != nil {
		return err
	}
	if err := resp.Err("wmode " + mode); err != nil {
		return err
	}
	c.mu.Lock()
	c.writeMode = mode
	c.mu.Unlock()
	return nil
}

// Write puts data at the given offset of a remote file, creating the file if
// it is not there and leaving whatever follows the written range alone. A
// gap between the end of the file and the offset becomes a hole.
func (c *Client) Write(ctx context.Context, p string, off int64, data []byte) error {
	if off < 0 {
		return fmt.Errorf("fishplus: write %q: negative offset %d", p, off)
	}
	if len(data) > MaxWriteLen {
		return fmt.Errorf("fishplus: write %q: length %d exceeds %d", p, len(data), MaxWriteLen)
	}
	mode := c.WriteMode()
	if mode == "" {
		return ErrNoWrite
	}
	encoded := mode == "b64"
	enc := "raw"
	if encoded {
		enc = "b64"
	}
	args := []string{strconv.FormatInt(off, 10), strconv.Itoa(len(data)), enc}
	resp, err := c.sess.ExecPayload(ctx, "write", []string{p}, args, data, encoded)
	if err != nil {
		return err
	}
	if resp.OK() {
		return nil
	}
	// A write that failed after the remote host started reading the payload
	// leaves an unknown number of bytes in the stream, and the next request
	// would be parsed out of the middle of a file. The helper says so by
	// staying silent about having drained it.
	if !hasLine(resp.Lines, drainedLine) {
		c.sess.MarkBroken()
	}
	return resp.Err("write " + p)
}

func hasLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

// Truncate sets the size of a file, creating it when it does not exist. Size
// zero is a plain shell redirection and works on every host; any other size
// needs the truncate utility, which not every host has.
func (c *Client) Truncate(ctx context.Context, p string, size int64) error {
	if size < 0 {
		return fmt.Errorf("fishplus: truncate %q: negative size %d", p, size)
	}
	resp, err := c.sess.ExecPath(ctx, "trunc", p, strconv.FormatInt(size, 10))
	if err != nil {
		return err
	}
	return resp.Err("trunc " + p)
}

// WriteFile replaces the contents of a file, in as many round trips as the
// current backend's chunk size calls for.
func (c *Client) WriteFile(ctx context.Context, p string, data []byte) error {
	w, err := c.Create(ctx, p)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// Writer is a sequential write handle on a remote file. It buffers up to one
// chunk so that a caller writing a line at a time does not turn into a round
// trip per line, and it holds its context because f4's vfs.VFS hands out a
// plain io.WriteCloser.
type Writer struct {
	c    *Client
	ctx  context.Context
	path string

	mu     sync.Mutex
	off    int64
	buf    []byte
	chunk  int
	closed bool
}

// Create truncates a file, or creates it, and returns a handle that appends
// from the beginning.
func (c *Client) Create(ctx context.Context, p string) (*Writer, error) {
	if c.WriteMode() == "" {
		return nil, ErrNoWrite
	}
	if err := c.Truncate(ctx, p, 0); err != nil {
		return nil, err
	}
	return c.OpenWriter(ctx, p, 0)
}

// OpenWriter returns a handle that writes from the given offset onwards,
// leaving the rest of the file as it is.
func (c *Client) OpenWriter(ctx context.Context, p string, off int64) (*Writer, error) {
	if c.WriteMode() == "" {
		return nil, ErrNoWrite
	}
	if off < 0 {
		return nil, fmt.Errorf("fishplus: open %q for writing: negative offset %d", p, off)
	}
	return &Writer{c: c, ctx: ctx, path: p, off: off, chunk: c.WriteChunk()}, nil
}

// Offset reports where the next flushed byte lands, buffered bytes included.
func (w *Writer) Offset() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.off + int64(len(w.buf))
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, ErrClosed
	}
	w.buf = append(w.buf, p...)
	for len(w.buf) >= w.chunk {
		if err := w.flush(w.chunk); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush sends whatever is buffered, so a caller that wants its data on the
// remote host now does not have to close the handle for it.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrClosed
	}
	return w.flush(len(w.buf))
}

// flush sends the first n buffered bytes. The offset only moves once the
// remote host confirmed the write, so a retry after a failure would not
// leave a hole.
func (w *Writer) flush(n int) error {
	if n == 0 {
		return nil
	}
	if err := w.c.Write(w.ctx, w.path, w.off, w.buf[:n]); err != nil {
		return err
	}
	w.off += int64(n)
	w.buf = append(w.buf[:0], w.buf[n:]...)
	return nil
}

// Close flushes the rest of the buffer. Closing twice is harmless, which is
// what lets a caller close on the error path as well.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	err := w.flush(len(w.buf))
	w.closed = true
	w.buf = nil
	return err
}
