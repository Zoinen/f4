package editor

import (
	context "context"
	piecetable "github.com/unxed/f4/internal/piecetable"
	vfs "github.com/unxed/f4/vfs"
)

func NewUTF8PieceTable(f vfs.ReadAtCloser, size int64, prefix []byte, offsets ...int64) (*piecetable.PieceTable, *AsyncBuffer) {
	pt, buf, _ := NewUTF8PieceTableContext(context.Background(), f, size, prefix, offsets...)
	return pt, buf
}

func NewUTF8PieceTableContext(ctx context.Context, f vfs.ReadAtCloser, size int64, prefix []byte, offsets ...int64) (*piecetable.PieceTable, *AsyncBuffer, error) {
	var dataOffset int64
	if len(offsets) > 0 {
		dataOffset = offsets[0]
	}
	// For small files the encoding probe already contains every byte. Feeding
	// those bytes directly to the piece table avoids reading the file again and
	// starting an indexer whose first and final UI updates would repaint the
	// editor immediately after it was shown.
	logicalSize := size - dataOffset
	if logicalSize < 0 {
		logicalSize = 0
	}
	logicalPrefix := prefix
	if dataOffset > 0 {
		if dataOffset <= int64(len(prefix)) {
			logicalPrefix = prefix[dataOffset:]
		} else {
			logicalPrefix = nil
		}
	}
	if int64(len(logicalPrefix)) == logicalSize {
		return piecetable.New(logicalPrefix), nil, nil
	}

	buf := NewAsyncBufferWithOffset(context.Background(), f, dataOffset)
	buf.SeedPrefix(logicalPrefix)
	if _, err := buf.ReadContext(ctx, 0, min(buf.ChunkSize, int(logicalSize))); err != nil {
		buf.Close()
		return nil, nil, err
	}
	return piecetable.NewWithBuffer(buf), buf, nil
}
