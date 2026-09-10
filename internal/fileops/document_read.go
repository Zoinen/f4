package fileops

import (
	"context"
	"github.com/unxed/f4/vfs"
	"io"
)

func ReadDocumentBytes(ctx context.Context, file vfs.ReadAtCloser, dst []byte, off int64) (int, error) {
	total := 0
	for total < len(dst) {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		// Direct-local operations remain bounded even for explicit long reads.
		end := min(len(dst), total+256*1024)
		n, err := file.ReadAt(ctx, dst[total:end], off+int64(total))
		if n < 0 || n > len(dst)-total {
			return total, io.ErrUnexpectedEOF
		}
		total += n
		if err != nil && !(err == io.EOF && total == len(dst)) {
			return total, err
		}
		if n == 0 && total < len(dst) {
			return total, io.ErrNoProgress
		}
	}
	return total, nil
}
