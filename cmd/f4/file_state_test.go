package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestF4FileStateProvider_LRUOrder(t *testing.T) {
	fs := &F4FileStateProvider{
		Limit: 3,
		Data:  make(map[string]*FileState),
	}

	// Заполняем до лимита
	fs.SaveViewerState("file1", 10, true, false)
	fs.SaveViewerState("file2", 20, true, false)
	fs.SaveViewerState("file3", 30, true, false)

	// "Трогаем" первый файл — он должен стать самым "новым"
	fs.SaveViewerState("file1", 100, true, false)

	// Добавляем четвертый файл. Вытесниться должен file2 (самый старый), а не file1.
	fs.SaveViewerState("file4", 40, true, false)

	if fs.GetState("file2") != nil {
		t.Error("file2 should have been evicted as the oldest")
	}
	if fs.GetState("file1") == nil {
		t.Error("file1 should be preserved as it was recently used")
	}
}

func TestF4FileStateProvider_SaveAndRestore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "file_states_mru.json")

	fs := &F4FileStateProvider{
		path:  dbPath,
		Limit: 10,
		Data:  make(map[string]*FileState),
	}

	// 1. Сохранение метаданных редактора
	fs.SaveEditorState("main.go", 10, 5, 2, 0, true)

	state := fs.GetState("main.go")
	if state == nil {
		t.Fatal("Expected state for main.go, got nil")
	}
	if state.EditorLine != 10 || state.EditorPos != 5 || state.EditorTopRow != 2 || state.EditorWrap != true {
		t.Errorf("Incorrect saved editor state: %+v", state)
	}

	// 2. Сохранение метаданных просмотрщика
	fs.SaveViewerState("readme.md", 500, false, true)

	stateV := fs.GetState("readme.md")
	if stateV == nil {
		t.Fatal("Expected state for readme.md, got nil")
	}
	if stateV.ViewerOffset != 500 || stateV.ViewerWrap != false || stateV.ViewerHex != true {
		t.Errorf("Incorrect saved viewer state: %+v", stateV)
	}

	fs.SaveQuickViewCodepage("readme.md", 866)
	stateV = fs.GetState("readme.md")
	if stateV == nil || stateV.QuickViewCodepage != 866 {
		t.Errorf("Incorrect saved Quick View codepage: %+v", stateV)
	}

	fs.SaveCodepage("readme.md", 1251)
	stateV = fs.GetState("readme.md")
	if stateV == nil || stateV.Codepage != 1251 {
		t.Errorf("Incorrect saved Editor/Viewer codepage: %+v", stateV)
	}
	fs.SaveCodepage("readme.md", 0)
	stateV = fs.GetState("readme.md")
	if stateV == nil || stateV.Codepage != 0 {
		t.Errorf("Codepage override was not cleared: %+v", stateV)
	}
}

func TestF4FileStateProvider_AsyncSaveUpdatesMemoryAndDisk(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "file_states_async.json")
	fs := &F4FileStateProvider{
		path:  dbPath,
		Limit: 10,
		Data:  make(map[string]*FileState),
	}

	fs.SaveEditorStateAsync("main.go", 12, 7, 3, 1, true)
	state := fs.GetState("main.go")
	if state == nil || state.EditorLine != 12 || state.EditorPos != 7 {
		t.Fatalf("async save did not update in-memory state immediately: %+v", state)
	}

	fs.Flush()
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("async state was not persisted after Flush: %v", err)
	}
}
