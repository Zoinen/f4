package afcproto

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"sync"
)

// File is an AFC file handle. Methods on one File are serialized so ReadAt can
// temporarily seek the remote handle without disturbing sequential reads.
type File struct {
	client *Client
	path   string
	handle uint64

	mu     sync.Mutex
	offset int64
	size   int64
	closed bool
}

func (f *File) Size() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.size
}

func (f *File) Read(p []byte) (int, error) {
	return f.ReadContext(context.Background(), p)
}

func (f *File) ReadContext(ctx context.Context, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkOpen("read"); err != nil {
		return 0, err
	}
	n, err := f.readChunk(ctx, p)
	f.offset += int64(n)
	return n, err
}

func (f *File) ReadAt(p []byte, off int64) (int, error) {
	return f.ReadAtContext(context.Background(), p, off)
}

func (f *File) ReadAtContext(ctx context.Context, p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, &fs.PathError{Op: "readat", Path: f.path, Err: fs.ErrInvalid}
	}
	if len(p) == 0 {
		return 0, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkOpen("readat"); err != nil {
		return 0, err
	}

	original := f.offset
	if off != original {
		if err := f.seek(ctx, off); err != nil {
			return 0, err
		}
	}

	n := 0
	var readErr error
	for n < len(p) {
		chunk := p[n:]
		if len(chunk) > maxIOChunk {
			chunk = chunk[:maxIOChunk]
		}
		var got int
		got, readErr = f.readChunk(ctx, chunk)
		n += got
		if readErr != nil {
			break
		}
		if got == 0 {
			readErr = io.EOF
			break
		}
	}

	if err := f.seek(ctx, original); err != nil {
		// The remote cursor can no longer be represented truthfully. Discard the
		// connection so a pool cannot hand the desynchronised handle to a caller.
		f.client.poison(err)
		if readErr == nil {
			readErr = err
		}
	}
	if readErr == nil && n < len(p) {
		readErr = io.EOF
	}
	return n, readErr
}

func (f *File) Write(p []byte) (int, error) {
	return f.WriteContext(context.Background(), p)
}

func (f *File) WriteContext(ctx context.Context, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkOpen("write"); err != nil {
		return 0, err
	}
	written := 0
	for written < len(p) {
		chunk := p[written:]
		if len(chunk) > maxIOChunk {
			chunk = chunk[:maxIOChunk]
		}
		_, err := f.client.exchange(ctx, opFileWrite, putUint64s(f.handle), chunk, opStatus)
		if err != nil {
			return written, pathError("write", f.path, err)
		}
		written += len(chunk)
		f.offset += int64(len(chunk))
		if f.offset > f.size {
			f.size = f.offset
		}
	}
	return written, nil
}

func (f *File) Truncate(size int64) error {
	return f.TruncateContext(context.Background(), size)
}

func (f *File) TruncateContext(ctx context.Context, size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.checkOpen("truncate"); err != nil {
		return err
	}
	if size < 0 {
		return pathError("truncate", f.path, fs.ErrInvalid)
	}
	_, err := f.client.exchange(ctx, opFileSetSize, putUint64s(f.handle, uint64(size)), nil, opStatus)
	if err != nil {
		return pathError("truncate", f.path, err)
	}
	f.size = size
	return nil
}

func (f *File) Close() error {
	return f.CloseContext(context.Background())
}

func (f *File) CloseContext(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	_, err := f.client.exchange(ctx, opFileClose, putUint64s(f.handle), nil, opStatus)
	return pathError("close", f.path, err)
}

func (f *File) readChunk(ctx context.Context, p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	want := len(p)
	if want > maxIOChunk {
		want = maxIOChunk
	}
	resp, err := f.client.exchange(ctx, opFileRead, putUint64s(f.handle, uint64(want)), nil, opData)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return 0, io.EOF
		}
		return 0, pathError("read", f.path, err)
	}
	if len(resp.payload) > want {
		return 0, pathError("read", f.path, f.client.protocolFailure(ErrProtocol))
	}
	n := copy(p, resp.payload)
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (f *File) seek(ctx context.Context, offset int64) error {
	if offset < 0 {
		return pathError("seek", f.path, fs.ErrInvalid)
	}
	// #nosec G115 -- negative offsets are rejected immediately above.
	_, err := f.client.exchange(ctx, opFileSeek, putUint64s(f.handle, uint64(io.SeekStart), uint64(offset)), nil, opStatus)
	return pathError("seek", f.path, err)
}

func (f *File) checkOpen(operation string) error {
	if f.closed {
		return pathError(operation, f.path, fs.ErrClosed)
	}
	return nil
}

var (
	_ io.Reader   = (*File)(nil)
	_ io.ReaderAt = (*File)(nil)
	_ io.Writer   = (*File)(nil)
	_ io.Closer   = (*File)(nil)
)
