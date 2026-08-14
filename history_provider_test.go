package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/unxed/vtui"
)

func TestF4HistoryProvider_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "history.json")

	hp := &F4HistoryProvider{
		path: dbPath,
		data: make(map[string][]string),
	}

	// 1. Save data
	items := []string{"cmd1", "cmd2"}
	hp.SaveHistory("test", items)

	// 2. Verify file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("History file was not created")
	}

	// 3. Create new provider and load
	hp2 := &F4HistoryProvider{
		path: dbPath,
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	hp2.load()

	loaded := hp2.LoadHistory("test")
	if len(loaded) != 2 || loaded[0] != "cmd1" || loaded[1] != "cmd2" {
		t.Errorf("Persistence failed. Expected %v, got %v", items, loaded)
	}
}

func TestAddFolderHistory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "history_mru.json")

	hp := &F4HistoryProvider{
		path: dbPath,
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	vtui.GlobalHistoryProvider = hp
	defer func() { vtui.GlobalHistoryProvider = nil }()

	// 1. Начальное наполнение (должно идти в порядке MRU: последний добавленный сверху)
	AddFolderHistory("/path/a")
	AddFolderHistory("/path/b")

	h := hp.LoadHistory("folders")
	if len(h) != 2 || h[0] != "/path/b" || h[1] != "/path/a" {
		t.Errorf("Expected '/path/b' to be on top, got %v", h)
	}

	// 2. Дедупликация и перемещение вверх списка (MRU)
	AddFolderHistory("/path/a") // "/path/a" должна вернуться наверх
	h = hp.LoadHistory("folders")
	if len(h) != 2 || h[0] != "/path/a" || h[1] != "/path/b" {
		t.Errorf("Deduplication and MRU move failed, got: %v", h)
	}

	// 3. Проверка лимита в 100 элементов
	for i := 0; i < 110; i++ {
		AddFolderHistory(filepath.Join("/path", strconv.Itoa(i)))
	}
	h = hp.LoadHistory("folders")
	if len(h) > 100 {
		t.Errorf("Expected history size to be capped at 100, got %d", len(h))
	}
}

func TestLockedFolderAndCommandHistorySurviveLimits(t *testing.T) {
	hp := &F4HistoryProvider{
		path: filepath.Join(t.TempDir(), "history.json"),
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	previous := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = hp
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previous })

	lockedPath := filepath.Join("/path", "locked")
	saveFolderHistoryRecords(hp, []HistoryRecord{{Name: lockedPath, Lock: true}})
	for i := 0; i < 110; i++ {
		AddFolderHistory(filepath.Join("/path", strconv.Itoa(i)))
	}
	folders, _ := loadFolderHistoryRecords(hp)
	found := false
	for _, record := range folders {
		if record.Name == lockedPath && record.Lock {
			found = true
		}
	}
	if !found {
		t.Fatal("locked folder was evicted by the history limit")
	}

	commands := []HistoryRecord{{Name: "newest"}, {Name: "pinned", Lock: true}, {Name: "old-1"}, {Name: "old-2"}}
	commands = limitRichHistory(commands, 2)
	if len(commands) != 2 || commands[0].Name != "newest" || commands[1].Name != "pinned" || !commands[1].Lock {
		t.Fatalf("limited command history = %#v", commands)
	}
}
