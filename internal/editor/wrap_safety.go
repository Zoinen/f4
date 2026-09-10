package editor

import (
	"bytes"
	"context"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtui"
)

// Word wrapping turns one logical line into many visual fragments. Beyond
// this size the editor's text layout and highlighter deliberately stop
// expanding the line, so keeping wrapping enabled would make the view both
// misleading and unnecessarily expensive.
const maxWordWrapLineBytes = 64 * 1024

// scanEditorWrapSafety consumes one piece-table chunk. lineLen is the number
// of bytes already seen on the current logical line; it is returned so a
// caller can carry the value across chunk boundaries.
//
// NUL is included here because it is the editor's binary-data marker for all
// codepages after decoding. Files with a NUL late in the file can therefore
// still be protected even when the opening header looked like text.
func scanEditorWrapSafety(data []byte, lineLen int) (nextLineLen int, unsafe bool) {
	if bytes.IndexByte(data, 0) >= 0 {
		return lineLen, true
	}

	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			lineLen += len(data)
			return lineLen, lineLen > maxWordWrapLineBytes
		}
		if lineLen+idx > maxWordWrapLineBytes {
			return lineLen + idx, true
		}
		lineLen = 0
		data = data[idx+1:]
	}
	return lineLen, false
}

// editorWrapIntervalUnsafe checks a line when a remote VFS has supplied only
// its line-start offsets. The byte before the next offset is the newline, so
// it is not part of the logical line length.
func editorWrapIntervalUnsafe(lineStart, nextLineStart int64) bool {
	return nextLineStart > lineStart && nextLineStart-lineStart-1 > maxWordWrapLineBytes
}

// disableUnsafeWordWrap turns wrapping off for content it cannot lay out.
// It deliberately leaves wordWrapWanted alone: the user's choice for this file
// is still their choice, and the file may not contain the offending line the
// next time it is opened. Persisting the suppressed value instead would let a
// single overlong line erase a setting the user never changed.
func (ev *EditorView) DisableUnsafeWordWrap() {
	if ev.WordWrapSuppressed {
		return
	}
	ev.WordWrapSuppressed = true
	if ev.WordWrap {
		ev.WordWrap = false
		ev.ScrollLeft = 0
		ev.ClearCaches()
		ev.EnsureCursorVisible()
	}
	vtui.FrameManager.Redraw()
}

func (ev *EditorView) postUnsafeWordWrap(sessionID int, ctx context.Context) {
	vtui.FrameManager.PostTask(func() {
		if ctx.Err() != nil || ev.editSession != sessionID || ev.IsDone() {
			return
		}
		ev.DisableUnsafeWordWrap()
	})
}

// probeUnsafeWordWrap avoids painting the first screen with wrapping enabled
// when the beginning of the file already proves that wrapping is unsafe.
func (ev *EditorView) probeUnsafeWordWrap() bool {
	take := min(maxWordWrapLineBytes+1, ev.Pt.Size())
	if take == 0 {
		return false
	}
	data, ok := ev.Pt.View(0, take)
	if !ok {
		var err error
		data, err = ev.Pt.GetRange(0, take)
		if err != nil {
			return false
		}
	}
	_, unsafe := scanEditorWrapSafety(data, 0)
	if unsafe {
		ev.DisableUnsafeWordWrap()
	}
	return unsafe
}

// lineIndexStillGrowing reports whether the line index may yet gain lines:
// the buffer is one the background scan indexes, and that scan has not
// finished.
func (ev *EditorView) lineIndexStillGrowing() bool {
	return (ev.AsyncBuf != nil || ev.Mapped != nil) && !ev.IndexIsComplete()
}

func (ev *EditorView) CurrentLineUnsafeForWordWrap() bool {
	if ev.CursorLine < 0 || ev.CursorLine >= ev.Li.LineCount() {
		return false
	}
	// The last line of an index that is still filling may be an unindexed
	// prefix of the whole file rather than a line, so its apparent length is
	// not a real line length yet. That is true of every buffer the background
	// scan indexes — a mapped file starts with an empty index just as a lazy
	// buffer does, and a mapped file is how a local file opens by default —
	// so the question is whether the scan has finished, not how the bytes
	// arrive. A fully decoded file is left out: its index was built with it,
	// and it never reaches IndexComplete.
	if ev.lineIndexStillGrowing() && ev.CursorLine == ev.Li.LineCount()-1 {
		return false
	}
	lineLen := ev.GetLineLength(ev.CursorLine)
	if lineLen > maxWordWrapLineBytes {
		return true
	}
	start := ev.Li.GetLineOffset(ev.CursorLine)
	data, err := ev.Pt.GetRange(start, lineLen)
	return err == nil && bytes.IndexByte(data, 0) >= 0
}

// scanFullyReadForUnsafeWordWrap covers codepage-decoded files. Those files
// do not need a line-index scan, but they still need the same safety check as
// lazily loaded UTF-8 files before the user can enable wrapping.
//
// The table comes in as an argument rather than off ev: this runs on its own
// goroutine, and ev.Pt belongs to the UI thread, which replaces it whenever
// the text is set wholesale. Reading the field from here would race that
// assignment and, worse, could reach the replacement before it is built.
func (ev *EditorView) scanFullyReadForUnsafeWordWrap(ctx context.Context, sessionID int, Pt *piecetable.PieceTable) {
	const chunkSize = 256 * 1024
	lineLen := 0
	for pos := 0; pos < Pt.Size(); {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if ev.IsDone() {
			return
		}

		take := min(chunkSize, Pt.Size()-pos)
		data, ok := Pt.View(pos, take)
		if !ok {
			var err error
			data, err = Pt.GetRange(pos, take)
			if err != nil {
				if err == piecetable.ErrLoading {
					continue
				}
				return
			}
		}
		var unsafe bool
		lineLen, unsafe = scanEditorWrapSafety(data, lineLen)
		if unsafe {
			ev.postUnsafeWordWrap(sessionID, ctx)
			return
		}
		pos += len(data)
	}
}
