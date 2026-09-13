package history

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3/driver"
)

func TestFar3ReadMergeAndReimport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history #1.db")
	db, err := driver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`PRAGMA journal_mode=WAL;
		CREATE TABLE history (id INTEGER PRIMARY KEY, kind INTEGER, key TEXT, type INTEGER,
		lock INTEGER, name TEXT, time INTEGER, guid TEXT, file TEXT, data TEXT);`)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 9, 13, 10, 20, 30, 123456700, time.UTC)
	ticks := (stamp.Unix()+11644473600)*10000000 + int64(stamp.Nanosecond()/100)
	for _, row := range []struct {
		kind, typ       int
		name, dir, guid string
	}{
		{0, 0, "echo café\nnext", `D:\Work`, ""},
		{0, 0, "echo café\nnext", `D:\Elsewhere`, ""},
		{1, 0, `D:\Pictures`, "", "00000000-0000-0000-0000-000000000000"},
		{2, 0, `D:\view.txt`, "", ""},
		{2, 1, `D:\edit.txt`, "", ""},
		{2, 4, `D:\readonly.txt`, "", ""},
		{2, 2, `D:\external.txt`, "", ""},
		{1, 0, "plugin:/path", "", "42E4AEB1-A230-44F4-B33C-F195BB654931"},
		{3, 0, "dialog text", "", ""},
	} {
		if _, err := db.Exec(`INSERT INTO history(kind,key,type,lock,name,time,guid,file,data) VALUES(?, '', ?, 1, ?, ?, ?, '', ?)`,
			row.kind, row.typ, row.name, ticks, row.guid, row.dir); err != nil {
			t.Fatal(err)
		}
	}
	// Leave the writer open to exercise reading committed records from a live WAL.
	got, err := ReadFar3History(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Commands) != 2 || len(got.Folders) != 1 || len(got.Files) != 3 || got.Skipped != 2 {
		t.Fatalf("unexpected snapshot counts: %+v", got)
	}
	if !got.Commands[0].Timestamp.Equal(stamp) || !got.Commands[0].Lock || got.Commands[0].Dir != `D:\Elsewhere` {
		t.Fatalf("lost metadata: %+v", got.Commands[0])
	}
	if got.Files[0].Mode != HistoryModeEdit || got.Files[2].Mode != HistoryModeView {
		t.Fatalf("wrong modes: %+v", got.Files)
	}
	historyPath := filepath.Join(t.TempDir(), "history.json")
	hp := NewProviderAtPath(historyPath)
	defer hp.Close()
	hp.SaveRichHistory("folders", []HistoryRecord{{Name: `d:/pictures`, Timestamp: stamp.Add(time.Hour)}, {Name: `D:\keep`}})
	counts := hp.MergeFar3History(got)
	if counts != (Far3ImportCounts{Commands: 2, Files: 3, Skipped: 2}) {
		t.Fatalf("counts: %+v", counts)
	}
	if !hp.LoadRichHistory("folders")[0].Lock {
		t.Fatal("duplicate lost lock")
	}
	before := hp.LoadHistory(ViewerEditorHistoryID)
	counts = hp.MergeFar3History(got)
	if counts.Commands+counts.Folders+counts.Files != 0 {
		t.Fatalf("not idempotent: %+v", counts)
	}
	if !reflect.DeepEqual(before, hp.LoadHistory(ViewerEditorHistoryID)) {
		t.Fatal("reimport changed files")
	}
	if err := hp.Flush(); err != nil {
		t.Fatal(err)
	}
	reloaded := NewProviderAtPath(historyPath)
	defer reloaded.Close()
	if !reflect.DeepEqual(hp.LoadHistory("cmdline"), reloaded.LoadHistory("cmdline")) {
		t.Fatal("commands did not persist")
	}
	if !reflect.DeepEqual(before, reloaded.LoadHistory(ViewerEditorHistoryID)) {
		t.Fatal("files did not persist")
	}
	var record ViewerEditorRecord
	if err := json.Unmarshal([]byte(before[0]), &record); err != nil || !record.Local || record.VFSType != "*vfs.OSVFS" {
		t.Fatalf("not reopenable: %+v, %v", record, err)
	}
}

func TestFar3FailuresDoNotCreateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := ReadFar3History(context.Background(), path); err == nil {
		t.Fatal("missing DB accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("reader created source: %v", err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFar3History(context.Background(), path); err == nil {
		t.Fatal("invalid DB accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadFar3History(ctx, path); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestFar3ImportRetainsAllEntries(t *testing.T) {
	hp := NewProviderAtPath(filepath.Join(t.TempDir(), "history.json"))
	defer hp.Close()
	var source Far3History
	for i := 0; i < 1500; i++ {
		source.Commands = append(source.Commands, HistoryRecord{Name: time.Unix(int64(i), 0).String()})
	}
	if hp.MergeFar3History(source).Commands != 1500 || len(hp.LoadHistory("cmdline")) != 1500 {
		t.Fatal("import was truncated")
	}
}

func TestFar3LocalProfileReadOnly(t *testing.T) {
	path := os.Getenv("F4_TEST_FAR3_HISTORY")
	if path == "" {
		t.Skip("set F4_TEST_FAR3_HISTORY for a read-only local smoke test")
	}
	got, err := ReadFar3History(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Read %d commands, %d files, %d folders; skipped %d", len(got.Commands), len(got.Files), len(got.Folders), got.Skipped)
}
