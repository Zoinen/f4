package main

import (
	"bytes"
	"context"

	"github.com/unxed/f4/piecetable"
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

func (ev *EditorView) disableUnsafeWordWrap() {
	if ev.wordWrapSuppressed {
		return
	}
	ev.wordWrapSuppressed = true
	if ev.WordWrap {
		ev.WordWrap = false
		ev.ScrollLeft = 0
		ev.clearCaches()
		ev.ensureCursorVisible()
	}
	vtui.FrameManager.Redraw()
}

func (ev *EditorView) postUnsafeWordWrap(sessionID int, ctx context.Context) {
	vtui.FrameManager.PostTask(func() {
		if ctx.Err() != nil || ev.editSession != sessionID || ev.IsDone() {
			return
		}
		ev.disableUnsafeWordWrap()
	})
}

// probeUnsafeWordWrap avoids painting the first screen with wrapping enabled
// when the beginning of the file already proves that wrapping is unsafe.
func (ev *EditorView) probeUnsafeWordWrap() bool {
	take := min(maxWordWrapLineBytes+1, ev.pt.Size())
	if take == 0 {
		return false
	}
	data, ok := ev.pt.View(0, take)
	if !ok {
		var err error
		data, err = ev.pt.GetRange(0, take)
		if err != nil {
			return false
		}
	}
	_, unsafe := scanEditorWrapSafety(data, 0)
	if unsafe {
		ev.disableUnsafeWordWrap()
	}
	return unsafe
}

func (ev *EditorView) currentLineUnsafeForWordWrap() bool {
	if ev.CursorLine < 0 || ev.CursorLine >= ev.li.LineCount() {
		return false
	}
	// The last line in a lazy buffer may still be an unindexed prefix of the
	// whole file. Its apparent length is therefore not a real line length yet.
	if ev.asyncBuf != nil && !ev.indexIsComplete() && ev.CursorLine == ev.li.LineCount()-1 {
		return false
	}
	lineLen := ev.getLineLength(ev.CursorLine)
	if lineLen > maxWordWrapLineBytes {
		return true
	}
	start := ev.li.GetLineOffset(ev.CursorLine)
	data, err := ev.pt.GetRange(start, lineLen)
	return err == nil && bytes.IndexByte(data, 0) >= 0
}

// scanFullyReadForUnsafeWordWrap covers codepage-decoded files. Those files
// do not need a line-index scan, but they still need the same safety check as
// lazily loaded UTF-8 files before the user can enable wrapping.
//
// The table comes in as an argument rather than off ev: this runs on its own
// goroutine, and ev.pt belongs to the UI thread, which replaces it whenever
// the text is set wholesale. Reading the field from here would race that
// assignment and, worse, could reach the replacement before it is built.
func (ev *EditorView) scanFullyReadForUnsafeWordWrap(ctx context.Context, sessionID int, pt *piecetable.PieceTable) {
	const chunkSize = 256 * 1024
	lineLen := 0
	for pos := 0; pos < pt.Size(); {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if ev.IsDone() {
			return
		}

		take := min(chunkSize, pt.Size()-pos)
		data, ok := pt.View(pos, take)
		if !ok {
			var err error
			data, err = pt.GetRange(pos, take)
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
