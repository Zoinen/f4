package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/unxed/f4/vfs"
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
	mu      sync.Mutex
	writeMu sync.Mutex
	saveWG  sync.WaitGroup
	path    string
	Limit   int
	Order   []string
	Data    map[string]*FileState
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
	fs.writeMu.Lock()
	defer fs.writeMu.Unlock()

	fs.mu.Lock()
	type DiskFormat struct {
		Order []string
		Data  map[string]*FileState
	}
	df := DiskFormat{
		Order: append([]string(nil), fs.Order...),
		Data:  make(map[string]*FileState, len(fs.Data)),
	}
	for path, state := range fs.Data {
		copy := *state
		df.Data[path] = &copy
	}
	statePath := fs.path
	fs.mu.Unlock()

	os.MkdirAll(filepath.Dir(statePath), 0755)
	file, err := json.MarshalIndent(df, "", "  ")
	if err == nil {
		os.WriteFile(statePath, file, 0644)
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
	fs.updateEditorState(path, line, pos, top, left, wrap)
	fs.save()
}

func (fs *F4FileStateProvider) SaveEditorStateAsync(path string, line, pos, top, left int, wrap bool) {
	fs.updateEditorState(path, line, pos, top, left, wrap)
	fs.saveAsync()
}

func (fs *F4FileStateProvider) updateEditorState(path string, line, pos, top, left int, wrap bool) {
	fs.mu.Lock()
	state := fs.touch(path)
	state.EditorLine = line
	state.EditorPos = pos
	state.EditorTopRow = top
	state.EditorLeft = left
	state.EditorWrap = wrap
	fs.mu.Unlock()
}

func (fs *F4FileStateProvider) SaveViewerState(path string, offset int64, wrap, hex bool) {
	fs.updateViewerState(path, offset, wrap, hex)
	fs.save()
}

func (fs *F4FileStateProvider) SaveViewerStateAsync(path string, offset int64, wrap, hex bool) {
	fs.updateViewerState(path, offset, wrap, hex)
	fs.saveAsync()
}

func (fs *F4FileStateProvider) updateViewerState(path string, offset int64, wrap, hex bool) {
	fs.mu.Lock()
	state := fs.touch(path)
	state.ViewerOffset = offset
	state.ViewerWrap = wrap
	state.ViewerHex = hex
	fs.mu.Unlock()
}

func (fs *F4FileStateProvider) saveAsync() {
	fs.saveWG.Add(1)
	go func() {
		defer fs.saveWG.Done()
		fs.save()
	}()
}

// Flush waits for state writes queued by editor and viewer close operations.
// Application shutdown uses it so an immediate exit cannot lose the last
// cursor or viewer position.
func (fs *F4FileStateProvider) Flush() {
	fs.saveWG.Wait()
}

// FileStateKey returns the key a file's saved editor and viewer state lives
// under. A path on its own does not identify a file: /etc/hosts on a FISH+ host
// and /etc/hosts here are different files, and so is the same path on a second
// host. The key therefore carries which file system the path was on. A local
// path is left bare, both because it needs no qualifier and because that is how
// the states already on disk are written.
func FileStateKey(v vfs.VFS, path string) string {
	if v == nil {
		return path
	}
	abs, err := v.Abs(path)
	if err != nil || abs == "" {
		abs = path
	}
	ns := vfsStateNamespace(v)
	if ns == "" {
		return abs
	}
	return ns + ":" + abs
}

// vfsStateNamespace names the file system a path belongs to. A remote site
// already has a name the user sees in the panel title, and that is exactly the
// distinction being drawn here. Anything else falls back to its type, which
// tells a local file from an archive member but not one archive from another —
// see REVIEW.md.
func vfsStateNamespace(v vfs.VFS) string {
	if _, isLocal := v.(*vfs.OSVFS); isLocal {
		return ""
	}
	if tp, ok := v.(vfs.TitleProvider); ok {
		if title := tp.GetTitle(); title != "" {
			return title
		}
	}
	return fmt.Sprintf("%T", v)
}
