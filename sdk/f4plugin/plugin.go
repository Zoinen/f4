package f4plugin

import (
	"fmt"
	"os"
	"time"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/vtinput"
	"github.com/vmihailenco/msgpack/v5"
)

// Host provides methods for the plugin to interact with the main f4 application.
type Host struct {
	sess       *f4rpc.Session
	onProgress func(msg string, percent int)
}

func (h *Host) Log(msg string) {
	_ = h.sess.Call("Host.Log", msg, nil)
}

func (h *Host) Message(msg string) {
	_ = h.sess.Call("Host.Message", msg, nil)
}

func (h *Host) GetVersion() string {
	var ver string
	_ = h.sess.Call("Host.GetVersion", nil, &ver)
	return ver
}
func (h *Host) RunAction(name string) bool {
	var res bool
	_ = h.sess.Call("Host.RunAction", name, &res)
	return res
}

// VFSItem mirrors the core's vfs.VFSItem format.
type VFSItem struct {
	Name         string
	Size         int64
	IsDir        bool
	MTime        time.Time
	Mode         string
	IsExecutable bool
	IsHidden     bool
}

type OpenReq struct{ Drive, Path string }
type OpenRes struct {
	ID   uint32
	Size int64
}
type ReadAtReq struct {
	ID  uint32
	Len int
	Off int64
}
type CloseReq struct{ ID uint32 }
type WriteReq struct {
	ID   uint32
	Data []byte
}
type MkDirReq struct{ Drive, Path string }
type RemoveReq struct{ Drive, Path string }
type RenameReq struct{ Drive, Old, New string }
type HighlightReq struct {
	Line string
	Prev any
	Base uint64
}
type HighlightRes struct {
	Attrs []uint64
	Next  any
}
type ProgressTaskReq struct {
	Title, StartMsg string
	Forked          bool
}
type ProgressUpdateReq struct {
	Msg     string
	Percent int
}
type HotkeyReq struct {
	VK   uint16
	Mods uint32
}
type AskOverwriteReq struct {
	Path     string
	Src, Dst VFSItem
}
type AskOverwriteRes struct {
	Choice   int
	Remember bool
}
type AskErrorReq struct {
	Op  string
	Err string
}
type InputBoxReq struct{ Title, Prompt, Default string }
type MenuReq struct {
	Title string
	Items []string
}

const (
	PluginCommandPanel uint8 = iota
	PluginCommandConfig
)

// PluginCommand describes one searchable command exposed by an RPC, Lua or
// WASM plugin. Localized maps use the same short language codes as f4's
// language packs (for example "en" and "ru"). ActiveDrives restricts a panel
// command to those RPC drives without requiring a synchronous remote Visible
// callback on f4's UI goroutine.
type PluginCommand struct {
	ID                    string
	Location              uint8
	Label                 string
	Description           string
	Shortcut              string
	LocalizedLabels       map[string]string
	LocalizedDescriptions map[string]string
	SearchTerms           []string
	ActiveDrives          []string
}

type PluginRunCommandRequest struct {
	ID string
}

// PanelState and PanelContext are the transport-safe snapshot delivered to a
// panel plugin. They intentionally contain no host pointers or VFS handles.
type PanelState struct {
	Side          int
	Active        bool
	Path          string
	SelectedName  string
	SelectedNames []string
}

type PanelContext struct {
	Side       int
	ActiveSide int
	Bounds     [4]int
	Current    PanelState
	Other      PanelState
}

type PanelDescriptor struct {
	ID          string
	Title       string
	Description string
}

type PanelEventRequest struct {
	ID      string
	Kind    string
	Event   vtinput.InputEvent
	Context PanelContext
}

type PanelEventResponse struct {
	Handled bool
	// Document is UTF-8 JSON in the vtui .vui format. An empty value keeps the
	// current document, which is useful for events that only need consuming.
	Document []byte
	Close    bool
}

// PanelProvider is an optional extension for plugins that own a full panel
// surface. The host sends raw input and a fresh context snapshot; the plugin
// may return a new .vui document after each event.
type PanelProvider interface {
	PanelDescriptors() []PanelDescriptor
	OpenPanel(id string, context PanelContext) ([]byte, error)
	HandlePanelEvent(request PanelEventRequest) (PanelEventResponse, error)
	ClosePanel(id string) error
}

// CommandProvider is an optional extension. Existing Plugin implementations
// remain source-compatible; implementations that opt in get commands in f4's
// plugin menus and command palette.
type CommandProvider interface {
	PluginCommands() []PluginCommand
	RunPluginCommand(id string) error
}

// Plugin is the primary interface a plugin developer implements.
type Plugin interface {
	Init(host *Host) ([]string, error)
	ReadDir(drive, path string) ([]VFSItem, error)
	Stat(drive, path string) (VFSItem, error)
	Open(drive, path string) (uint32, int64, error)
	ReadAt(fileID uint32, length int, offset int64) ([]byte, error)
	Create(drive, path string) (uint32, error)
	Write(fileID uint32, data []byte) error
	CloseFile(fileID uint32) error
	MkDir(drive, path string) error
	Remove(drive, path string) error
	Rename(drive, old, new string) error

	// Optional extensions
	Highlight(line string, prev any, base uint64) ([]uint64, any, error)
	ProcessKey(drive string, event vtinput.InputEvent) (bool, error)
	OnHotkey(vk uint16, mods uint32) error
	OnProgressTask() error
}

// Run attaches the plugin to stdin/stdout and starts the RPC server loop.
func Run(p Plugin) {
	sess := f4rpc.NewSession(os.Stdin, os.Stdout)
	sess.OnError = func(err error) {
		_, _ = fmt.Fprintf(os.Stderr, "f4rpc: %v\n", err)
	}
	host := &Host{sess: sess}

	sess.Register("Plugin.Init", func(data msgpack.RawMessage) (any, error) {
		drives, err := p.Init(host)
		if commands, ok := p.(CommandProvider); ok {
			response := map[string]any{
				"Drives":   drives,
				"Commands": commands.PluginCommands(),
			}
			if panels, ok := p.(PanelProvider); ok {
				response["Panels"] = panels.PanelDescriptors()
			}
			return response, err
		}
		if panels, ok := p.(PanelProvider); ok {
			return map[string]any{
				"Drives": drives,
				"Panels": panels.PanelDescriptors(),
			}, err
		}
		// Preserve the original wire shape unless the plugin opts into the
		// command extension, so existing hosts can still load it.
		return drives, err
	})

	sess.Register("Plugin.RunCommand", func(data msgpack.RawMessage) (any, error) {
		commands, ok := p.(CommandProvider)
		if !ok {
			return nil, fmt.Errorf("plugin does not implement CommandProvider")
		}
		var request PluginRunCommandRequest
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		return nil, commands.RunPluginCommand(request.ID)
	})

	sess.Register("Plugin.OpenPanel", func(data msgpack.RawMessage) (any, error) {
		panels, ok := p.(PanelProvider)
		if !ok {
			return nil, fmt.Errorf("plugin does not implement PanelProvider")
		}
		var request struct {
			ID      string
			Context PanelContext
		}
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		document, err := panels.OpenPanel(request.ID, request.Context)
		if err != nil {
			return nil, err
		}
		return map[string]any{"Document": document}, nil
	})

	sess.Register("Plugin.PanelEvent", func(data msgpack.RawMessage) (any, error) {
		panels, ok := p.(PanelProvider)
		if !ok {
			return nil, fmt.Errorf("plugin does not implement PanelProvider")
		}
		var request PanelEventRequest
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		return panels.HandlePanelEvent(request)
	})

	sess.Register("Plugin.ClosePanel", func(data msgpack.RawMessage) (any, error) {
		panels, ok := p.(PanelProvider)
		if !ok {
			return nil, fmt.Errorf("plugin does not implement PanelProvider")
		}
		var request struct{ ID string }
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		return nil, panels.ClosePanel(request.ID)
	})

	sess.Register("VFS.ReadDir", func(data msgpack.RawMessage) (any, error) {
		var req map[string]string
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		items, err := p.ReadDir(req["Drive"], req["Path"])
		return items, err
	})

	sess.Register("VFS.Stat", func(data msgpack.RawMessage) (any, error) {
		var req map[string]string
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		item, err := p.Stat(req["Drive"], req["Path"])
		return item, err
	})
	sess.Register("VFS.Open", func(data msgpack.RawMessage) (any, error) {
		var req OpenReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		id, size, err := p.Open(req.Drive, req.Path)
		return OpenRes{ID: id, Size: size}, err
	})

	sess.Register("VFS.ReadAt", func(data msgpack.RawMessage) (any, error) {
		var req ReadAtReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return p.ReadAt(req.ID, req.Len, req.Off)
	})

	sess.Register("VFS.CloseFile", func(data msgpack.RawMessage) (any, error) {
		var req CloseReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return nil, p.CloseFile(req.ID)
	})
	sess.Register("VFS.Create", func(data msgpack.RawMessage) (any, error) {
		var req OpenReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		id, err := p.Create(req.Drive, req.Path)
		return OpenRes{ID: id}, err
	})

	sess.Register("VFS.Write", func(data msgpack.RawMessage) (any, error) {
		var req WriteReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return nil, p.Write(req.ID, req.Data)
	})

	sess.Register("VFS.MkDir", func(data msgpack.RawMessage) (any, error) {
		var req MkDirReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return nil, p.MkDir(req.Drive, req.Path)
	})

	sess.Register("VFS.Remove", func(data msgpack.RawMessage) (any, error) {
		var req RemoveReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return nil, p.Remove(req.Drive, req.Path)
	})

	sess.Register("VFS.Rename", func(data msgpack.RawMessage) (any, error) {
		var req RenameReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return nil, p.Rename(req.Drive, req.Old, req.New)
	})
	sess.Register("VFS.Highlight", func(data msgpack.RawMessage) (any, error) {
		var req HighlightReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		attrs, next, err := p.Highlight(req.Line, req.Prev, req.Base)
		return HighlightRes{Attrs: attrs, Next: next}, err
	})

	sess.Register("VFS.ProcessKey", func(data msgpack.RawMessage) (any, error) {
		type PKReq struct {
			Drive string
			Event vtinput.InputEvent
		}
		var req PKReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return p.ProcessKey(req.Drive, req.Event)
	})

	sess.Register("Plugin.OnHotkey", func(data msgpack.RawMessage) (any, error) {
		var req HotkeyReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return nil, p.OnHotkey(req.VK, req.Mods)
	})

	sess.Register("Plugin.OnProgressTask", func(data msgpack.RawMessage) (any, error) {
		return nil, p.OnProgressTask()
	})

	_ = sess.Serve()
}

func (h *Host) RegisterHighlighter() {
	_ = h.sess.Call("Host.RegisterHighlighter", nil, nil)
}

func (h *Host) RegisterGlobalHotkey(vk uint16, mods uint32) {
	_ = h.sess.Call("Host.RegisterGlobalHotkey", HotkeyReq{VK: vk, Mods: mods}, nil)
}

func (h *Host) RunProgressTask(title, startMsg string, forked bool, onUpdate func(msg string, percent int)) {
	h.onProgress = onUpdate
	_ = h.sess.Call("Host.RunProgressTask", ProgressTaskReq{Title: title, StartMsg: startMsg, Forked: forked}, nil)
}

func (h *Host) UpdateProgress(msg string, percent int) {
	_ = h.sess.Call("Host.UpdateProgress", ProgressUpdateReq{Msg: msg, Percent: percent}, nil)
}
func (h *Host) IsProgressCancelled() bool {
	var cancelled bool
	_ = h.sess.Call("Host.IsProgressCancelled", nil, &cancelled)
	return cancelled
}

func (h *Host) AskOverwrite(path string, src, dst VFSItem) (int, bool) {
	var res AskOverwriteRes
	_ = h.sess.Call("Host.AskOverwrite", AskOverwriteReq{Path: path, Src: src, Dst: dst}, &res)
	return res.Choice, res.Remember
}

func (h *Host) AskError(op string, err error) int {
	var res int
	_ = h.sess.Call("Host.AskError", AskErrorReq{Op: op, Err: err.Error()}, &res)
	return res
}
func (h *Host) InputBox(title, prompt, defaultText string) string {
	var res string
	_ = h.sess.Call("Host.InputBox", InputBoxReq{Title: title, Prompt: prompt, Default: defaultText}, &res)
	return res
}

func (h *Host) Menu(title string, items []string) int {
	var res int
	_ = h.sess.Call("Host.Menu", MenuReq{Title: title, Items: items}, &res)
	return res
}
