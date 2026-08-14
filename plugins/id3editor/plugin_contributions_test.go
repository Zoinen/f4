package id3editor

import (
	"errors"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type id3HostMock struct {
	legacyLabel   string
	legacyHandler func(vfs.App)
}

func (*id3HostMock) GetVersion() string                           { return "test" }
func (*id3HostMock) Log(string)                                   {}
func (*id3HostMock) Message(string)                               {}
func (*id3HostMock) RegisterHighlighter(vtui.HighlighterProvider) {}
func (*id3HostMock) RegisterVFSProvider(vfs.VFSProvider)          {}
func (*id3HostMock) RegisterURIProvider(vfs.URIProvider) error    { return nil }
func (*id3HostMock) RegisterDrive(string, func() vfs.VFS)         {}
func (*id3HostMock) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {
}
func (host *id3HostMock) RegisterPluginMenuItem(label string, handler func(vfs.App)) {
	host.legacyLabel, host.legacyHandler = label, handler
}
func (*id3HostMock) RunAction(string) bool { return false }

type id3RegistrationMock struct{ unregistered int }

func (registration *id3RegistrationMock) Unregister() {
	registration.unregistered++
}

type id3ContributionHostMock struct {
	*id3HostMock
	command      vfs.PluginCommand
	registration *id3RegistrationMock
	err          error
}

func (*id3ContributionHostMock) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected quick-view registration")
}

func (host *id3ContributionHostMock) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	host.command = command
	if host.err != nil {
		return nil, host.err
	}
	if host.registration == nil {
		host.registration = &id3RegistrationMock{}
	}
	return host.registration, nil
}

func (*id3ContributionHostMock) RegisterCommandPrefix(string, string, func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	return nil, errors.New("unexpected command-prefix registration")
}

func (*id3ContributionHostMock) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected macro registration")
}

func TestPluginUsesLegacyMenuForHostsWithoutRichContributions(t *testing.T) {
	host := &id3HostMock{}
	plugin := &ID3EditorPlugin{}
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if host.legacyLabel == "" || host.legacyHandler == nil {
		t.Fatal("legacy host did not receive an ID3 editor menu item")
	}
}

func TestPluginPrefersRichCommandAndUnregistersIt(t *testing.T) {
	host := &id3ContributionHostMock{id3HostMock: &id3HostMock{}}
	plugin := &ID3EditorPlugin{}
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if host.legacyLabel != "" || host.legacyHandler != nil {
		t.Fatal("rich host also received a duplicate legacy menu item")
	}
	command := host.command
	if command.ID != "id3editor.edit" || command.Location != vfs.PluginCommandPanel ||
		command.Label != "ID3 Tag &Editor" || command.LabelKey != "ID3Editor.Menu" ||
		command.Description == "" || command.DescriptionKey != "ID3Editor.Command.Edit.Desc" ||
		len(command.SearchKeys) != 3 || command.Run == nil {
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
	if plugin.api != nil || plugin.registration != nil {
		t.Fatal("Close retained host or registration state")
	}
}

func TestPluginDoesNotFallBackToLegacyMenuAfterRichRegistrationFailure(t *testing.T) {
	host := &id3ContributionHostMock{
		id3HostMock: &id3HostMock{},
		err:         errors.New("injected registration failure"),
	}
	plugin := &ID3EditorPlugin{}
	if err := plugin.Init(host); err == nil {
		t.Fatal("Init succeeded despite rich registration failure")
	}
	if host.legacyLabel != "" || host.legacyHandler != nil {
		t.Fatal("failed rich registration silently installed a legacy menu item")
	}
	if plugin.api != nil || plugin.registration != nil {
		t.Fatal("failed Init retained host or registration state")
	}
}

var _ vfs.HostAPI = (*id3HostMock)(nil)
var _ vfs.ContributionHost = (*id3ContributionHostMock)(nil)
