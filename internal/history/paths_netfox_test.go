package history

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

func TestNetFoxHistoryPathsNormalizeWithoutDoubleDecoding(t *testing.T) {
	const legacy = "net://HC_SFTP/C%3A/Users/a%20%23%3F.txt"
	const readable = "net://HC_SFTP/C:/Users/a #?.txt"
	for _, tt := range []struct{ input, want string }{
		{legacy, readable},
		{readable, readable},
		{"net://HC_SFTP/C%3A/literal%253A.txt", "net://HC_SFTP/C:/literal%253A.txt"},
		{"/local/C%3A", "/local/C%3A"},
		{"net://HC_SFTP/%ZZ", "net://HC_SFTP/%ZZ"},
		{"net://HC_SFTP", "net://HC_SFTP"},
	} {
		if got := normalizeHistoryPath(tt.input); got != tt.want {
			t.Errorf("normalizeHistoryPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	hp := NewProviderAtPath(filepath.Join(t.TempDir(), "history.json"))
	previous := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = hp
	t.Cleanup(func() {
		_ = hp.Close()
		vtui.GlobalHistoryProvider = previous
	})
	hp.SaveHistory("folders", []string{legacy})
	hp.mu.Lock()
	if got := hp.data["folders"][0]; got != readable {
		hp.mu.Unlock()
		t.Fatalf("stored folder = %q", got)
	}
	// Simulate an existing profile from before canonical readable paths.
	hp.data["folders"] = []string{legacy}
	hp.mu.Unlock()
	records, _ := LoadFolderHistoryRecords(hp)
	if len(records) != 1 || records[0].Name != readable {
		t.Fatalf("loaded legacy records = %#v", records)
	}
	SaveCommandHistoryPaths([]string{"ls"}, []string{legacy})
	if got := LoadCommandHistoryPaths([]string{"ls"})[0]; got != readable {
		t.Fatalf("command directory = %q", got)
	}
	hp.SaveRichHistory("cmdline", []HistoryRecord{{Name: "ls", Dir: legacy}})
	if got := hp.LoadRichHistory("cmdline")[0].Directory(); got != readable {
		t.Fatalf("rich command directory = %q", got)
	}
	encoded, err := json.Marshal(ViewerEditorRecord{Path: legacy, Display: legacy})
	if err != nil {
		t.Fatal(err)
	}
	hp.SaveHistory(ViewerEditorHistoryID, []string{string(encoded)})
	var record ViewerEditorRecord
	if err := json.Unmarshal([]byte(hp.LoadHistory(ViewerEditorHistoryID)[0]), &record); err != nil {
		t.Fatal(err)
	}
	if record.Path != readable || record.Display != readable {
		t.Fatalf("viewer history = %#v", record)
	}
}
