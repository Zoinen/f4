package cloudfox

import (
	"errors"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type cloudFoxRegistrationMock struct{ unregistered int }

func (registration *cloudFoxRegistrationMock) Unregister() {
	registration.unregistered++
}

type cloudFoxContributionHostMock struct {
	commands      []vfs.PluginCommand
	registrations []*cloudFoxRegistrationMock
	failCommand   int
	uriErr        error
	uriCalls      int
	vfsCalls      int
	driveCalls    int
}

func (*cloudFoxContributionHostMock) GetVersion() string                           { return "test" }
func (*cloudFoxContributionHostMock) Log(string)                                   {}
func (*cloudFoxContributionHostMock) Message(string)                               {}
func (*cloudFoxContributionHostMock) RegisterHighlighter(vtui.HighlighterProvider) {}
func (host *cloudFoxContributionHostMock) RegisterVFSProvider(vfs.VFSProvider) {
	host.vfsCalls++
}
func (host *cloudFoxContributionHostMock) RegisterURIProvider(vfs.URIProvider) error {
	host.uriCalls++
	return host.uriErr
}
func (host *cloudFoxContributionHostMock) RegisterDrive(string, func() vfs.VFS) {
	host.driveCalls++
}
func (*cloudFoxContributionHostMock) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {
}
func (*cloudFoxContributionHostMock) RegisterPluginMenuItem(string, func(vfs.App)) {}
func (*cloudFoxContributionHostMock) RunAction(string) bool                        { return false }

func (*cloudFoxContributionHostMock) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected quick-view registration")
}

func (host *cloudFoxContributionHostMock) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	host.commands = append(host.commands, command)
	if host.failCommand != 0 && len(host.commands) == host.failCommand {
		return nil, errors.New("injected command registration failure")
	}
	registration := &cloudFoxRegistrationMock{}
	host.registrations = append(host.registrations, registration)
	return registration, nil
}

func (*cloudFoxContributionHostMock) RegisterCommandPrefix(string, string, func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	return nil, errors.New("unexpected command-prefix registration")
}

func (*cloudFoxContributionHostMock) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected macro registration")
}

type cloudFoxCommandApp struct {
	vfs.App
	active   vfs.VFS
	selected []string
}

func (app *cloudFoxCommandApp) GetActivePanelVFS() vfs.VFS { return app.active }
func (app *cloudFoxCommandApp) GetSelectedNames() []string { return app.selected }
func (*cloudFoxCommandApp) RefreshAll()                    {}

func TestPluginRegistersContextCommandsAndUnregistersThem(t *testing.T) {
	var edited []string
	plugin := NewPlugin(Options{
		ConfigDir: t.TempDir(),
		Portable:  true,
		Factories: []BackendFactory{},
		Editor: ProfileEditorFunc(func(_ vfs.App, _ *ManagerVFS, existing *Connection) {
			if existing == nil {
				edited = append(edited, "")
			} else {
				edited = append(edited, existing.Name)
			}
		}),
	})
	host := &cloudFoxContributionHostMock{}
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if len(host.commands) != 3 || len(host.registrations) != 3 {
		t.Fatalf("commands=%#v registrations=%d", host.commands, len(host.registrations))
	}
	if host.uriCalls != 1 || host.vfsCalls != 1 || host.driveCalls != 1 {
		t.Fatalf("host registrations: URI=%d VFS=%d drive=%d", host.uriCalls, host.vfsCalls, host.driveCalls)
	}

	commands := make(map[string]vfs.PluginCommand, len(host.commands))
	for _, command := range host.commands {
		commands[command.ID] = command
		if command.Location != vfs.PluginCommandPanel || command.LabelKey == "" ||
			command.Description == "" || command.DescriptionKey == "" || command.Visible == nil || command.Run == nil {
			t.Errorf("incomplete command metadata: %#v", command)
		}
	}
	if commands[cloudFoxAddConnectionCommandID].Shortcut != "Shift+F4" ||
		commands[cloudFoxEditConnectionCommandID].Shortcut != "F4" ||
		commands[cloudFoxDeleteConnectionCommandID].Shortcut != "F8" {
		t.Fatalf("command shortcuts = %#v", commands)
	}

	manager := plugin.manager()
	manager.rows["Saved cloud"] = Connection{ID: "saved", Name: "Saved cloud", Provider: ProviderWebDAV}
	app := &cloudFoxCommandApp{active: manager, selected: []string{"Saved cloud"}}
	if !commands[cloudFoxAddConnectionCommandID].Visible(app) ||
		!commands[cloudFoxEditConnectionCommandID].Visible(app) ||
		!commands[cloudFoxDeleteConnectionCommandID].Visible(app) {
		t.Fatal("manager context commands were not visible for a saved connection")
	}
	commands[cloudFoxAddConnectionCommandID].Run(app)
	commands[cloudFoxEditConnectionCommandID].Run(app)
	if len(edited) != 2 || edited[0] != "" || edited[1] != "Saved cloud" {
		t.Fatalf("editor calls = %#v", edited)
	}

	app.selected = []string{manager.strings.AddConnection}
	if commands[cloudFoxEditConnectionCommandID].Visible(app) || commands[cloudFoxDeleteConnectionCommandID].Visible(app) {
		t.Fatal("edit/delete commands accepted the synthetic add-connection row")
	}
	app.active = vfs.NewOSVFS(t.TempDir())
	for id, command := range commands {
		if command.Visible(app) {
			t.Errorf("command %q visible outside CloudFox manager", id)
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
			t.Errorf("registration %d unregistered %d times", index, registration.unregistered)
		}
	}
}

func TestPluginCommandRegistrationFailureRollsBackWithoutGlobalSideEffects(t *testing.T) {
	for failCommand := 1; failCommand <= 3; failCommand++ {
		t.Run(string(rune('0'+failCommand)), func(t *testing.T) {
			plugin := NewPlugin(Options{ConfigDir: t.TempDir(), Portable: true, Factories: []BackendFactory{}})
			host := &cloudFoxContributionHostMock{failCommand: failCommand}
			if err := plugin.Init(host); err == nil {
				t.Fatal("Init succeeded despite command registration failure")
			}
			if host.uriCalls != 0 || host.vfsCalls != 0 || host.driveCalls != 0 {
				t.Fatalf("failed command registration left global side effects: URI=%d VFS=%d drive=%d", host.uriCalls, host.vfsCalls, host.driveCalls)
			}
			for index, registration := range host.registrations {
				if registration.unregistered != 1 {
					t.Errorf("registration %d rollback count = %d", index, registration.unregistered)
				}
			}
			if err := plugin.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPluginURIRegistrationFailureRollsBackContextCommands(t *testing.T) {
	plugin := NewPlugin(Options{ConfigDir: t.TempDir(), Portable: true, Factories: []BackendFactory{}})
	host := &cloudFoxContributionHostMock{uriErr: errors.New("injected URI registration failure")}
	if err := plugin.Init(host); err == nil {
		t.Fatal("Init succeeded despite URI registration failure")
	}
	if host.uriCalls != 1 || host.vfsCalls != 0 || host.driveCalls != 0 {
		t.Fatalf("failed URI registration side effects: URI=%d VFS=%d drive=%d", host.uriCalls, host.vfsCalls, host.driveCalls)
	}
	for index, registration := range host.registrations {
		if registration.unregistered != 1 {
			t.Errorf("registration %d rollback count = %d", index, registration.unregistered)
		}
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
}

var _ vfs.HostAPI = (*cloudFoxContributionHostMock)(nil)
var _ vfs.ContributionHost = (*cloudFoxContributionHostMock)(nil)
