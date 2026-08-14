package visren

import (
	"errors"
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
func (*hostAPIMock) RegisterURIProvider(vfs.URIProvider) error                           { return nil }
func (*hostAPIMock) RegisterDrive(string, func() vfs.VFS)                                {}
func (*hostAPIMock) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {}
func (m *hostAPIMock) RegisterPluginMenuItem(label string, handler func(vfs.App)) {
	m.label, m.handler = label, handler
}
func (*hostAPIMock) RunAction(string) bool { return false }

type contributionRegistrationMock struct{ unregistered int }

func (registration *contributionRegistrationMock) Unregister() {
	registration.unregistered++
}

type contributionHostMock struct {
	*hostAPIMock
	command      vfs.PluginCommand
	registration *contributionRegistrationMock
	err          error
}

func (*contributionHostMock) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected quick-view registration")
}

func (host *contributionHostMock) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	host.command = command
	if host.err != nil {
		return nil, host.err
	}
	if host.registration == nil {
		host.registration = &contributionRegistrationMock{}
	}
	return host.registration, nil
}

func (*contributionHostMock) RegisterCommandPrefix(string, string, func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	return nil, errors.New("unexpected command-prefix registration")
}

func (*contributionHostMock) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected macro registration")
}

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

func TestPluginPrefersRichCommandAndUnregistersIt(t *testing.T) {
	host := &contributionHostMock{hostAPIMock: &hostAPIMock{}}
	plugin := &Plugin{}
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if host.label != "" || host.handler != nil {
		t.Fatal("rich host also received a duplicate legacy menu item")
	}
	command := host.command
	if command.ID != "visren.open" || command.Location != vfs.PluginCommandPanel ||
		command.Label != "&Visual File Renamer" || command.LabelKey != "VisRen.Menu" ||
		command.Description == "" || command.DescriptionKey != "VisRen.Command.Open.Desc" ||
		len(command.SearchKeys) != 2 || command.Run == nil {
		t.Fatalf("rich command metadata = %#v", command)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if host.registration == nil || host.registration.unregistered != 1 {
		t.Fatalf("unregister calls = %#v", host.registration)
	}
}

func TestPluginDoesNotFallBackToLegacyMenuAfterRichRegistrationFailure(t *testing.T) {
	host := &contributionHostMock{
		hostAPIMock: &hostAPIMock{},
		err:         errors.New("injected registration failure"),
	}
	plugin := &Plugin{}
	if err := plugin.Init(host); err == nil {
		t.Fatal("Init succeeded despite rich registration failure")
	}
	if host.label != "" || host.handler != nil {
		t.Fatal("failed rich registration silently installed a legacy menu item")
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
