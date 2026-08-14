package netfox

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

type netFoxPluginTestRegistration struct {
	unregistered int
}

func (registration *netFoxPluginTestRegistration) Unregister() {
	registration.unregistered++
}

type netFoxPluginTestHost struct {
	commands      []vfs.PluginCommand
	registrations []*netFoxPluginTestRegistration
	registerCalls int
	failAtCall    int
	uriCalls      int
	uriErr        error
	driveName     string
	driveFactory  func() vfs.VFS
}

func (*netFoxPluginTestHost) GetVersion() string                           { return "test" }
func (*netFoxPluginTestHost) Log(string)                                   {}
func (*netFoxPluginTestHost) Message(string)                               {}
func (*netFoxPluginTestHost) RegisterHighlighter(vtui.HighlighterProvider) {}
func (*netFoxPluginTestHost) RegisterVFSProvider(vfs.VFSProvider)          {}
func (host *netFoxPluginTestHost) RegisterURIProvider(vfs.URIProvider) error {
	host.uriCalls++
	return host.uriErr
}
func (host *netFoxPluginTestHost) RegisterDrive(name string, factory func() vfs.VFS) {
	host.driveName = name
	host.driveFactory = factory
}
func (*netFoxPluginTestHost) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {
}
func (*netFoxPluginTestHost) RegisterPluginMenuItem(string, func(vfs.App)) {}
func (*netFoxPluginTestHost) RunAction(string) bool                        { return false }

func (*netFoxPluginTestHost) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	panic("unexpected Quick View registration")
}

func (host *netFoxPluginTestHost) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	host.registerCalls++
	if host.registerCalls == host.failAtCall {
		return nil, errors.New("injected command registration failure")
	}
	registration := &netFoxPluginTestRegistration{}
	host.commands = append(host.commands, command)
	host.registrations = append(host.registrations, registration)
	return registration, nil
}

func (*netFoxPluginTestHost) RegisterCommandPrefix(string, string, func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	panic("unexpected command-prefix registration")
}

func (*netFoxPluginTestHost) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	panic("unexpected macro registration")
}

type netFoxPluginTestApp struct {
	active   vfs.VFS
	selected string
}

func (app *netFoxPluginTestApp) GetActivePanelVFS() vfs.VFS { return app.active }
func (*netFoxPluginTestApp) GetPassivePanelVFS() vfs.VFS    { return nil }
func (*netFoxPluginTestApp) GetSelectedNames() []string     { return nil }
func (app *netFoxPluginTestApp) GetSelectedName() string    { return app.selected }
func (*netFoxPluginTestApp) RefreshAll()                    {}
func (*netFoxPluginTestApp) SetPendingSelection(string)     {}
func (*netFoxPluginTestApp) RunProgressTask(string, string, bool, func(context.Context, func(string, int)) error, func(error)) {
}
func (*netFoxPluginTestApp) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
}
func (*netFoxPluginTestApp) Message(string, string, []string) int { return 0 }
func (*netFoxPluginTestApp) InputBox(string, string, string, func(string)) {
}
func (*netFoxPluginTestApp) Menu(string, []string, func(int)) {}

func TestNetFoxPluginRegistersContextualPanelCommands(t *testing.T) {
	host := &netFoxPluginTestHost{}
	plugin := &NetFoxPlugin{}
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}

	if host.uriCalls != 1 {
		t.Fatalf("URI registrations = %d, want 1", host.uriCalls)
	}
	if host.driveName != "NetFox" || host.driveFactory == nil {
		t.Fatalf("drive registration = %q, hasFactory=%t", host.driveName, host.driveFactory != nil)
	}
	if len(host.commands) != 2 {
		t.Fatalf("registered commands = %d, want 2", len(host.commands))
	}

	commands := make(map[string]vfs.PluginCommand, len(host.commands))
	for _, command := range host.commands {
		commands[command.ID] = command
	}
	want := map[string]struct {
		label          string
		labelKey       string
		descriptionKey string
		shortcut       string
	}{
		netFoxEditConnectionCommandID: {label: "Edit connection", labelKey: "NetFox.Command.EditConnection", descriptionKey: "NetFox.Command.EditConnection.Desc", shortcut: "F4"},
		netFoxAddConnectionCommandID:  {label: "Add connection", labelKey: "NetFox.Command.AddConnection", descriptionKey: "NetFox.Command.AddConnection.Desc", shortcut: "Shift+F4"},
	}
	active := &netFoxPluginTestApp{
		active:   &netFoxVFSWrapper{NetFoxVFS: NewNetFoxVFS(t.TempDir() + "/netfox.json")},
		selected: "example",
	}
	inactive := &netFoxPluginTestApp{active: vfs.NewOSVFS(t.TempDir())}
	for id, expected := range want {
		command, ok := commands[id]
		if !ok {
			t.Errorf("command %q was not registered", id)
			continue
		}
		if command.Location != vfs.PluginCommandPanel || command.Label != expected.label ||
			command.LabelKey != expected.labelKey || command.Description == "" || command.DescriptionKey != expected.descriptionKey ||
			!reflect.DeepEqual(command.SearchKeys, []string{"NetFox.ConnectionTitle"}) || command.Shortcut != expected.shortcut {
			t.Errorf("command %q = %#v, want panel command labelled %q", id, command, expected.label)
		}
		if strings.Contains(command.Label, "&") {
			t.Errorf("discoverable command label contains an accelerator: %q", command.Label)
		}
		if command.Run == nil || command.Visible == nil {
			t.Errorf("command %q lacks Run or Visible", id)
			continue
		}
		if !command.Visible(active) {
			t.Errorf("command %q is hidden on the NetFox root", id)
		}
		if command.Visible(inactive) || command.Visible(nil) {
			t.Errorf("command %q is visible outside the NetFox root", id)
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

func TestNetFoxPluginCommandRegistrationFailureRollsBackBeforeLegacySideEffects(t *testing.T) {
	for failAt := 1; failAt <= 2; failAt++ {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			host := &netFoxPluginTestHost{failAtCall: failAt}
			plugin := &NetFoxPlugin{}
			if err := plugin.Init(host); err == nil {
				t.Fatal("Init succeeded despite injected command registration failure")
			}
			if host.uriCalls != 0 || host.driveName != "" || host.driveFactory != nil {
				t.Fatalf("failed Init left URI/drive side effects: uriCalls=%d drive=%q", host.uriCalls, host.driveName)
			}
			if len(host.registrations) != failAt-1 {
				t.Fatalf("successful registrations = %d, want %d", len(host.registrations), failAt-1)
			}
			for index, registration := range host.registrations {
				if registration.unregistered != 1 {
					t.Errorf("registration %d unregistered %d times, want once", index, registration.unregistered)
				}
			}
			if err := plugin.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNetFoxPluginURIRegistrationFailureRollsBackCommandsAndSkipsDrive(t *testing.T) {
	host := &netFoxPluginTestHost{uriErr: errors.New("injected URI registration failure")}
	plugin := &NetFoxPlugin{}
	if err := plugin.Init(host); err == nil {
		t.Fatal("Init succeeded despite injected URI registration failure")
	}
	if host.uriCalls != 1 || host.driveName != "" || host.driveFactory != nil {
		t.Fatalf("failed URI registration left an unexpected drive: uriCalls=%d drive=%q", host.uriCalls, host.driveName)
	}
	if len(host.registrations) != 2 {
		t.Fatalf("successful command registrations = %d, want 2", len(host.registrations))
	}
	for index, registration := range host.registrations {
		if registration.unregistered != 1 {
			t.Errorf("registration %d unregistered %d times, want once", index, registration.unregistered)
		}
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	for index, registration := range host.registrations {
		if registration.unregistered != 1 {
			t.Errorf("Close repeated rollback for registration %d", index)
		}
	}
}
