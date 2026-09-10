package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Editor tests whose subject is not the editor: three drive the Workspace.Close
// action, which internal/editor reaches only through a seam; one uses the
// metadata mock this package declares; one exercises the session file the root
// owns. They stayed here when the editor left.

func TestEditorView_WorkspaceCloseActionConfirmsUnsavedChanges(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager.Push(vtui.NewDesktop())

	ev := editor.NewEditorView(piecetable.New([]byte("test")), nil, "file.txt")
	defer ev.Close()
	vtui.FrameManager.AddScreen(ev)
	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: '!',
	})

	if !ActionWorkspaceClose() {
		t.Fatal("Workspace.Close action was not handled")
	}
	vtui.FrameManager.Step(0)

	if ev.IsDone() {
		t.Fatal("Workspace.Close action closed an editor with unsaved changes")
	}
	if len(vtui.FrameManager.Screens) != 2 {
		t.Fatalf("Workspace.Close action left %d workspaces, want the editor workspace preserved", len(vtui.FrameManager.Screens))
	}
	requireUnsavedChangesConfirm(t)
}

func TestEditorView_WorkspaceCloseActionClosesCleanEditor(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager.Push(vtui.NewDesktop())

	ev := editor.NewEditorView(piecetable.New([]byte("test")), nil, "file.txt")
	defer ev.Close()
	vtui.FrameManager.AddScreen(ev)

	if !ActionWorkspaceClose() {
		t.Fatal("Workspace.Close action was not handled")
	}
	vtui.FrameManager.Step(0)

	if !ev.IsDone() {
		t.Fatal("Workspace.Close action did not close an unmodified editor")
	}
	if len(vtui.FrameManager.Screens) != 1 {
		t.Fatalf("Workspace.Close action left %d workspaces, want the clean editor workspace closed", len(vtui.FrameManager.Screens))
	}
}

func TestEditorView_BackgroundWorkspaceCloseAnchorsUnsavedChangesConfirm(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager.Push(vtui.NewDesktop())
	previouslyActive := vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx]

	ev := editor.NewEditorView(piecetable.New([]byte("test")), nil, "file.txt")
	defer ev.Close()
	vtui.FrameManager.AddScreen(ev)
	editorScreen := vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx]
	pressKey(ev, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: '!',
	})
	vtui.FrameManager.SwitchScreen(0)

	if !actionWorkspaceCloseNumber(editorScreen.Number) {
		t.Fatal("background Workspace.Close action was not handled")
	}
	vtui.FrameManager.Step(0)

	if ev.IsDone() {
		t.Fatal("background Workspace.Close action closed an editor with unsaved changes")
	}
	if vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx] != editorScreen {
		t.Fatal("unsaved-changes confirmation did not focus the editor's workspace")
	}
	if confirms := unsavedChangesConfirms(previouslyActive); len(confirms) != 0 {
		t.Fatalf("previously active workspace has %d unsaved-changes confirmations, want none", len(confirms))
	}
	requireUnsavedChangesConfirm(t)
}
func TestEditorView_Save_MetadataIntegrity(t *testing.T) {
	// Verifies that owner, group, and permissions are restored after atomic save.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "meta.txt")
	if err := os.WriteFile(path, []byte("Original"), 0600); err != nil {
		t.Fatal(err)
	}

	// Mock metadata
	expectedMeta := vfs.VFSItem{
		Name:     "meta.txt",
		UnixMode: 0600, // Private file
		Uid:      1234,
		Gid:      5678,
		MTime:    time.Now().Add(-1 * time.Hour),
		ATime:    time.Now().Add(-2 * time.Hour),
	}

	// Mock VFS to track calls
	var capturedMeta vfs.VFSItem
	var attrCalled bool

	baseVfs := vfs.NewOSVFS(tmpDir)
	mock := &mockMetadataVFS{
		VFS:          baseVfs,
		statToReturn: expectedMeta,
		onSetAttr: func(item vfs.VFSItem) {
			capturedMeta = item
			attrCalled = true
		},
	}

	Pt := piecetable.New([]byte("Original"))
	ev := editor.NewEditorView(Pt, mock, path)
	defer ev.Close()
	f, err := mock.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer func() { _ = f.Close() }()
	ev.File = f

	// 1. Modify and Save
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '!'})
	ev.SaveToFile(nil)

	// Pump tasks
	timeout := time.After(2 * time.Second)
	for ev.Saving {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout")
		}
	}
	// 2. Verification
	if !attrCalled {
		t.Error("vfs.SetAttributes was not called after save")
	}
	if capturedMeta.UnixMode != expectedMeta.UnixMode || capturedMeta.Uid != expectedMeta.Uid {
		t.Errorf("Metadata mismatch. Expected mode %o UID %d, got mode %o UID %d",
			expectedMeta.UnixMode, expectedMeta.Uid, capturedMeta.UnixMode, capturedMeta.Uid)
	}
}

func TestEditorView_SearchPersistence(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	origPattern := editor.LastEditorSearch
	origCase := editor.LastEditorSearchCase
	origReverse := editor.LastEditorSearchReverse
	origRegexp := editor.LastEditorSearchRegexp
	origWholeWord := editor.LastEditorSearchWholeWord
	t.Cleanup(func() {
		editor.LastEditorSearch = origPattern
		editor.LastEditorSearchCase = origCase
		editor.LastEditorSearchReverse = origReverse
		editor.LastEditorSearchRegexp = origRegexp
		editor.LastEditorSearchWholeWord = origWholeWord
	})
	origSessionPath := GetSessionIniPath
	sessionDir := t.TempDir()
	GetSessionIniPath = func() string { return filepath.Join(sessionDir, "session.ini") }
	t.Cleanup(func() { GetSessionIniPath = origSessionPath })

	// 1. Выполняем поиск
	ev1 := editor.NewEditorView(piecetable.New([]byte("pattern")), nil, "f1.txt")
	t.Cleanup(ev1.Close)
	ev1.CursorPos = len("pattern")
	ev1.Search("pattern", true, true, false, false, false)

	// Дожидаемся завершения асинхронного поиска
	timeout := time.After(1 * time.Second)
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(10 * time.Millisecond):
		}
		if ev1.SelActive {
			break
		}
		select {
		case <-timeout:
			t.Fatal("Search globals were not updated")
		default:
		}
	}

	// 2. Проверяем переменные
	if editor.LastEditorSearch != "pattern" || !editor.LastEditorSearchCase || !editor.LastEditorSearchReverse {
		t.Errorf("Search parameters lost: %q, case:%v, rev:%v",
			editor.LastEditorSearch, editor.LastEditorSearchCase, editor.LastEditorSearchReverse)
	}
}
