package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var GlobalFileState *F4FileStateProvider

type FileState struct {
	EditorLine   int   `json:"el"`
	EditorPos    int   `json:"ep"`
	EditorTopRow int   `json:"et"`
	EditorLeft   int   `json:"elft"`
	EditorWrap   bool  `json:"ew"`
	ViewerOffset int64 `json:"vo"`
	ViewerWrap   bool  `json:"vw"`
	ViewerHex    bool  `json:"vh"`
}

type F4FileStateProvider struct {
	mu    sync.Mutex
	path  string
	Limit int
	Order []string
	Data  map[string]*FileState
}

func NewF4FileStateProvider() *F4FileStateProvider {
	p := filepath.Join(GetF4ConfigDir(), "file_states.json")
	fs := &F4FileStateProvider{
		path:  p,
		Limit: 1000,
		Data:  make(map[string]*FileState),
	}
	fs.load()
	return fs
}

func (fs *F4FileStateProvider) load() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	file, err := os.ReadFile(fs.path)
	if err == nil {
		type DiskFormat struct {
			Order []string
			Data  map[string]*FileState
		}
		var df DiskFormat
		if json.Unmarshal(file, &df) == nil {
			fs.Order = df.Order
			fs.Data = df.Data
		}
	}
	if fs.Data == nil {
		fs.Data = make(map[string]*FileState)
	}
}

func (fs *F4FileStateProvider) save() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	os.MkdirAll(filepath.Dir(fs.path), 0755)
	type DiskFormat struct {
		Order []string
		Data  map[string]*FileState
	}
	df := DiskFormat{Order: fs.Order, Data: fs.Data}
	file, err := json.MarshalIndent(df, "", "  ")
	if err == nil {
		os.WriteFile(fs.path, file, 0644)
	}
}

func (fs *F4FileStateProvider) GetState(path string) *FileState {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if state, ok := fs.Data[path]; ok {
		cp := *state
		return &cp
	}
	return nil
}

func (fs *F4FileStateProvider) touch(path string) *FileState {
	state, ok := fs.Data[path]
	if !ok {
		state = &FileState{ViewerWrap: true}
		fs.Data[path] = state
	} else {
		for i, p := range fs.Order {
			if p == path {
				fs.Order = append(fs.Order[:i], fs.Order[i+1:]...)
				break
			}
		}
	}
	fs.Order = append(fs.Order, path)
	if len(fs.Order) > fs.Limit {
		oldest := fs.Order[0]
		fs.Order = fs.Order[1:]
		delete(fs.Data, oldest)
	}
	return state
}

func (fs *F4FileStateProvider) SaveEditorState(path string, line, pos, top, left int, wrap bool) {
	fs.mu.Lock()
	state := fs.touch(path)
	state.EditorLine = line
	state.EditorPos = pos
	state.EditorTopRow = top
	state.EditorLeft = left
	state.EditorWrap = wrap
	fs.mu.Unlock()
	fs.save()
}

func (fs *F4FileStateProvider) SaveViewerState(path string, offset int64, wrap, hex bool) {
	fs.mu.Lock()
	state := fs.touch(path)
	state.ViewerOffset = offset
	state.ViewerWrap = wrap
	state.ViewerHex = hex
	fs.mu.Unlock()
	fs.save()
}
