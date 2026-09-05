package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// openLocalEditor opens path the way showEditor does for a local UTF-8 file:
// lazily, through an AsyncBuffer, so the save path sees the piece table it
// sees in production.
func openLocalEditor(t *testing.T, dir, path string) *EditorView {
	t.Helper()

	v := vfs.NewOSVFS(dir)
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	buf := NewAsyncBuffer(context.Background(), f)
	buf.prewarm()

	ev := NewEditorView(piecetable.NewWithBuffer(buf), v, path)
	ev.file = f
	ev.asyncBuf = buf
	ev.Codepage = 65001
	return ev
}

// TestEditorSave_InPlacePatchDoesNotRenameAMissingStage covers the save that
// OSVFS.PatchInPlace accepts: it writes through to the file itself, so there is
// no staged sibling to rename into place. The finalize step used to try anyway
// and reported "Failed to finalize save (rename failed): ... no such file or
// directory" — with the new content already on disk, so the file was correct
// and the editor still believed the save had failed.
//
// PatchInPlace accepts exactly the edits that leave every unchanged piece at
// its original offset, which is why this needs a same-length replacement: an
// insertion shifts them and falls back to the staged path.
func TestEditorSave_InPlacePatchDoesNotRenameAMissingStage(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "aa.txt")
	if err := os.WriteFile(path, []byte("hello world\nsecond line\n"), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openLocalEditor(t, dir, path)
	defer ev.Close()

	ev.replaceRange(0, 5, []byte("HELLO"))
	ev.modified = true

	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	drainPendingTasks()

	// The content alone proves nothing here: PatchInPlace writes it before the
	// finalize step runs, so it is correct either way. Going clean is what says
	// the save actually completed.
	if ev.modified {
		t.Error("editor still marked modified: the save reported failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "HELLO world\nsecond line\n" {
		t.Errorf("content = %q, want %q", got, "HELLO world\nsecond line\n")
	}
	assertNoEditorTempSiblings(t, path)
}

// TestEditorSave_UnmodifiedBufferCompletes is the same trap reached the
// shortest way: an untouched buffer is one original piece at offset zero, which
// PatchInPlace accepts trivially.
func TestEditorSave_UnmodifiedBufferCompletes(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "untouched.txt")
	content := strings.Repeat("line of text\n", 100)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openLocalEditor(t, dir, path)
	defer ev.Close()

	ev.modified = true // F2 on a buffer whose bytes happen to be unchanged
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	drainPendingTasks()

	if ev.modified {
		t.Error("editor still marked modified: the save reported failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != content {
		t.Errorf("content changed: got %d bytes, want %d", len(got), len(content))
	}
	assertNoEditorTempSiblings(t, path)
}

// TestEditorSave_InsertionStillStages keeps the other half honest: an edit that
// shifts offsets is refused by PatchInPlace, so the save must still go through
// a staged sibling and rename it into place.
func TestEditorSave_InsertionStillStages(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "grown.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openLocalEditor(t, dir, path)
	defer ev.Close()

	ev.insertTextAtCursor([]byte("XYZ"))
	ev.modified = true

	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	drainPendingTasks()

	if ev.modified {
		t.Error("editor still marked modified: the save reported failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "XYZhello world\n" {
		t.Errorf("content = %q, want %q", got, "XYZhello world\n")
	}
	assertNoEditorTempSiblings(t, path)
}

func TestEditorSave_ShrinkingEditStillTruncates(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "shrunk.txt")
	if err := os.WriteFile(path, []byte("aGVsbG8gd29ybGQ=\n"), 0600); err != nil {
		t.Fatal(err)
	}

	ev := openLocalEditor(t, dir, path)
	defer ev.Close()

	selectEditorBytes(ev, len("aGVsbG8gd29ybGQ=\n"))
	if err := ev.transformBase64Selection(false); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	drainPendingTasks()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", got, "hello world")
	}
}
