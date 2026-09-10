package f4plugin

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/vtinput"
	"github.com/vmihailenco/msgpack/v5"
)

type rpcHarness struct {
	client     *f4rpc.Session
	toPlugin   *io.PipeWriter
	fromPlugin *io.PipeWriter
	pluginDone chan error
	hostDone   chan error
}

func newHarness(t *testing.T, plugin Plugin) *rpcHarness {
	t.Helper()

	hostToPluginR, hostToPluginW := io.Pipe()
	pluginToHostR, pluginToHostW := io.Pipe()
	host := f4rpc.NewSession(pluginToHostR, hostToPluginW)
	registerHostHandlers(host)
	hostDone := make(chan error, 1)
	go func() { hostDone <- host.Serve() }()

	pluginDone := make(chan error, 1)
	go func() {
		pluginDone <- run(plugin, hostToPluginR, pluginToHostW, io.Discard)
	}()

	h := &rpcHarness{
		client:     host,
		toPlugin:   hostToPluginW,
		fromPlugin: pluginToHostW,
		pluginDone: pluginDone,
		hostDone:   hostDone,
	}
	t.Cleanup(func() {
		if err := h.toPlugin.Close(); err != nil {
			t.Errorf("close host-to-plugin pipe: %v", err)
		}
		if err := h.fromPlugin.Close(); err != nil {
			t.Errorf("close plugin-to-host pipe: %v", err)
		}
		if err := <-h.pluginDone; err != nil {
			t.Errorf("plugin Run: %v", err)
		}
		if err := <-h.hostDone; err != nil {
			t.Errorf("host Serve: %v", err)
		}
	})
	return h
}

func registerHostHandlers(host *f4rpc.Session) {
	for _, method := range []string{
		"Host.Log",
		"Host.Message",
		"Host.RegisterHighlighter",
		"Host.RegisterGlobalHotkey",
		"Host.RunProgressTask",
		"Host.UpdateProgress",
	} {
		host.Register(method, func(_ msgpack.RawMessage) (any, error) { return nil, nil })
	}
	host.Register("Host.GetVersion", func(_ msgpack.RawMessage) (any, error) { return "test-version", nil })
	host.Register("Host.RunAction", func(_ msgpack.RawMessage) (any, error) { return true, nil })
	host.Register("Host.IsProgressCancelled", func(_ msgpack.RawMessage) (any, error) { return false, nil })
	host.Register("Host.AskOverwrite", func(_ msgpack.RawMessage) (any, error) {
		return AskOverwriteRes{Choice: 7, Remember: true}, nil
	})
	host.Register("Host.AskError", func(_ msgpack.RawMessage) (any, error) { return 9, nil })
	host.Register("Host.InputBox", func(_ msgpack.RawMessage) (any, error) { return "answer", nil })
	host.Register("Host.Menu", func(_ msgpack.RawMessage) (any, error) { return 3, nil })
}

type basePlugin struct {
	commandID string
	closed    string
}

func (p *basePlugin) Init(host *Host) ([]string, error) {
	host.Log("log")
	host.Message("message")
	if got := host.GetVersion(); got != "test-version" {
		return nil, errors.New("unexpected host version")
	}
	if !host.RunAction("action") {
		return nil, errors.New("host action was not accepted")
	}
	host.RegisterHighlighter()
	host.RegisterGlobalHotkey(12, 34)
	host.RunProgressTask("title", "start", true, func(string, int) {})
	host.UpdateProgress("update", 50)
	if host.IsProgressCancelled() {
		return nil, errors.New("progress unexpectedly cancelled")
	}
	choice, remember := host.AskOverwrite("path", VFSItem{Name: "src"}, VFSItem{Name: "dst"})
	if choice != 7 || !remember {
		return nil, errors.New("unexpected overwrite answer")
	}
	if got := host.AskError("read", errors.New("failure")); got != 9 {
		return nil, errors.New("unexpected error answer")
	}
	if got := host.InputBox("title", "prompt", "default"); got != "answer" {
		return nil, errors.New("unexpected input answer")
	}
	if got := host.Menu("menu", []string{"one", "two"}); got != 3 {
		return nil, errors.New("unexpected menu answer")
	}
	return []string{"base:"}, nil
}

func (p *basePlugin) ReadDir(string, string) ([]VFSItem, error) {
	return []VFSItem{{Name: "entry", Size: 4}}, nil
}

func (p *basePlugin) Stat(string, string) (VFSItem, error) {
	return VFSItem{Name: "stat", IsDir: true}, nil
}

func (p *basePlugin) Open(string, string) (uint32, int64, error) {
	return 17, 99, nil
}

func (p *basePlugin) ReadAt(uint32, int, int64) ([]byte, error) {
	return []byte("data"), nil
}

func (p *basePlugin) Create(string, string) (uint32, error) {
	return 18, nil
}

func (p *basePlugin) Write(uint32, []byte) error { return nil }

func (p *basePlugin) CloseFile(fileID uint32) error {
	p.closed = "file"
	if fileID != 18 {
		return errors.New("unexpected file id")
	}
	return nil
}

func (p *basePlugin) MkDir(string, string) error  { return nil }
func (p *basePlugin) Remove(string, string) error { return nil }

func (p *basePlugin) Rename(string, string, string) error { return nil }

func (p *basePlugin) Highlight(string, any, uint64) ([]uint64, any, error) {
	return []uint64{1, 2}, "next", nil
}

func (p *basePlugin) ProcessKey(string, vtinput.InputEvent) (bool, error) {
	return true, nil
}

func (p *basePlugin) OnHotkey(uint16, uint32) error { return nil }

func (p *basePlugin) OnProgressTask() error { return nil }

type providerPlugin struct{ basePlugin }

func (p *providerPlugin) PluginCommands() []PluginCommand {
	return []PluginCommand{{ID: "command", Label: "Command"}}
}

func (p *providerPlugin) RunPluginCommand(id string) error {
	p.commandID = id
	return nil
}

func (p *providerPlugin) PanelDescriptors() []PanelDescriptor {
	return []PanelDescriptor{{ID: "panel", Title: "Panel"}}
}

func (p *providerPlugin) OpenPanel(id string, _ PanelContext) ([]byte, error) {
	if id != "panel" {
		return nil, errors.New("unexpected panel id")
	}
	return []byte(`{"type":"panel"}`), nil
}

func (p *providerPlugin) HandlePanelEvent(request PanelEventRequest) (PanelEventResponse, error) {
	if request.ID != "panel" || request.Kind != "key" {
		return PanelEventResponse{}, errors.New("unexpected panel event")
	}
	return PanelEventResponse{Handled: true, Close: true}, nil
}

func (p *providerPlugin) ClosePanel(id string) error {
	if id != "panel" {
		return errors.New("unexpected panel id")
	}
	return nil
}

func TestRunRegistersBasePluginRPCMethods(t *testing.T) {
	plugin := &basePlugin{}
	h := newHarness(t, plugin)

	var drives []string
	if err := h.client.Call("Plugin.Init", nil, &drives); err != nil {
		t.Fatalf("Plugin.Init: %v", err)
	}
	if len(drives) != 1 || drives[0] != "base:" {
		t.Fatalf("drives = %#v", drives)
	}

	var items []VFSItem
	if err := h.client.Call("VFS.ReadDir", map[string]string{"Drive": "base:", "Path": "/"}, &items); err != nil {
		t.Fatalf("VFS.ReadDir: %v", err)
	}
	if len(items) != 1 || items[0].Name != "entry" {
		t.Fatalf("items = %#v", items)
	}
	var stat VFSItem
	if err := h.client.Call("VFS.Stat", map[string]string{"Drive": "base:", "Path": "/entry"}, &stat); err != nil {
		t.Fatalf("VFS.Stat: %v", err)
	}
	if !stat.IsDir {
		t.Fatal("stat should describe a directory")
	}

	var opened OpenRes
	if err := h.client.Call("VFS.Open", OpenReq{Drive: "base:", Path: "/entry"}, &opened); err != nil {
		t.Fatalf("VFS.Open: %v", err)
	}
	if opened.ID != 17 || opened.Size != 99 {
		t.Fatalf("opened = %#v", opened)
	}
	var data []byte
	if err := h.client.Call("VFS.ReadAt", ReadAtReq{ID: 17, Len: 4, Off: 0}, &data); err != nil {
		t.Fatalf("VFS.ReadAt: %v", err)
	}
	if string(data) != "data" {
		t.Fatalf("data = %q", data)
	}
	var created OpenRes
	if err := h.client.Call("VFS.Create", OpenReq{Drive: "base:", Path: "/new"}, &created); err != nil {
		t.Fatalf("VFS.Create: %v", err)
	}
	if created.ID != 18 {
		t.Fatalf("created = %#v", created)
	}
	for method, request := range map[string]any{
		"VFS.Write":  WriteReq{ID: 18, Data: []byte("x")},
		"VFS.MkDir":  MkDirReq{Drive: "base:", Path: "/dir"},
		"VFS.Remove": RemoveReq{Drive: "base:", Path: "/old"},
		"VFS.Rename": RenameReq{Drive: "base:", Old: "/old", New: "/new"},
	} {
		if err := h.client.Call(method, request, nil); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	if err := h.client.Call("VFS.CloseFile", CloseReq{ID: 18}, nil); err != nil {
		t.Fatalf("VFS.CloseFile: %v", err)
	}
	if plugin.closed != "file" {
		t.Fatalf("closed = %q", plugin.closed)
	}

	var highlighted HighlightRes
	if err := h.client.Call("VFS.Highlight", HighlightReq{Line: "line", Base: 1}, &highlighted); err != nil {
		t.Fatalf("VFS.Highlight: %v", err)
	}
	if len(highlighted.Attrs) != 2 || highlighted.Next != "next" {
		t.Fatalf("highlighted = %#v", highlighted)
	}
	var handled bool
	if err := h.client.Call("VFS.ProcessKey", struct {
		Drive string
		Event vtinput.InputEvent
	}{Drive: "base:"}, &handled); err != nil {
		t.Fatalf("VFS.ProcessKey: %v", err)
	}
	if !handled {
		t.Fatal("ProcessKey was not handled")
	}
	if err := h.client.Call("Plugin.OnHotkey", HotkeyReq{VK: 1, Mods: 2}, nil); err != nil {
		t.Fatalf("Plugin.OnHotkey: %v", err)
	}
	if err := h.client.Call("Plugin.OnProgressTask", nil, nil); err != nil {
		t.Fatalf("Plugin.OnProgressTask: %v", err)
	}

	for method, request := range map[string]any{
		"Plugin.RunCommand": PluginRunCommandRequest{ID: "missing"},
		"Plugin.OpenPanel":  struct{ ID string }{ID: "missing"},
		"Plugin.PanelEvent": PanelEventRequest{ID: "missing"},
		"Plugin.ClosePanel": struct{ ID string }{ID: "missing"},
	} {
		err := h.client.Call(method, request, nil)
		if err == nil || !strings.Contains(err.Error(), "does not implement") {
			t.Fatalf("%s error = %v", method, err)
		}
	}
}

func TestRunRegistersProviderExtensions(t *testing.T) {
	plugin := &providerPlugin{}
	h := newHarness(t, plugin)

	var initRes struct {
		Drives   []string
		Commands []PluginCommand
		Panels   []PanelDescriptor
	}
	if err := h.client.Call("Plugin.Init", nil, &initRes); err != nil {
		t.Fatalf("Plugin.Init: %v", err)
	}
	if len(initRes.Commands) != 1 || initRes.Commands[0].ID != "command" {
		t.Fatalf("commands = %#v", initRes.Commands)
	}
	if len(initRes.Panels) != 1 || initRes.Panels[0].ID != "panel" {
		t.Fatalf("panels = %#v", initRes.Panels)
	}

	if err := h.client.Call("Plugin.RunCommand", PluginRunCommandRequest{ID: "command"}, nil); err != nil {
		t.Fatalf("Plugin.RunCommand: %v", err)
	}
	if plugin.commandID != "command" {
		t.Fatalf("commandID = %q", plugin.commandID)
	}
	var opened struct{ Document []byte }
	if err := h.client.Call("Plugin.OpenPanel", struct {
		ID      string
		Context PanelContext
	}{ID: "panel"}, &opened); err != nil {
		t.Fatalf("Plugin.OpenPanel: %v", err)
	}
	if string(opened.Document) != `{"type":"panel"}` {
		t.Fatalf("document = %q", opened.Document)
	}
	var event PanelEventResponse
	if err := h.client.Call("Plugin.PanelEvent", PanelEventRequest{ID: "panel", Kind: "key"}, &event); err != nil {
		t.Fatalf("Plugin.PanelEvent: %v", err)
	}
	if !event.Handled || !event.Close {
		t.Fatalf("event = %#v", event)
	}
	if err := h.client.Call("Plugin.ClosePanel", struct{ ID string }{ID: "panel"}, nil); err != nil {
		t.Fatalf("Plugin.ClosePanel: %v", err)
	}
}
