package fileops

import (
	"context"
	"io"

	"github.com/unxed/f4/vfs"
)

// readFailure and writeFailure tell the caller of pumpFile which side of the
// copy failed, because "Ignore read errors" and "Ignore write errors" are two
// separate choices. Both unwrap to the error as the VFS returned it.
type readFailure struct{ err error }

func (e *readFailure) Error() string { return e.err.Error() }
func (e *readFailure) Unwrap() error { return e.err }

type writeFailure struct{ err error }

func (e *writeFailure) Error() string { return e.err.Error() }
func (e *writeFailure) Unwrap() error { return e.err }

// pumpFile copies src into dst through buf.
//
// A failed read is tried again only when the operation ignores read errors.
// A Read that failed leaves the stream position undefined, so the retry and
// every read after it address the file by offset. ReadAttempts counts failures
// in a row: a read that returns data starts the count again, so one bad spot
// does not use up the attempts of the next.
func pumpFile(ctx context.Context, state *FileOpState, src vfs.ReadAtCloser, dst io.Writer, buf []byte, name string) error {
	attempts := 1
	if state != nil && state.IgnoreReadErrors {
		attempts = max(state.ReadAttempts, 1)
	}
	var offset int64
	positional := false
	failures := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var n int
		var rerr error
		if positional {
			n, rerr = src.ReadAt(ctx, buf, offset)
		} else {
			n, rerr = src.Read(ctx, buf)
		}
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return &writeFailure{werr}
			}
			offset += int64(n)
			failures = 0
			if state != nil && state.OnBytes != nil {
				state.OnBytes(n)
			}
		}
		if rerr == nil {
			if !positional || n > 0 {
				continue
			}
			// ReaderAt promises an error with every short read; a VFS that
			// breaks the promise must not spin here forever.
			rerr = io.ErrNoProgress
		}
		if rerr == io.EOF {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		failures++
		if attempts <= 1 || failures >= attempts || OperationMustNotRetry(rerr) {
			return &readFailure{rerr}
		}
		state.note("RETRY    %s: read at offset %d failed (attempt %d of %d): %v", name, offset, failures, attempts, rerr)
		positional = true
	}
}
