package main

import (
	"encoding/json"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"sync"
)

type F4HistoryProvider struct {
	mu   sync.Mutex
	path string
	data map[string][]string
}

func NewF4HistoryProvider() *F4HistoryProvider {
	p := filepath.Join(GetF4ConfigDir(), "history.json")
	hp := &F4HistoryProvider{
		path: p,
		data: make(map[string][]string),
	}
	hp.load()
	return hp
}

func (hp *F4HistoryProvider) load() {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	file, err := os.ReadFile(hp.path)
	if err == nil {
		json.Unmarshal(file, &hp.data)
	}
	if hp.data == nil {
		hp.data = make(map[string][]string)
	}
}

func (hp *F4HistoryProvider) save() {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	os.MkdirAll(filepath.Dir(hp.path), 0755)
	file, err := json.MarshalIndent(hp.data, "", "  ")
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
	hp.data[id] = history
	hp.mu.Unlock()
	hp.save()
}

func AddFolderHistory(path string) {
	if path == "" || path == "." || vtui.GlobalHistoryProvider == nil {
		return
	}
	h := vtui.GlobalHistoryProvider.LoadHistory("folders")
	// Deduplicate and move to top
	newHist := []string{path}
	for _, item := range h {
		if item != path {
			newHist = append(newHist, item)
		}
	}
	// Limit to 100 items
	if len(newHist) > 100 {
		newHist = newHist[:100]
	}
	vtui.GlobalHistoryProvider.SaveHistory("folders", newHist)
}
