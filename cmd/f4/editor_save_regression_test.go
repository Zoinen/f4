package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestEditorRegularFileSaveFinalizesAndPreservesContent(t *testing.T) {
	for _, change := range []string{"unchanged", "insert", "shorten"} {
		t.Run(change, func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			dir := t.TempDir()
			path := filepath.Join(dir, "sample.txt")
			source := []byte("original body\nsecond line")
			if err := os.WriteFile(path, source, 0600); err != nil {
				t.Fatal(err)
			}
			filesystem := vfs.NewOSVFS(dir)
			file, err := filesystem.Open(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			pt := piecetable.New(source)
			editor := NewEditorView(pt, filesystem, path)
			defer editor.Close()
			editor.file = file
			editor.Codepage = 65001
			switch change {
			case "insert":
				pt.Insert(0, []byte("new prefix:"))
			case "shorten":
				pt.Delete(pt.Size()-5, 5)
			}
			want := pt.String()
			editor.modified = true
			saved := false
			editor.SaveToFile(func() { saved = true })
			waitEditorSave(t, editor)
			if !saved || editor.modified {
				t.Fatalf("save did not finalize: callback=%v modified=%v", saved, editor.modified)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != want {
				t.Fatalf("saved data=%q, error=%v; want %q", got, err, want)
			}
			if got := waitPtString(t, editor.pt); got != want {
				t.Fatalf("reopened data=%q; want %q", got, want)
			}
			assertNoEditorTempSiblings(t, path)
		})
	}
}

func TestEditorInPlaceSaveDoesNotRenameNonexistentStage(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	base := newEditorCloudSaveVFS([]byte("original"))
	filesystem := &editorInPlaceSaveVFS{editorCloudSaveVFS: base}
	editor := NewEditorView(piecetable.New([]byte("original")), filesystem, base.original)
	defer editor.Close()
	editor.Codepage = 65001
	editor.modified = true
	saved := false
	editor.SaveToFile(func() { saved = true })
	waitEditorSave(t, editor)
	if !saved || editor.modified || len(base.renamed) != 0 || filesystem.calls != 1 {
		t.Fatalf("in-place save: callback=%v modified=%v renames=%v calls=%v",
			saved, editor.modified, base.renamed, filesystem.calls)
	}
}

func TestRegularFileInPlacePatchDeclinesBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")
	const original = "original contents"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	err := vfs.NewOSVFS(dir).PatchInPlace(context.Background(), path, []vfs.PatchPiece{
		{Data: []byte("prefix"), Length: 6},
		{Offset: 0, Length: int64(len(original))},
	})
	if err == nil {
		t.Fatal("regular file must use a staged save")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != original {
		t.Fatalf("declined patch changed its source: %q, %v", got, err)
	}
}

type editorInPlaceSaveVFS struct {
	*editorCloudSaveVFS
	calls int
}

func (filesystem *editorInPlaceSaveVFS) PatchInPlace(context.Context, string, []vfs.PatchPiece) error {
	filesystem.calls++
	return nil
}
