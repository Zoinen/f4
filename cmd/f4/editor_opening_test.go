package main

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

// Keep the remainder unavailable so initial-frame assertions cannot pass just
// because an asynchronous read happened to finish before the first render.
type editorOpeningFile struct {
	readCount atomic.Int32
}

func (*editorOpeningFile) Size() int64  { return 16 * 1024 * 1024 }
func (*editorOpeningFile) Close() error { return nil }
func (f *editorOpeningFile) Read(ctx context.Context, p []byte) (int, error) {
	return f.ReadAt(ctx, p, 0)
}
func (f *editorOpeningFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	f.readCount.Add(1)
	if off+int64(len(p)) > 32*1024 {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	for i := range p {
		p[i] = 'x'
		if (off+int64(i)+1)%64 == 0 {
			p[i] = '\n'
		}
	}
	return len(p), nil
}

func TestEditorFirstWindowUsesAvailablePrefix(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	file := &editorOpeningFile{}
	pt, buf := newUTF8EditorPieceTable(file, file.Size(), nil, 0)
	defer buf.Close()
	editor := newEditorView(pt, nil, "", false, true)
	defer editor.Close()
	editor.highlighter = nil
	editor.SetPosition(0, 0, 119, 40)

	window := editor.semanticWindow()
	if len(window.rows) < 40 {
		t.Fatalf("initial window has %d rows; available prefix was not indexed", len(window.rows))
	}
	want := bytes.Repeat([]byte{'x'}, 63)
	for _, row := range window.rows[:40] {
		if !bytes.Equal(bytes.TrimSpace([]byte(row.Text)), want) {
			t.Fatalf("initial row %d is not ready: %q", row.VisualRow, row.Text)
		}
	}
	if editor.semanticExtentKnown {
		t.Fatal("partial line index advertised a complete content extent")
	}
	if count := file.readCount.Load(); count != 1 {
		t.Fatalf("initial window issued %d reads, want only the existing first-block read", count)
	}
}

func TestEditorPrefixRestoresReadySavedPositionImmediately(t *testing.T) {
	for _, tc := range []struct {
		name    string
		line    int
		pending bool
	}{
		{"ready-line", 3, false},
		{"incomplete-last-line", 512, true},
		{"beyond-prefix", 700, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			file := &editorOpeningFile{}
			pt, buf := newUTF8EditorPieceTable(file, file.Size(), nil, 0)
			editor := newEditorView(pt, nil, "", false, true)
			editor.asyncBuf = buf
			defer func() {
				editor.Close()
				collectQueuedTasks(50 * time.Millisecond)
			}()
			editor.highlighter = nil
			editor.SetPosition(0, 0, 119, 40)
			editor.SetVisible(true)
			editor.targetLine = tc.line
			editor.targetPos = 1
			editor.targetTopRow = 0
			editor.targetLeft = 0

			editor.StartIndexing()
			if pending := editor.targetLine != -1; pending != tc.pending {
				t.Fatalf("saved position pending = %v, want %v", pending, tc.pending)
			}
			if tc.pending {
				return
			}
			if editor.CursorLine != tc.line || editor.CursorPos != 1 {
				t.Fatalf("cursor = %d:%d, want %d:1", editor.CursorLine, editor.CursorPos, tc.line)
			}
			rows := semanticStyledEditorWindowRows(editor, editor.semanticWindow(), 120)
			for _, row := range rows[:40] {
				var text strings.Builder
				for _, run := range row.Runs {
					text.WriteString(run.Text)
				}
				if !strings.Contains(text.String(), "xxxxxxxx") || strings.Contains(text.String(), "Loading") {
					t.Fatalf("saved-position first frame has an unready row: %q", text.String())
				}
			}
		})
	}
}
