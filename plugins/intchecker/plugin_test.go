package intchecker

import (
	"context"
	"errors"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type hostMock struct {
	legacyLabel   string
	legacyHandler func(vfs.App)
}

func (*hostMock) GetVersion() string                                                  { return "test" }
func (*hostMock) Log(string)                                                          {}
func (*hostMock) Message(string)                                                      {}
func (*hostMock) RegisterHighlighter(vtui.HighlighterProvider)                        {}
func (*hostMock) RegisterVFSProvider(vfs.VFSProvider)                                 {}
func (*hostMock) RegisterURIProvider(vfs.URIProvider) error                           { return nil }
func (*hostMock) RegisterDrive(string, func() vfs.VFS)                                {}
func (*hostMock) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {}
func (host *hostMock) RegisterPluginMenuItem(label string, handler func(vfs.App)) {
	host.legacyLabel, host.legacyHandler = label, handler
}
func (*hostMock) RunAction(string) bool { return false }

type registrationMock struct{ unregistered int }

func (r *registrationMock) Unregister() { r.unregistered++ }

type contributionHostMock struct {
	*hostMock
	commands      []vfs.PluginCommand
	registrations []*registrationMock
	err           error
}

func (*contributionHostMock) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected quick-view registration")
}

func (host *contributionHostMock) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	host.commands = append(host.commands, command)
	if host.err != nil {
		return nil, host.err
	}
	registration := &registrationMock{}
	host.registrations = append(host.registrations, registration)
	return registration, nil
}

// commandByID looks up a registered command; the test fails loudly if it is
// missing rather than comparing against a zero value.
func (host *contributionHostMock) commandByID(t *testing.T, id string) vfs.PluginCommand {
	t.Helper()
	for _, command := range host.commands {
		if command.ID == id {
			return command
		}
	}
	t.Fatalf("command %q was not registered", id)
	return vfs.PluginCommand{}
}

func (*contributionHostMock) RegisterCommandPrefix(string, string, func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	return nil, errors.New("unexpected command-prefix registration")
}

func (*contributionHostMock) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected macro registration")
}

var _ vfs.HostAPI = (*hostMock)(nil)
var _ vfs.ContributionHost = (*contributionHostMock)(nil)

func TestPluginRegistersPanelCommandAndUnregistersIt(t *testing.T) {
	host := &contributionHostMock{hostMock: &hostMock{}}
	plugin := NewPlugin(t.TempDir())
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if host.legacyHandler != nil {
		t.Fatal("rich host also received a legacy menu item")
	}
	if len(host.commands) != 3 {
		t.Fatalf("registered %d commands, want 3", len(host.commands))
	}
	menu := host.commandByID(t, "intchecker.menu")
	if menu.Location != vfs.PluginCommandPanel || menu.LabelKey != "IntChecker.Menu" ||
		menu.DescriptionKey != "IntChecker.Command.Desc" || menu.MenuPath != "" ||
		menu.Run == nil || menu.Enabled == nil {
		t.Fatalf("menu command metadata = %#v", menu)
	}
	// The generate/validate commands are Files-menu entries (f4#1623 part
	// 4), the same pattern plugins/archive uses for "Add to archive" and
	// "Extract files": MenuPath places them in the Files menu, and the
	// host's plugin-command keymap gives each its own assignable hotkey
	// without the plugin hardcoding one.
	generate := host.commandByID(t, "intchecker.generate")
	if generate.Location != vfs.PluginCommandPanel || generate.LabelKey != "IntChecker.Command.Generate" ||
		generate.DescriptionKey != "IntChecker.Command.Generate.Desc" || generate.MenuPath != "Files" ||
		generate.Run == nil || generate.Enabled == nil {
		t.Fatalf("generate command metadata = %#v", generate)
	}
	validate := host.commandByID(t, "intchecker.validate")
	if validate.Location != vfs.PluginCommandPanel || validate.LabelKey != "IntChecker.Command.Validate" ||
		validate.DescriptionKey != "IntChecker.Command.Validate.Desc" || validate.MenuPath != "Files" ||
		validate.Run == nil || validate.Enabled == nil {
		t.Fatalf("validate command metadata = %#v", validate)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if len(plugin.registrations) != 0 || plugin.api != nil {
		t.Fatalf("Close left state behind: registrations=%d", len(plugin.registrations))
	}
	for _, registration := range host.registrations {
		if registration.unregistered != 1 {
			t.Fatalf("registration unregistered %d times, want 1", registration.unregistered)
		}
	}
}

func TestPluginFallsBackToLegacyMenu(t *testing.T) {
	host := &hostMock{}
	if err := NewPlugin(t.TempDir()).Init(host); err != nil {
		t.Fatal(err)
	}
	if host.legacyLabel == "" || host.legacyHandler == nil {
		t.Fatal("legacy host got no menu item")
	}
}

func TestPluginInitFailsWhenRegistrationFails(t *testing.T) {
	host := &contributionHostMock{hostMock: &hostMock{}, err: errors.New("injected")}
	plugin := NewPlugin(t.TempDir())
	if err := plugin.Init(host); err == nil {
		t.Fatal("Init succeeded despite registration failure")
	}
	if plugin.api != nil || host.legacyHandler != nil {
		t.Fatal("failed Init kept state or installed a legacy item")
	}
}

type appMock struct {
	fs       vfs.VFS
	selected []string
	menu     []string
	// cursor is what GetSelectedName returns -- the name f4#1623's
	// showValidate checks to decide between checking the file under the
	// cursor right away or opening the dialog. Empty, the zero value, keeps
	// every existing test's "nothing under the cursor" behaviour.
	cursor string
}

func (a *appMock) GetActivePanelVFS() vfs.VFS  { return a.fs }
func (a *appMock) GetPassivePanelVFS() vfs.VFS { return nil }
func (a *appMock) GetSelectedNames() []string  { return a.selected }
func (a *appMock) GetSelectedName() string     { return a.cursor }
func (a *appMock) RefreshAll()                 {}
func (a *appMock) SetPendingSelection(string)  {}
func (a *appMock) RunProgressTask(string, string, bool, func(context.Context, func(string, int)) error, func(error)) {
}
func (a *appMock) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
}
func (a *appMock) Message(string, string, []string) int          { return 0 }
func (a *appMock) InputBox(string, string, string, func(string)) {}
func (a *appMock) Menu(_ string, items []string, _ func(int))    { a.menu = items }

var _ vfs.App = (*appMock)(nil)

func TestCanRunNeedsAPanelFilesystem(t *testing.T) {
	if canRun(&appMock{}) {
		t.Fatal("enabled without a panel filesystem")
	}
	if !canRun(&appMock{fs: vfs.NewOSVFS(t.TempDir())}) {
		t.Fatal("disabled on a local panel")
	}
}

func TestSelectedFileNamesDropsParentEntry(t *testing.T) {
	got := selectedFileNames(&appMock{selected: []string{"..", "a.txt", "", "b"}})
	if len(got) != 2 || got[0] != "a.txt" || got[1] != "b" {
		t.Fatalf("selectedFileNames = %q", got)
	}
}

func TestMenuOffersGenerateAndValidate(t *testing.T) {
	app := &appMock{}
	NewPlugin(t.TempDir()).showMenu(app)
	if len(app.menu) != 2 || app.menu[menuGenerate] != vtui.Msg("IntChecker.Generate") ||
		app.menu[menuValidate] != vtui.Msg("IntChecker.Validate") {
		t.Fatalf("menu = %q", app.menu)
	}
}

// Calculate and Verify checksum are dimmed with nothing but ".." to work on
// (f4#1356); the plugin's own menu entry only needs a panel.
func TestCanWorkOnSelection(t *testing.T) {
	fs := vfs.NewOSVFS(t.TempDir())
	if canWorkOnSelection(&appMock{}) {
		t.Fatal("enabled without a panel filesystem")
	}
	if canWorkOnSelection(&appMock{fs: fs, selected: nil}) {
		t.Fatal("enabled with nothing under the cursor")
	}
	if canWorkOnSelection(&appMock{fs: fs, selected: []string{".."}}) {
		t.Fatal("enabled with only \"..\" selected")
	}
	if !canWorkOnSelection(&appMock{fs: fs, selected: []string{"a.txt"}}) {
		t.Fatal("disabled on a real file")
	}
}
