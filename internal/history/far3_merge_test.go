package history

import (
	"path/filepath"
	"testing"
	"time"
)

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
