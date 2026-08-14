package main

import (
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type virtualItem struct {
	isDir bool
	data  []byte
	mtime time.Time
}

type DummyPlugin struct {
	host   *f4plugin.Host
	fs     map[string]*virtualItem
	open   map[uint32]string // ID -> Path
	mu     sync.Mutex
	nextID uint32
}

func (p *DummyPlugin) Init(host *f4plugin.Host) ([]string, error) {
	p.host = host
	p.fs = make(map[string]*virtualItem)
	p.open = make(map[uint32]string)

	// Create some initial dummy content
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("/file_%d.txt", i)
		p.fs[name] = &virtualItem{
			data:  []byte(fmt.Sprintf("This is dummy file number %d\n", i)),
			mtime: time.Now(),
		}
	}
	p.fs["/folder"] = &virtualItem{isDir: true, mtime: time.Now()}

	return []string{"Dummy RPC VFS"}, nil
}
func (p *DummyPlugin) Highlight(line string, prev any, base uint64) ([]uint64, any, error) {
	attrs := make([]uint64, len([]rune(line)))
	for i, r := range []rune(line) {
		attrs[i] = base
		if unicode.IsDigit(r) {
			attrs[i] = vtui.SetRGBFore(base, 0xFF8700) // Orange digits
		}
	}
	return attrs, nil, nil
}

func (p *DummyPlugin) ProcessKey(drive string, event vtinput.InputEvent) (bool, error) {
	if event.KeyDown && event.VirtualKeyCode == vtinput.VK_F1 && event.ControlKeyState == 0 {
		return true, p.showHello()
	}
	return false, nil
}

func (p *DummyPlugin) PluginCommands() []f4plugin.PluginCommand {
	return []f4plugin.PluginCommand{{
		ID:          "dummy-rpc.hello",
		Location:    f4plugin.PluginCommandPanel,
		Label:       "Show RPC panel greeting",
		Description: "Show the sample command contributed by the Dummy RPC VFS plugin",
		Shortcut:    "F1",
		LocalizedLabels: map[string]string{
			"ru": "Показать приветствие RPC-панели",
		},
		LocalizedDescriptions: map[string]string{
			"ru": "Показать пример команды, опубликованной плагином Dummy RPC VFS",
		},
		SearchTerms:  []string{"hello", "приветствие", "RPC"},
		ActiveDrives: []string{"Dummy RPC VFS"},
	}}
}

func (p *DummyPlugin) RunPluginCommand(id string) error {
	if id != "dummy-rpc.hello" {
		return fmt.Errorf("unknown plugin command %q", id)
	}
	return p.showHello()
}

func (p *DummyPlugin) showHello() error {
	if p.host == nil {
		return fmt.Errorf("plugin host is not initialized")
	}
	p.host.Message("Hello! You invoked the Dummy RPC panel command.")
	return nil
}

func (p *DummyPlugin) OnHotkey(vk uint16, mods uint32) error {
	p.host.Log("Global Hotkey triggered!")
	return nil
}

func (p *DummyPlugin) OnProgressTask() error {
	for i := 0; i <= 100; i += 5 {
		if p.host.IsProgressCancelled() {
			p.host.Log("Progress task cancelled by user.")
			return nil
		}
		p.host.UpdateProgress(fmt.Sprintf("RPC working... %d%%", i), i)
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func (p *DummyPlugin) ReadDir(drive, dpath string) ([]f4plugin.VFSItem, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var items []f4plugin.VFSItem
	prefix := dpath
	if prefix == "." {
		prefix = "/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	for name, item := range p.fs {
		if name == dpath {
			continue
		}
		dir := path.Dir(name)
		if !strings.HasSuffix(dir, "/") {
			dir += "/"
		}

		if dir == prefix {
			items = append(items, f4plugin.VFSItem{
				Name:  path.Base(name),
				Size:  int64(len(item.data)),
				IsDir: item.isDir,
				MTime: item.mtime,
			})
		}
	}
	return items, nil
}

func (p *DummyPlugin) Stat(drive, spath string) (f4plugin.VFSItem, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if spath == "/" || spath == "." || spath == "" {
		return f4plugin.VFSItem{Name: "/", IsDir: true}, nil
	}

	item, ok := p.fs[spath]
	if !ok {
		return f4plugin.VFSItem{}, fmt.Errorf("not found")
	}

	return f4plugin.VFSItem{
		Name:  path.Base(spath),
		Size:  int64(len(item.data)),
		IsDir: item.isDir,
		MTime: item.mtime,
	}, nil
}

func (p *DummyPlugin) Open(drive, dpath string) (uint32, int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item, ok := p.fs[dpath]
	if !ok || item.isDir {
		return 0, 0, fmt.Errorf("not a file")
	}

	p.nextID++
	id := p.nextID
	p.open[id] = dpath
	return id, int64(len(item.data)), nil
}

func (p *DummyPlugin) ReadAt(fileId uint32, length int, offset int64) ([]byte, error) {
	p.mu.Lock()
	dpath, ok := p.open[fileId]
	if !ok {
		p.mu.Unlock()
		return nil, fmt.Errorf("bad handle")
	}
	item := p.fs[dpath]
	p.mu.Unlock()

	if offset >= int64(len(item.data)) {
		return nil, io.EOF
	}
	end := offset + int64(length)
	if end > int64(len(item.data)) {
		end = int64(len(item.data))
	}
	return item.data[offset:end], nil
}

func (p *DummyPlugin) Create(drive, dpath string) (uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fs[dpath] = &virtualItem{data: []byte{}, mtime: time.Now()}
	p.nextID++
	id := p.nextID
	p.open[id] = dpath
	return id, nil
}

func (p *DummyPlugin) Write(fileId uint32, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	dpath, ok := p.open[fileId]
	if !ok {
		return fmt.Errorf("bad handle")
	}
	p.fs[dpath].data = append(p.fs[dpath].data, data...)
	p.fs[dpath].mtime = time.Now()
	return nil
}

func (p *DummyPlugin) CloseFile(fileId uint32) error {
	p.mu.Lock()
	delete(p.open, fileId)
	p.mu.Unlock()
	return nil
}

func (p *DummyPlugin) MkDir(drive, dpath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fs[dpath] = &virtualItem{isDir: true, mtime: time.Now()}
	return nil
}

func (p *DummyPlugin) Remove(drive, dpath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.fs, dpath)
	return nil
}

func (p *DummyPlugin) Rename(drive, old, new string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if item, ok := p.fs[old]; ok {
		p.fs[new] = item
		delete(p.fs, old)
		return nil
	}
	return fmt.Errorf("not found")
}

func main() {
	f4plugin.Run(&DummyPlugin{})
}
