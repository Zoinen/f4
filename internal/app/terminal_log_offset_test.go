package app

import (
	"context"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"testing"
)

type offsetLogVFS struct {
	vfs.VFS
	offset int64
}

func (v offsetLogVFS) InitialOffset() (int64, bool) { return v.offset, true }
func TestDocumentOpeningRestoresLogViewport(t *testing.T) {
	data := []byte("first row\nsecond row\nthird row\n")
	offset := int64(len("first row\n"))
	v := offsetLogVFS{terminal.NewTerminalLogVFS(nil, func() []byte { return data }), offset}
	// The viewer consumes the exact wrapped-fragment byte boundary.
	viewer, err := viewer.NewViewerView(context.Background(), v, "Terminal Log")
	if err != nil {
		t.Fatalf("create terminal-log viewer: %v", err)
	}
	defer viewer.Backend.Close()
	if !applyInitialViewerOffset(viewer) {
		t.Fatal("viewer did not accept the terminal snapshot offset")
	}
	if viewer.TopOffset != offset {
		t.Fatalf("viewer top offset=%d, want %d", viewer.TopOffset, offset)
	}

	// The editor restores both its cursor byte and the visual top row from the
	// same boundary. Its piece table shares the immutable snapshot bytes.
	editor := editor.NewEditorView(piecetable.New(data), v, "Terminal Log")
	editor.WordWrap = true
	editor.SetPosition(0, 0, 39, 10)
	if !applyInitialEditorOffset(editor, v) {
		t.Fatal("editor did not accept the terminal snapshot offset")
	}
	wantTop, _ := editor.Engine.LogicalToVisual(int(offset))
	editor.StartIndexing()
	gotOffset := editor.Li.GetLineOffset(editor.CursorLine) + editor.CursorPos
	if gotOffset != int(offset) {
		t.Fatalf("editor cursor offset=%d, want %d", gotOffset, offset)
	}
	if editor.ScrollTopRow != wantTop {
		t.Fatalf("editor top row=%d, want %d", editor.ScrollTopRow, wantTop)
	}
}
