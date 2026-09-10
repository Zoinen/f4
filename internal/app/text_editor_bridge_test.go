package app

import (
	"context"
	"errors"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type blockingTextEditorStatVFS struct {
	vfs.VFS
	release <-chan struct{}
	called  chan<- struct{}
}

func (filesystem *blockingTextEditorStatVFS) Stat(context.Context, string) (vfs.VFSItem, error) {
	if filesystem.called != nil {
		filesystem.called <- struct{}{}
	}
	<-filesystem.release
	return vfs.VFSItem{}, os.ErrNotExist
}

func TestOpenTextEditorCreatesUnsavedVFSBuffer(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldFileState := fileops.GlobalFileState
	fileops.GlobalFileState = nil
	t.Cleanup(func() { fileops.GlobalFileState = oldFileState })

	dir := t.TempDir()
	filesystem := vfs.NewOSVFS(dir)
	target := filepath.Join(dir, "clip.mkv.MediaInfo.txt")
	pf := &panel.PanelsFrame{LastW: 100, LastH: 30}
	content := []byte("General\nFormat : Matroska\n")
	if err := pf.OpenTextEditor(vfs.TextEditorRequest{
		VFS:      filesystem,
		Path:     target,
		Content:  content,
		Modified: true,
	}); err != nil {
		t.Fatal(err)
	}

	editor, ok := vtui.FrameManager.GetTopFrame().(*editor.EditorView)
	if !ok {
		t.Fatalf("top frame = %T", vtui.FrameManager.GetTopFrame())
	}
	if editor.FilePath != target || !editor.Modified {
		t.Fatalf("editor target/modified = %q/%t", editor.FilePath, editor.Modified)
	}
	if editor.UseEditorConfig {
		t.Fatal("generated report unexpectedly enabled .editorconfig lookup")
	}
	got, err := editor.Pt.GetRange(0, editor.Pt.Size())
	if err != nil || string(got) != string(content) {
		t.Fatalf("editor content = %q, %v", got, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("create-new target unexpectedly exists: %v", err)
	}
	editor.Checkpoint()
	editor.Pt.Insert(editor.Pt.Size(), []byte("edited"))
	editor.Undo()
	if !editor.Modified {
		t.Fatal("edit + Undo cleared the unsaved create-new report state")
	}
	editor.Close()
}

func TestOpenTextEditorTemporaryFileIsRemovedOnClose(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldFileState := fileops.GlobalFileState
	fileops.GlobalFileState = nil
	t.Cleanup(func() { fileops.GlobalFileState = oldFileState })

	pf := &panel.PanelsFrame{LastW: 80, LastH: 25}
	if err := pf.OpenTextEditor(vfs.TextEditorRequest{Temporary: true, Content: []byte("temporary")}); err != nil {
		t.Fatal(err)
	}
	editor := vtui.FrameManager.GetTopFrame().(*editor.EditorView)
	temporaryPath := editor.FilePath
	if _, err := os.Stat(temporaryPath); err != nil {
		t.Fatalf("temporary file missing while editor is open: %v", err)
	}
	editor.Close()
	if _, err := os.Stat(temporaryPath); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains after close: %v", err)
	}
}

func TestOpenTextEditorRejectsExistingCreateNewTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	pf := &panel.PanelsFrame{}
	if err := pf.OpenTextEditor(vfs.TextEditorRequest{VFS: vfs.NewOSVFS(dir), Path: target, Content: []byte("replace")}); err == nil {
		t.Fatal("existing target was accepted")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "keep" {
		t.Fatalf("existing target changed: %q, %v", got, err)
	}
}

func TestTextEditorTargetCheckIsBoundedWhenVFSIgnoresContext(t *testing.T) {
	release := make(chan struct{})
	filesystem := &blockingTextEditorStatVFS{release: release}
	start := time.Now()
	_, err := panel.StatTextEditorTarget(filesystem, "report.txt", 25*time.Millisecond)
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("target check error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("target check took %v", elapsed)
	}
}

func TestOpenTextEditorSkipsRedundantCheckedTargetStat(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldFileState := fileops.GlobalFileState
	fileops.GlobalFileState = nil
	t.Cleanup(func() { fileops.GlobalFileState = oldFileState })

	release := make(chan struct{})
	called := make(chan struct{}, 1)
	dir := t.TempDir()
	filesystem := &blockingTextEditorStatVFS{
		VFS:     vfs.NewOSVFS(dir),
		release: release,
		called:  called,
	}
	pf := &panel.PanelsFrame{LastW: 80, LastH: 25}
	if err := pf.OpenTextEditor(vfs.TextEditorRequest{
		VFS:               filesystem,
		Path:              filepath.Join(dir, "report.txt"),
		Content:           []byte("report"),
		Modified:          true,
		TargetKnownAbsent: true,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("host repeated the caller's off-UI target Stat")
	default:
	}
	vtui.FrameManager.GetTopFrame().(*editor.EditorView).Close()
	close(release)
}
