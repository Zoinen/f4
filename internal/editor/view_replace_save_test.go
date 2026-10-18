package editor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestEditorReplaceAllThenSave(t *testing.T) {
	for _, backing := range []string{"chunks", "mapped"} {
		t.Run(backing, func(t *testing.T) {
			for _, tc := range []struct {
				name, pattern, replacement string
				regexp                     bool
			}{
				{name: "ssh path", pattern: `C:\Users\xs\.ssh\`, replacement: "/Users/zoin/.ssh/"},
				{name: "same length", pattern: "old", replacement: "new"},
				{name: "shorter", pattern: "old", replacement: "x"},
				{name: "longer", pattern: "old", replacement: "replacement"},
				{name: "delete", pattern: "old", replacement: ""},
				{name: "regex", pattern: "old", replacement: "new", regexp: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					testReplaceAllSave(t, backing, tc.pattern, tc.replacement, tc.regexp)
				})
			}
		})
	}
}

func testReplaceAllSave(t *testing.T, backing, pattern, replacement string, regexp bool) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	original := "Host test\n    IdentityFile " + pattern + "test_key\n" + pattern + "\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	var ev *EditorView
	if backing == "mapped" {
		if !CanMemoryMapFile(context.Background(), vfs.NewOSVFS(dir), path) {
			t.Skip("memory mapping is disabled for this filesystem/platform")
		}
		ev = openMappedEditor(t, dir, path)
	} else {
		ev = openLocalEditor(t, dir, path)
	}
	defer ev.Close()

	ev.Replace(pattern, replacement, true, false, regexp, false, true)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !ev.Modified {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline.C:
			t.Fatal("timeout waiting for Replace All")
		}
	}
	want := strings.ReplaceAll(original, pattern, replacement)
	if got, err := ev.Pt.Bytes(); err != nil || string(got) != want {
		t.Fatalf("replacement buffer = %q, %v; want %q", got, err, want)
	}
	// Whole-buffer replacement must remain one undoable edit, including any
	// pieces that originally pointed into the file rather than the add buffer.
	ev.Undo()
	if got, err := ev.Pt.Bytes(); err != nil || string(got) != original {
		t.Fatalf("undo buffer = %q, %v; want %q", got, err, original)
	}
	ev.Redo()
	if got, err := ev.Pt.Bytes(); err != nil || string(got) != want {
		t.Fatalf("redo buffer = %q, %v; want %q", got, err, want)
	}

	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("saved content = %q, want %q", got, want)
	}
	if ev.Modified {
		t.Fatal("editor still modified after save")
	}
	if got, err := ev.Pt.Bytes(); err != nil || string(got) != want {
		t.Fatalf("reloaded buffer = %q, %v; want %q", got, err, want)
	}
	assertNoEditorTempSiblings(t, path)
}

func TestEditorSetTextPreservesUndoBuffers(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	ev := NewEditorView(piecetable.New([]byte("original")), nil, "")
	defer ev.Close()
	ev.InsertTextAtCursor([]byte("added "))
	for _, text := range []string{"replacement", "", "another replacement"} {
		ev.saveUndo(opOther)
		ev.SetText(text)
		pieces, ok := patchPiecesFromTable(ev.Pt)
		if !ok {
			t.Fatal("replacement could not be described as save patches")
		}
		if got := applyPieces([]byte("original"), pieces); string(got) != text {
			t.Fatalf("save patches produce %q, want %q", got, text)
		}
	}
	for _, want := range []string{"", "replacement", "added original", "original"} {
		ev.Undo()
		if got, err := ev.Pt.Bytes(); err != nil || string(got) != want {
			t.Fatalf("undo buffer = %q, %v; want %q", got, err, want)
		}
	}
	for _, want := range []string{"added original", "replacement", "", "another replacement"} {
		ev.Redo()
		if got, err := ev.Pt.Bytes(); err != nil || string(got) != want {
			t.Fatalf("redo buffer = %q, %v; want %q", got, err, want)
		}
	}
}
