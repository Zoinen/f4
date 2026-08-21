package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
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
	oldFileState := GlobalFileState
	GlobalFileState = nil
	t.Cleanup(func() { GlobalFileState = oldFileState })

	dir := t.TempDir()
	filesystem := vfs.NewOSVFS(dir)
	target := filepath.Join(dir, "clip.mkv.MediaInfo.txt")
	pf := &PanelsFrame{lastW: 100, lastH: 30}
	content := []byte("General\nFormat : Matroska\n")
	if err := pf.OpenTextEditor(vfs.TextEditorRequest{
		VFS:      filesystem,
		Path:     target,
		Content:  content,
		Modified: true,
	}); err != nil {
		t.Fatal(err)
	}

	editor, ok := vtui.FrameManager.GetTopFrame().(*EditorView)
	if !ok {
		t.Fatalf("top frame = %T", vtui.FrameManager.GetTopFrame())
	}
	if editor.filePath != target || !editor.modified {
		t.Fatalf("editor target/modified = %q/%t", editor.filePath, editor.modified)
	}
	if editor.UseEditorConfig {
		t.Fatal("generated report unexpectedly enabled .editorconfig lookup")
	}
	got, err := editor.pt.GetRange(0, editor.pt.Size())
	if err != nil || string(got) != string(content) {
		t.Fatalf("editor content = %q, %v", got, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("create-new target unexpectedly exists: %v", err)
	}
	editor.saveUndo(opOther)
	editor.pt.Insert(editor.pt.Size(), []byte("edited"))
	editor.Undo()
	if !editor.modified {
		t.Fatal("edit + Undo cleared the unsaved create-new report state")
	}
	editor.Close()
}

func TestOpenTextEditorTemporaryFileIsRemovedOnClose(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldFileState := GlobalFileState
	GlobalFileState = nil
	t.Cleanup(func() { GlobalFileState = oldFileState })

	pf := &PanelsFrame{lastW: 80, lastH: 25}
	if err := pf.OpenTextEditor(vfs.TextEditorRequest{Temporary: true, Content: []byte("temporary")}); err != nil {
		t.Fatal(err)
	}
	editor := vtui.FrameManager.GetTopFrame().(*EditorView)
	temporaryPath := editor.filePath
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
	pf := &PanelsFrame{}
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
	_, err := statTextEditorTarget(filesystem, "report.txt", 25*time.Millisecond)
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
	oldFileState := GlobalFileState
	GlobalFileState = nil
	t.Cleanup(func() { GlobalFileState = oldFileState })

	release := make(chan struct{})
	called := make(chan struct{}, 1)
	dir := t.TempDir()
	filesystem := &blockingTextEditorStatVFS{
		VFS:     vfs.NewOSVFS(dir),
		release: release,
		called:  called,
	}
	pf := &PanelsFrame{lastW: 80, lastH: 25}
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
	vtui.FrameManager.GetTopFrame().(*EditorView).Close()
	close(release)
}

func TestOpenTextEditorSaveCallbackOwnsSuccessfulSave(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldFileState := GlobalFileState
	GlobalFileState = nil
	t.Cleanup(func() { GlobalFileState = oldFileState })

	got := make(chan []byte, 1)
	pf := &PanelsFrame{lastW: 80, lastH: 25}
	if err := pf.OpenTextEditor(vfs.TextEditorRequest{
		Temporary: true,
		Content:   []byte("before"),
		Modified:  true,
		OnSave: func(ctx context.Context, content []byte) error {
			if ctx == nil {
				return errors.New("save callback received a nil context")
			}
			got <- append([]byte(nil), content...)
			content[0] = 'X' // The editor must retain ownership of its buffer.
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	editor := vtui.FrameManager.GetTopFrame().(*EditorView)
	editor.pt.Insert(editor.pt.Size(), []byte(" after"))
	editor.modified = true
	afterSave := false
	editor.SaveToFile(func() { afterSave = true })
	waitEditorSave(t, editor)

	select {
	case content := <-got:
		if string(content) != "before after" {
			t.Fatalf("callback content = %q", content)
		}
	default:
		t.Fatal("successful save callback did not publish content")
	}
	current, err := editor.pt.Bytes()
	if err != nil || string(current) != "before after" {
		t.Fatalf("editor content after callback = %q, %v", current, err)
	}
	if editor.modified {
		t.Fatal("successful callback save left the editor modified")
	}
	if !afterSave {
		t.Fatal("successful callback save did not run afterSave")
	}
	editor.Close()
}

func TestOpenTextEditorSaveCallbackCanRejectSave(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldFileState := GlobalFileState
	GlobalFileState = nil
	t.Cleanup(func() { GlobalFileState = oldFileState })

	rejected := errors.New("patch no longer applies")
	called := make(chan struct{}, 1)
	pf := &PanelsFrame{lastW: 80, lastH: 25}
	if err := pf.OpenTextEditor(vfs.TextEditorRequest{
		Temporary: true,
		Content:   []byte("patch"),
		Modified:  true,
		OnSave: func(context.Context, []byte) error {
			called <- struct{}{}
			return rejected
		},
	}); err != nil {
		t.Fatal(err)
	}

	editor := vtui.FrameManager.GetTopFrame().(*EditorView)
	afterSave := false
	editor.SaveToFile(func() { afterSave = true })
	waitEditorSave(t, editor)

	select {
	case <-called:
	default:
		t.Fatal("save callback was not invoked")
	}
	if !editor.modified {
		t.Fatal("rejected callback save cleared the modified state")
	}
	if afterSave {
		t.Fatal("rejected callback save closed or completed the editor")
	}
	editor.Close()
}
