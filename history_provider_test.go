package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func TestF4HistoryProvider_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "history.json")

	hp := &F4HistoryProvider{
		path: dbPath,
		data: make(map[string][]string),
	}
	t.Cleanup(func() { _ = hp.Close() })

	// 1. Save data
	items := []string{"cmd1", "cmd2"}
	hp.SaveHistory("test", items)
	if err := hp.Flush(); err != nil {
		t.Fatalf("flush history: %v", err)
	}

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
	defer func() {
		_ = hp.Close()
		vtui.GlobalHistoryProvider = nil
	}()

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

func TestAddFolderHistoryCoalescesAliasesAndPreservesLock(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "folder")
	alias := canonical + string(os.PathSeparator) + "."
	hp := &F4HistoryProvider{
		path: filepath.Join(t.TempDir(), "history.json"),
		data: map[string][]string{
			"folders": {canonical, alias},
		},
		rich: map[string][]HistoryRecord{
			"folders": {
				{Name: canonical},
				{Name: alias, Lock: true},
			},
		},
		saveDebounce: time.Hour,
	}
	t.Cleanup(func() { _ = hp.Close() })

	before, after, _ := hp.addFolderHistory(canonical, nil)
	if before != 1 || after != 1 {
		t.Fatalf("alias-coalesced sizes = (%d, %d), want (1, 1)", before, after)
	}
	if got := hp.LoadHistory("folders"); !reflect.DeepEqual(got, []string{canonical}) {
		t.Fatalf("plain folder history = %#v, want [%q]", got, canonical)
	}
	records := hp.LoadRichHistory("folders")
	if len(records) != 1 || records[0].Name != canonical || !records[0].Lock {
		t.Fatalf("rich folder history = %#v, want one locked canonical record", records)
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
	t.Cleanup(func() {
		_ = hp.Close()
		vtui.GlobalHistoryProvider = previous
	})

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

func TestF4HistoryProviderFolderBurstUpdatesBothViewsAndWritesOnce(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.json")
	var writes atomic.Int32
	hp := &F4HistoryProvider{
		path:         dbPath,
		data:         make(map[string][]string),
		rich:         make(map[string][]HistoryRecord),
		saveDebounce: time.Hour,
		writeFile: func(path string, data []byte, mode os.FileMode) error {
			writes.Add(1)
			return os.WriteFile(path, data, mode)
		},
	}
	previous := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = hp
	t.Cleanup(func() {
		_ = hp.Close()
		vtui.GlobalHistoryProvider = previous
	})

	AddFolderHistory("/one")
	AddFolderHistory("/two")
	AddFolderHistory("/one")

	want := []string{"/one", "/two"}
	if got := hp.LoadHistory("folders"); !reflect.DeepEqual(got, want) {
		t.Fatalf("plain MRU before persistence = %#v, want %#v", got, want)
	}
	rich := hp.LoadRichHistory("folders")
	if got := extractNames(rich); !reflect.DeepEqual(got, want) {
		t.Fatalf("rich MRU before persistence = %#v, want %#v", got, want)
	}
	if got := writes.Load(); got != 0 {
		t.Fatalf("debounced burst wrote synchronously %d times", got)
	}

	if err := hp.Flush(); err != nil {
		t.Fatalf("flush folder burst: %v", err)
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("folder burst disk writes = %d, want 1", got)
	}

	reloaded := &F4HistoryProvider{
		path: dbPath,
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	reloaded.load()
	if got := reloaded.LoadHistory("folders"); !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted plain MRU = %#v, want %#v", got, want)
	}
	if got := extractNames(reloaded.LoadRichHistory("folders")); !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted rich MRU = %#v, want %#v", got, want)
	}
}

func TestF4HistoryProviderRevisionPreventsOlderWriteWinning(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.json")
	firstWriteStarted := make(chan struct{})
	releaseFirstWrite := make(chan struct{})
	var writes atomic.Int32
	hp := &F4HistoryProvider{
		path:         dbPath,
		data:         make(map[string][]string),
		rich:         make(map[string][]HistoryRecord),
		saveDebounce: time.Millisecond,
		writeFile: func(path string, data []byte, mode os.FileMode) error {
			if writes.Add(1) == 1 {
				close(firstWriteStarted)
				<-releaseFirstWrite
			}
			return os.WriteFile(path, data, mode)
		},
	}
	t.Cleanup(func() { _ = hp.Close() })

	hp.SaveHistory("test", []string{"old"})
	select {
	case <-firstWriteStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first asynchronous write did not start")
	}
	hp.SaveHistory("test", []string{"new"})
	flushDone := make(chan error, 1)
	go func() { flushDone <- hp.Flush() }()
	close(releaseFirstWrite)
	select {
	case err := <-flushDone:
		if err != nil {
			t.Fatalf("flush latest revision: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("flush blocked behind an older revision")
	}

	if got := writes.Load(); got != 2 {
		t.Fatalf("revisioned writes = %d, want old snapshot then latest snapshot", got)
	}
	reloaded := &F4HistoryProvider{
		path: dbPath,
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	reloaded.load()
	if got := reloaded.LoadHistory("test"); !reflect.DeepEqual(got, []string{"new"}) {
		t.Fatalf("persisted latest revision = %#v, want [new]", got)
	}
}

func TestF4HistoryProviderConcurrentFolderUpdatesLoseNoEntries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.json")
	hp := &F4HistoryProvider{
		path:         dbPath,
		data:         make(map[string][]string),
		rich:         make(map[string][]HistoryRecord),
		saveDebounce: time.Hour,
	}
	previous := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = hp
	t.Cleanup(func() {
		_ = hp.Close()
		vtui.GlobalHistoryProvider = previous
	})

	const count = 32
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		path := fmt.Sprintf("/concurrent/%02d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			AddFolderHistory(path)
		}()
	}
	wg.Wait()

	plain := hp.LoadHistory("folders")
	if len(plain) != count {
		t.Fatalf("concurrent MRU contains %d entries, want %d: %#v", len(plain), count, plain)
	}
	seen := make(map[string]bool, count)
	for _, path := range plain {
		seen[path] = true
	}
	for i := 0; i < count; i++ {
		path := fmt.Sprintf("/concurrent/%02d", i)
		if !seen[path] {
			t.Fatalf("concurrent MRU lost %q: %#v", path, plain)
		}
	}
	if got := extractNames(hp.LoadRichHistory("folders")); !reflect.DeepEqual(got, plain) {
		t.Fatalf("plain/rich projections diverged: plain=%#v rich=%#v", plain, got)
	}
	if err := hp.Close(); err != nil {
		t.Fatalf("shutdown close: %v", err)
	}

	reloaded := &F4HistoryProvider{
		path: dbPath,
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	reloaded.load()
	if got := reloaded.LoadHistory("folders"); !reflect.DeepEqual(got, plain) {
		t.Fatalf("shutdown did not durably persist MRU: got %#v, want %#v", got, plain)
	}
}
