package history

import (
	"encoding/json"
	"sort"
	"strings"
)

type Far3ImportCounts struct {
	Commands, Folders, Files, Skipped int
}

func far3PathKey(path string) string {
	return strings.ToLower(strings.ReplaceAll(path, "/", "\\"))
}

func mergeFar3Records(current, incoming []HistoryRecord, commands bool) ([]HistoryRecord, int) {
	merged := append([]HistoryRecord(nil), current...)
	type key struct{ name, dir string }
	identity := func(r HistoryRecord) key {
		if commands {
			return key{r.Name, far3PathKey(r.Directory())}
		}
		return key{name: far3PathKey(r.Name)}
	}
	seen := make(map[key]int, len(merged)+len(incoming))
	for i, r := range merged {
		seen[identity(r)] = i
	}
	added := 0
	for _, r := range incoming {
		k := identity(r)
		if i, ok := seen[k]; ok {
			locked := merged[i].Lock || r.Lock
			if r.Timestamp.After(merged[i].Timestamp) {
				merged[i] = r
			}
			merged[i].Lock = locked
		} else {
			seen[k] = len(merged)
			merged = append(merged, r)
			added++
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Timestamp.After(merged[j].Timestamp) })
	return merged, added
}

func mergeFar3Files(current []string, incoming []ViewerEditorRecord) ([]string, int) {
	merged := append([]string(nil), current...)
	seen := make(map[string]int, len(merged)+len(incoming))
	for i, raw := range merged {
		var r ViewerEditorRecord
		if json.Unmarshal([]byte(raw), &r) == nil && r.Local {
			seen[far3PathKey(r.Path)] = i
		}
	}
	added := 0
	for _, r := range incoming {
		k := far3PathKey(r.Path)
		if i, ok := seen[k]; ok {
			var old ViewerEditorRecord
			_ = json.Unmarshal([]byte(merged[i]), &old)
			locked := r.Lock || old.Lock
			if !r.Timestamp.After(old.Timestamp) {
				r = old
			}
			r.Lock = locked
			encoded, _ := json.Marshal(r)
			merged[i] = string(encoded)
		} else {
			seen[k] = len(merged)
			encoded, _ := json.Marshal(r)
			merged = append(merged, string(encoded))
			added++
		}
	}
	// Decode once for sorting; histories can contain thousands of files.
	type dated struct {
		raw    string
		record ViewerEditorRecord
	}
	items := make([]dated, len(merged))
	for i, raw := range merged {
		items[i].raw = raw
		_ = json.Unmarshal([]byte(raw), &items[i].record)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].record.Timestamp.After(items[j].record.Timestamp) })
	for i := range items {
		merged[i] = items[i].raw
	}
	return merged, added
}

// MergeFar3History merges all groups under one lock and schedules one write.
// Imported records are not truncated to the ordinary interactive history limit.
func (hp *F4HistoryProvider) MergeFar3History(source Far3History) Far3ImportCounts {
	hp.lockForMutation()
	defer hp.mu.Unlock()
	counts := Far3ImportCounts{Skipped: source.Skipped}
	hp.rich["cmdline"], counts.Commands = mergeFar3Records(hp.rich["cmdline"], source.Commands, true)
	hp.rich["folders"], counts.Folders = mergeFar3Records(hp.rich["folders"], source.Folders, false)
	hp.data["cmdline"] = ExtractHistoryNames(hp.rich["cmdline"])
	hp.data["folders"] = ExtractHistoryNames(hp.rich["folders"])
	hp.data[ViewerEditorHistoryID], counts.Files = mergeFar3Files(hp.data[ViewerEditorHistoryID], source.Files)
	hp.markDirtyLocked("import", "far3", nil)
	return counts
}
