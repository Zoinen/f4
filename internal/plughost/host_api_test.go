package plughost

import (
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// luaTestHostAPI is a vfs.HostAPI that records what a plugin does instead of
// touching the real UI.
type luaTestHostAPI struct {
	logs   []string
	drives map[string]func() vfs.VFS
}

func newLuaTestHostAPI() *luaTestHostAPI {
	return &luaTestHostAPI{drives: make(map[string]func() vfs.VFS)}
}

func (h *luaTestHostAPI) GetVersion() string { return "test-version" }
func (h *luaTestHostAPI) Log(msg string)     { h.logs = append(h.logs, msg) }
func (h *luaTestHostAPI) Message(msg string) { h.logs = append(h.logs, msg) }

func (h *luaTestHostAPI) RegisterHighlighter(p vtui.HighlighterProvider) {}
func (h *luaTestHostAPI) RegisterVFSProvider(p vfs.VFSProvider)          {}
func (h *luaTestHostAPI) RegisterURIProvider(p vfs.URIProvider) error    { return nil }

func (h *luaTestHostAPI) RegisterDrive(name string, factory func() vfs.VFS) {
	h.drives[name] = factory
}

func (h *luaTestHostAPI) RegisterGlobalHotkey(vk uint16, mods vtinput.ControlKeyState, handler func(app vfs.App)) {
}

func (h *luaTestHostAPI) RegisterPluginMenuItem(label string, handler func(app vfs.App)) {}

func (h *luaTestHostAPI) RunAction(name string) bool {
	h.logs = append(h.logs, "action:"+name)
	return true
}
