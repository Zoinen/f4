package mediainfo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type configDialogApp struct{}

func (configDialogApp) GetActivePanelVFS() vfs.VFS  { return nil }
func (configDialogApp) GetPassivePanelVFS() vfs.VFS { return nil }
func (configDialogApp) GetSelectedNames() []string  { return nil }
func (configDialogApp) GetSelectedName() string     { return "" }
func (configDialogApp) RefreshAll()                 {}
func (configDialogApp) SetPendingSelection(string)  {}
func (configDialogApp) RunProgressTask(string, string, bool, func(context.Context, func(string, int)) error, func(error)) {
}
func (configDialogApp) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
}
func (configDialogApp) Message(string, string, []string) int { return 0 }
func (configDialogApp) InputBox(string, string, string, func(string)) {
}
func (configDialogApp) Menu(string, []string, func(int)) {}

type configDialogFrameApp struct {
	configDialogApp
	vtui.Frame
}

type configDialogControls struct {
	showMenu  *vtui.Checkbox
	quickView *vtui.Checkbox
	useEditor *vtui.Checkbox
	prefix    *vtui.Edit
	language  *vtui.ComboBox
	template  *vtui.Edit
	save      *vtui.Button
	cancel    *vtui.Button
}

func configDialogFrameManager(t *testing.T) *vtui.FrameManagerType {
	t.Helper()
	old := vtui.FrameManager
	fm := vtui.NewFrameManager()
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	fm.Init(screen)
	vtui.FrameManager = fm
	t.Cleanup(func() {
		fm.Shutdown()
		vtui.FrameManager = old
	})
	return fm
}

func openConfigDialog(t *testing.T, plugin *Plugin, app vfs.App) (*vtui.FrameManagerType, *vtui.Window, configDialogControls) {
	t.Helper()
	fm := configDialogFrameManager(t)
	plugin.configure(app)
	dialog, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want *vtui.Window", fm.GetTopFrame())
	}
	var controls configDialogControls
	var buttons []*vtui.Button
	for _, child := range dialog.GetChildren() {
		switch control := child.(type) {
		case *vtui.Checkbox:
			switch {
			case controls.showMenu == nil:
				controls.showMenu = control
			case controls.quickView == nil:
				controls.quickView = control
			case controls.useEditor == nil:
				controls.useEditor = control
			}
		case *vtui.Edit:
			switch {
			case controls.prefix == nil:
				controls.prefix = control
			case controls.template == nil:
				controls.template = control
			}
		case *vtui.ComboBox:
			controls.language = control
		case *vtui.Button:
			buttons = append(buttons, control)
		}
	}
	if controls.showMenu == nil || controls.quickView == nil || controls.useEditor == nil ||
		controls.prefix == nil || controls.language == nil || controls.template == nil || len(buttons) != 2 {
		t.Fatalf("dialog controls are incomplete: %#v, buttons=%d", controls, len(buttons))
	}
	controls.save, controls.cancel = buttons[0], buttons[1]
	return fm, dialog, controls
}

type configDialogPrefix struct {
	prefix string
	err    error
}

func (*configDialogPrefix) Unregister() {}

func (prefix *configDialogPrefix) SetPrefix(value string) error {
	if prefix.err != nil {
		return prefix.err
	}
	prefix.prefix = value
	return nil
}

func expectSettingsError(t *testing.T, fm *vtui.FrameManagerType, dialog *vtui.Window) {
	t.Helper()
	if dialog.IsDone() {
		t.Fatal("dialog closed after rejected settings")
	}
	top := fm.GetTopFrame()
	if top == dialog {
		t.Fatal("settings error dialog was not shown")
	}
	if !strings.Contains(strings.ToLower(top.GetTitle()), "settings") {
		t.Fatalf("error dialog title = %q", top.GetTitle())
	}
}

func TestConfigureBuildsDialogForEachReportLanguage(t *testing.T) {
	for _, test := range []struct {
		name string
		code string
		want int
	}{
		{name: "automatic", code: "auto", want: 0},
		{name: "english", code: "en", want: 1},
		{name: "russian", code: "ru", want: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Language = test.code
			plugin := NewPlugin(t.TempDir())
			plugin.store = &settingsStore{current: settings}
			_, _, controls := openConfigDialog(t, plugin, configDialogApp{})
			if got := controls.language.Menu.SelectPos; got != test.want {
				t.Fatalf("language selection = %d, want %d", got, test.want)
			}
		})
	}
}

func TestConfigureSavesAndCancelsSettings(t *testing.T) {
	plugin := NewPlugin(t.TempDir())
	store := &settingsStore{
		path:    filepath.Join(t.TempDir(), "plugins", "mediainfo.json"),
		current: DefaultSettings(),
	}
	prefix := &configDialogPrefix{prefix: "MediaInfo"}
	plugin.store = store
	plugin.prefix = prefix
	fm, dialog, controls := openConfigDialog(t, plugin, configDialogApp{})
	controls.showMenu.State = 0
	controls.quickView.State = 1
	controls.useEditor.State = 1
	controls.prefix.SetText("Media")
	controls.language.Menu.SetSelectPos(2)
	controls.save.OnClick()
	if !dialog.IsDone() {
		t.Fatal("dialog remained open after successful save")
	}
	got := store.snapshot()
	if got.ShowInPluginMenu || !got.EnableQuickView || !got.UseEditor || got.Prefix != "Media" || got.Language != "ru" {
		t.Fatalf("saved settings = %#v", got)
	}
	if prefix.prefix != "Media" {
		t.Fatalf("registered prefix = %q", prefix.prefix)
	}
	if fm.GetTopFrame() != dialog {
		t.Fatalf("top frame changed unexpectedly after close: %T", fm.GetTopFrame())
	}

	plugin = NewPlugin(t.TempDir())
	store = &settingsStore{current: DefaultSettings()}
	prefix = &configDialogPrefix{prefix: "MediaInfo"}
	plugin.store = store
	plugin.prefix = prefix
	_, dialog, controls = openConfigDialog(t, plugin, configDialogApp{})
	controls.prefix.SetText("Changed")
	controls.cancel.OnClick()
	if !dialog.IsDone() {
		t.Fatal("dialog remained open after cancel")
	}
	if got := store.snapshot(); got.Prefix != "MediaInfo" || prefix.prefix != "MediaInfo" {
		t.Fatalf("cancel changed settings: settings=%#v prefix=%q", got, prefix.prefix)
	}
}

func TestConfigureUsesFrameAnchor(t *testing.T) {
	plugin := NewPlugin(t.TempDir())
	plugin.store = &settingsStore{current: DefaultSettings()}
	fm := configDialogFrameManager(t)
	app := &configDialogFrameApp{Frame: vtui.NewWindow(1, 1, 30, 10, "anchor")}
	fm.Push(app)
	plugin.configure(app)
	if _, ok := fm.GetTopFrame().(*vtui.Window); !ok {
		t.Fatalf("top frame = %T, want anchored dialog", fm.GetTopFrame())
	}
	if len(fm.Screens[0].Frames) != 2 || fm.Screens[0].Frames[0] != app {
		t.Fatalf("anchored frames = %#v", fm.Screens[0].Frames)
	}
}

func TestConfigureReportsSaveErrors(t *testing.T) {
	t.Run("not initialized", func(t *testing.T) {
		plugin := NewPlugin(t.TempDir())
		fm, dialog, controls := openConfigDialog(t, plugin, configDialogApp{})
		controls.save.OnClick()
		expectSettingsError(t, fm, dialog)
	})

	t.Run("invalid settings", func(t *testing.T) {
		plugin := NewPlugin(t.TempDir())
		plugin.store = &settingsStore{current: DefaultSettings()}
		plugin.prefix = &configDialogPrefix{prefix: "MediaInfo"}
		fm, dialog, controls := openConfigDialog(t, plugin, configDialogApp{})
		controls.prefix.SetText("bad prefix")
		controls.save.OnClick()
		expectSettingsError(t, fm, dialog)
	})

	t.Run("prefix update", func(t *testing.T) {
		plugin := NewPlugin(t.TempDir())
		plugin.store = &settingsStore{current: DefaultSettings()}
		plugin.prefix = &configDialogPrefix{prefix: "MediaInfo", err: errors.New("prefix update failed")}
		fm, dialog, controls := openConfigDialog(t, plugin, configDialogApp{})
		controls.prefix.SetText("Media")
		controls.save.OnClick()
		expectSettingsError(t, fm, dialog)
	})

	t.Run("store update rolls back prefix", func(t *testing.T) {
		plugin := NewPlugin(t.TempDir())
		blocker := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		store := &settingsStore{
			path:    filepath.Join(blocker, "mediainfo.json"),
			current: DefaultSettings(),
		}
		prefix := &configDialogPrefix{prefix: "MediaInfo"}
		plugin.store = store
		plugin.prefix = prefix
		fm, dialog, controls := openConfigDialog(t, plugin, configDialogApp{})
		controls.prefix.SetText("Media")
		controls.save.OnClick()
		expectSettingsError(t, fm, dialog)
		if prefix.prefix != "MediaInfo" {
			t.Fatalf("prefix was not rolled back: %q", prefix.prefix)
		}
	})
}
