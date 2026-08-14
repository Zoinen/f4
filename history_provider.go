package main

import (
	"encoding/json"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

import "time"

type HistoryRecord struct {
	Name      string    `json:"name"`
	Extra     string    `json:"extra,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	Lock      bool      `json:"lock,omitempty"`
}

func (r HistoryRecord) DisplayText() string {
	res := ""
	if !r.Timestamp.IsZero() {
		res += r.Timestamp.Format("15:04:05 ")
	}
	if r.Extra != "" {
		extra := r.Extra
		if len(extra) > 15 {
			extra = "..." + extra[len(extra)-12:]
		}
		if !strings.HasSuffix(extra, "/") && !strings.HasSuffix(extra, "\\") {
			res += extra + "/ "
		} else {
			res += extra + " "
		}
	}
	res += r.Name
	return res
}

type F4HistoryProvider struct {
	mu   sync.Mutex
	path string
	data map[string][]string
	rich map[string][]HistoryRecord
}

func NewF4HistoryProvider() *F4HistoryProvider {
	p := filepath.Join(GetF4ConfigDir(), "history.json")
	hp := &F4HistoryProvider{
		path: p,
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	hp.load()
	return hp
}

func (hp *F4HistoryProvider) load() {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	file, err := os.ReadFile(hp.path)
	if err == nil {
		var wrapper struct {
			Data map[string][]string        `json:"data,omitempty"`
			Rich map[string][]HistoryRecord `json:"rich,omitempty"`
		}
		if err := json.Unmarshal(file, &wrapper); err == nil && (wrapper.Data != nil || wrapper.Rich != nil) {
			if wrapper.Data != nil {
				hp.data = wrapper.Data
			}
			if wrapper.Rich != nil {
				hp.rich = wrapper.Rich
			}
		} else {
			var oldData map[string][]string
			if err := json.Unmarshal(file, &oldData); err == nil {
				hp.data = oldData
				if cmds, ok := hp.data["cmdline"]; ok {
					var rich []HistoryRecord
					for _, c := range cmds {
						rich = append(rich, HistoryRecord{Name: c})
					}
					hp.rich["cmdline"] = rich
					delete(hp.data, "cmdline")
				}
			}
		}
	}
	if hp.data == nil {
		hp.data = make(map[string][]string)
	}
	if hp.rich == nil {
		hp.rich = make(map[string][]HistoryRecord)
	}
}

func (hp *F4HistoryProvider) save() {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	os.MkdirAll(filepath.Dir(hp.path), 0755)
	wrapper := struct {
		Data map[string][]string        `json:"data,omitempty"`
		Rich map[string][]HistoryRecord `json:"rich,omitempty"`
	}{
		Data: hp.data,
		Rich: hp.rich,
	}
	if len(hp.data) == 0 {
		wrapper.Data = nil
	}
	if len(hp.rich) == 0 {
		wrapper.Rich = nil
	}
	file, err := json.MarshalIndent(wrapper, "", "  ")
	if err == nil {
		os.WriteFile(hp.path, file, 0644)
	}
}

func (hp *F4HistoryProvider) LoadHistory(id string) []string {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if items, ok := hp.data[id]; ok {
		// Return a copy to avoid concurrent slice modification issues
		res := make([]string, len(items))
		copy(res, items)
		return res
	}
	return nil
}

func (hp *F4HistoryProvider) SaveHistory(id string, history []string) {
	hp.mu.Lock()
	if hp.data == nil {
		hp.data = make(map[string][]string)
	}
	hp.data[id] = history
	hp.mu.Unlock()
	hp.save()
}
func (hp *F4HistoryProvider) LoadRichHistory(id string) []HistoryRecord {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if items, ok := hp.rich[id]; ok {
		res := make([]HistoryRecord, len(items))
		copy(res, items)
		return res
	}
	return nil
}

func (hp *F4HistoryProvider) SaveRichHistory(id string, history []HistoryRecord) {
	hp.mu.Lock()
	if hp.rich == nil {
		hp.rich = make(map[string][]HistoryRecord)
	}
	hp.rich[id] = history
	hp.mu.Unlock()
	hp.save()
}

func limitRichHistory(history []HistoryRecord, limit int) []HistoryRecord {
	if limit <= 0 || len(history) <= limit {
		return history
	}
	locked := 0
	for _, record := range history {
		if record.Lock {
			locked++
		}
	}
	unlockedBudget := limit - locked
	if unlockedBudget < 0 {
		unlockedBudget = 0
	}
	kept := make([]HistoryRecord, 0, limit)
	for _, record := range history {
		if record.Lock {
			kept = append(kept, record)
			continue
		}
		if unlockedBudget > 0 {
			kept = append(kept, record)
			unlockedBudget--
		}
	}
	return kept
}

func loadFolderHistoryRecords(provider vtui.HistoryProvider) ([]HistoryRecord, *F4HistoryProvider) {
	hp, _ := provider.(*F4HistoryProvider)
	plain := provider.LoadHistory("folders")
	if hp == nil {
		records := make([]HistoryRecord, 0, len(plain))
		for _, path := range plain {
			records = append(records, HistoryRecord{Name: path})
		}
		return records, nil
	}
	rich := hp.LoadRichHistory("folders")
	records := make([]HistoryRecord, 0, len(plain))
	for _, path := range plain {
		var record HistoryRecord
		for _, candidate := range rich {
			if sameFolderHistoryPath(candidate.Name, path) {
				record = candidate
				break
			}
		}
		record.Name = path
		records = append(records, record)
	}
	return records, hp
}

func saveFolderHistoryRecords(hp *F4HistoryProvider, records []HistoryRecord) {
	if hp == nil {
		return
	}
	hp.SaveRichHistory("folders", records)
	hp.SaveHistory("folders", extractNames(records))
}

func AddFolderHistory(path string) {
	if path == "" || path == "." || vtui.GlobalHistoryProvider == nil {
		return
	}
	if records, hp := loadFolderHistoryRecords(vtui.GlobalHistoryProvider); hp != nil {
		current := HistoryRecord{Name: path}
		newHistory := []HistoryRecord{current}
		for _, record := range records {
			if sameFolderHistoryPath(record.Name, path) {
				newHistory[0].Lock = record.Lock
				continue
			}
			newHistory = append(newHistory, record)
		}
		newHistory = limitRichHistory(newHistory, 100)
		saveFolderHistoryRecords(hp, newHistory)
		return
	}
	h := vtui.GlobalHistoryProvider.LoadHistory("folders")
	// Deduplicate and move to top
	newHist := []string{path}
	for _, item := range h {
		if !sameFolderHistoryPath(item, path) {
			newHist = append(newHist, item)
		}
	}
	// Limit to 100 items
	if len(newHist) > 100 {
		newHist = newHist[:100]
	}
	vtui.GlobalHistoryProvider.SaveHistory("folders", newHist)
}
