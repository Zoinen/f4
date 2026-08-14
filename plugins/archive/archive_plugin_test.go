package archive

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type archivePluginTestRegistration struct {
	unregistered int
}

func (registration *archivePluginTestRegistration) Unregister() {
	registration.unregistered++
}

type archivePluginTestHotkey struct {
	vk      uint16
	mods    vtinput.ControlKeyState
	handler func(vfs.App)
}

type archivePluginLegacyTestHost struct {
	providers int
	hotkeys   []archivePluginTestHotkey
}

func (*archivePluginLegacyTestHost) GetVersion() string                           { return "test" }
func (*archivePluginLegacyTestHost) Log(string)                                   {}
func (*archivePluginLegacyTestHost) Message(string)                               {}
func (*archivePluginLegacyTestHost) RegisterHighlighter(vtui.HighlighterProvider) {}
func (host *archivePluginLegacyTestHost) RegisterVFSProvider(vfs.VFSProvider) {
	host.providers++
}
func (*archivePluginLegacyTestHost) RegisterURIProvider(vfs.URIProvider) error { return nil }
func (*archivePluginLegacyTestHost) RegisterDrive(string, func() vfs.VFS)      {}
func (host *archivePluginLegacyTestHost) RegisterGlobalHotkey(vk uint16, mods vtinput.ControlKeyState, handler func(vfs.App)) {
	host.hotkeys = append(host.hotkeys, archivePluginTestHotkey{vk: vk, mods: mods, handler: handler})
}
func (*archivePluginLegacyTestHost) RegisterPluginMenuItem(string, func(vfs.App)) {}
func (*archivePluginLegacyTestHost) RunAction(string) bool                        { return false }

type archivePluginContributionTestHost struct {
	*archivePluginLegacyTestHost
	commands      []vfs.PluginCommand
	registrations []*archivePluginTestRegistration
	registerCalls int
	failAtCall    int
}

func (host *archivePluginContributionTestHost) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	host.registerCalls++
	if host.registerCalls == host.failAtCall {
		return nil, errors.New("injected registration failure")
	}
	registration := &archivePluginTestRegistration{}
	host.commands = append(host.commands, command)
	host.registrations = append(host.registrations, registration)
	return registration, nil
}

func (*archivePluginContributionTestHost) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	panic("unexpected Quick View registration")
}

func (*archivePluginContributionTestHost) RegisterCommandPrefix(string, string, func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	panic("unexpected command-prefix registration")
}

func (*archivePluginContributionTestHost) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	panic("unexpected macro registration")
}

func TestArchivePluginRegistrationFailureLeavesNoLegacySideEffects(t *testing.T) {
	host := &archivePluginContributionTestHost{
		archivePluginLegacyTestHost: &archivePluginLegacyTestHost{},
		failAtCall:                  2,
	}
	plugin := &ArchivePlugin{}
	if err := plugin.Init(host); err == nil {
		t.Fatal("Init succeeded despite injected command registration failure")
	}
	if host.providers != 0 || len(host.hotkeys) != 0 {
		t.Fatalf("failed Init left providers=%d hotkeys=%d", host.providers, len(host.hotkeys))
	}
	if len(host.registrations) != 1 || host.registrations[0].unregistered != 1 {
		t.Fatalf("partial rich registration was not rolled back: %#v", host.registrations)
	}
}

type archivePluginMenuTestApp struct {
	title string
	items []string
}

func (*archivePluginMenuTestApp) GetActivePanelVFS() vfs.VFS  { return nil }
func (*archivePluginMenuTestApp) GetPassivePanelVFS() vfs.VFS { return nil }
func (*archivePluginMenuTestApp) GetSelectedNames() []string  { return nil }
func (*archivePluginMenuTestApp) GetSelectedName() string     { return "" }
func (*archivePluginMenuTestApp) RefreshAll()                 {}
func (*archivePluginMenuTestApp) SetPendingSelection(string)  {}
func (*archivePluginMenuTestApp) RunProgressTask(string, string, bool, func(context.Context, func(string, int)) error, func(error)) {
}
func (*archivePluginMenuTestApp) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
}
func (*archivePluginMenuTestApp) Message(string, string, []string) int { return 0 }
func (*archivePluginMenuTestApp) InputBox(string, string, string, func(string)) {
}
func (app *archivePluginMenuTestApp) Menu(title string, items []string, _ func(int)) {
	app.title = title
	app.items = append([]string(nil), items...)
}

func TestArchivePluginRegistersDiscoverableCommands(t *testing.T) {
	host := &archivePluginContributionTestHost{archivePluginLegacyTestHost: &archivePluginLegacyTestHost{}}
	plugin := &ArchivePlugin{}
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}

	if len(host.commands) != 2 {
		t.Fatalf("registered commands = %d, want 2", len(host.commands))
	}
	want := []struct {
		id             string
		label          string
		labelKey       string
		descriptionKey string
	}{
		{id: archiveAddCommandID, label: "Add to archive", labelKey: "Archive.Command.Add", descriptionKey: "Archive.Command.Add.Desc"},
		{id: archiveExtractCommandID, label: "Extract files", labelKey: "Archive.Command.Extract", descriptionKey: "Archive.Command.Extract.Desc"},
	}
	for index, expected := range want {
		command := host.commands[index]
		if command.ID != expected.id || command.Location != vfs.PluginCommandPanel || command.Label != expected.label ||
			command.LabelKey != expected.labelKey || command.Description == "" || command.DescriptionKey != expected.descriptionKey ||
			!reflect.DeepEqual(command.SearchKeys, []string{"Attributes.Archive"}) || command.Shortcut != "" {
			t.Errorf("command %d = %#v, want id=%q location=Panel label=%q", index, command, expected.id, expected.label)
		}
		if strings.Contains(command.Label, "&") {
			t.Errorf("discoverable command label contains an accelerator: %q", command.Label)
		}
		if command.Run == nil {
			t.Errorf("command %q has no handler", command.ID)
		}
	}

	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	for index, registration := range host.registrations {
		if registration.unregistered != 1 {
			t.Errorf("registration %d unregistered %d times, want once", index, registration.unregistered)
		}
	}
}

func TestArchivePluginKeepsShiftF1MenuWithoutContributionHost(t *testing.T) {
	host := &archivePluginLegacyTestHost{}
	plugin := &ArchivePlugin{}
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	defer plugin.Close()

	if host.providers != 1 {
		t.Fatalf("registered archive providers = %d, want 1", host.providers)
	}
	if len(host.hotkeys) != 1 {
		t.Fatalf("registered global hotkeys = %d, want 1", len(host.hotkeys))
	}
	hotkey := host.hotkeys[0]
	if hotkey.vk != vtinput.VK_F1 || hotkey.mods != vtinput.ShiftPressed || hotkey.handler == nil {
		t.Fatalf("legacy archive hotkey = %#v, want Shift+F1 with a handler", hotkey)
	}

	app := &archivePluginMenuTestApp{}
	hotkey.handler(app)
	if app.title != " Archive Commands " {
		t.Errorf("legacy menu title = %q", app.title)
	}
	wantItems := []string{"&1. Add to archive", "&2. Extract files"}
	if len(app.items) != len(wantItems) {
		t.Fatalf("legacy menu items = %#v", app.items)
	}
	for index := range wantItems {
		if app.items[index] != wantItems[index] {
			t.Errorf("legacy menu item %d = %q, want %q", index, app.items[index], wantItems[index])
		}
	}
}
