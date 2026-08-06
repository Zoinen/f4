package visren

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type hostAPIMock struct {
	label   string
	handler func(vfs.App)
}

func (*hostAPIMock) GetVersion() string                                                  { return "test" }
func (*hostAPIMock) Log(string)                                                          {}
func (*hostAPIMock) Message(string)                                                      {}
func (*hostAPIMock) RegisterHighlighter(vtui.HighlighterProvider)                        {}
func (*hostAPIMock) RegisterVFSProvider(vfs.VFSProvider)                                 {}
func (*hostAPIMock) RegisterDrive(string, func() vfs.VFS)                                {}
func (*hostAPIMock) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {}
func (m *hostAPIMock) RegisterPluginMenuItem(label string, handler func(vfs.App)) {
	m.label, m.handler = label, handler
}
func (*hostAPIMock) RunAction(string) bool { return false }

func TestPluginRegistersF11MenuItem(t *testing.T) {
	host := &hostAPIMock{}
	p := &Plugin{}
	if err := p.Init(host); err != nil {
		t.Fatal(err)
	}
	if host.label == "" || host.handler == nil {
		t.Fatalf("menu registration missing: label=%q handlerSet=%t", host.label, host.handler != nil)
	}
	if hotkey := vtui.ExtractHotkey(host.label); hotkey == 0 {
		t.Fatalf("menu label has no accelerator: %q", host.label)
	}
	clean, _, _ := vtui.ParseAmpersandString(host.label)
	if clean != "Visual File Renamer" {
		t.Fatalf("menu label=%q, want Visual File Renamer", clean)
	}
}

func TestSelectedForRename(t *testing.T) {
	marked := []string{"one.txt", "two.txt"}
	if got := selectedForRename(marked, "cursor.txt"); len(got) != 2 || got[0] != marked[0] || got[1] != marked[1] {
		t.Fatalf("marked selection=%v", got)
	}
	if got := selectedForRename(nil, "cursor.txt"); len(got) != 1 || got[0] != "cursor.txt" {
		t.Fatalf("cursor fallback=%v", got)
	}
	if got := selectedForRename(nil, ".."); got != nil {
		t.Fatalf("parent entry must not be selected: %v", got)
	}
}
