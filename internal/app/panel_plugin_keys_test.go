package app

import (
	"testing"

	"github.com/unxed/f4/internal/appcmd"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// panelKeysTestController is a panel plugin that declares F8 through the
// shared vfs.PanelKeyProvider primitive and counts what reaches ProcessKey.
type panelKeysTestController struct {
	panelPluginTestController
	ran int
}

func (p *panelKeysTestController) PanelKeys() []vfs.PanelKey {
	return []vfs.PanelKey{{VK: vtinput.VK_F8, Label: "Kill", Run: func() { p.ran++ }}}
}

func (p *panelKeysTestController) ProcessKey(e *vtinput.InputEvent) bool {
	p.keys++
	return e != nil && e.VirtualKeyCode == vtinput.VK_F4
}

// TestPanelPluginOwnsItsKeysAndKeyBar covers the f4#312 primitive end to
// end through the real key router: a declared key runs ahead of the file
// panel's own binding for it, a File.* binding stands down and hands its key
// to the plugin, a window-level binding keeps its caption, and the keybar
// shows the declared caption instead of the file panel's.
func TestPanelPluginOwnsItsKeysAndKeyBar(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ResizeConsole(80, 25)
	defer pf.Close()
	vtui.FrameManager.Push(pf)

	shell := map[string]string{"F1": "App.Help", "F4": "File.Edit", "F8": "File.Delete"}
	previousHotkeys, previousMacro := keymap.GlobalHotkeysMgr, macro.MacroMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{"Shell": shell},
		Bindings: map[string]map[string]string{"Shell": shell},
	}
	manager := &macro.MacroManager{}
	macro.MacroMgr = manager
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = previousHotkeys
		macro.MacroMgr = previousMacro
	})

	controller := &panelKeysTestController{}
	registration, err := (&coreAPI{}).RegisterPanelProvider(vfs.PanelProvider{
		ID:    "test.panel.keys",
		Title: "Keys panel",
		Open:  func(vfs.PanelContext) (vfs.PanelController, error) { return controller, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()

	panel.OpenRegisteredPanelProvider(pf, "test.panel.keys")
	instance, ok := pf.AltPanels[pf.ActiveIdx].(*panel.PluginPanelInstance)
	if !ok {
		t.Fatalf("active slot contains %T, want *panel.PluginPanelInstance", pf.AltPanels[pf.ActiveIdx])
	}
	instance.SetFocus(true)
	if !pf.PluginPanelFocused() {
		t.Fatal("PluginPanelFocused is false with a focused plugin panel in the active slot")
	}

	f8 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F8}
	if !macroFilter(manager, f8) || controller.ran != 1 {
		t.Fatalf("declared F8: ran=%d, want the plugin's key to run once ahead of File.Delete", controller.ran)
	}

	f4 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4}
	if macroFilter(manager, f4) {
		t.Fatal("File.Edit on F4 was dispatched although a panel plugin owns the keyboard")
	}
	if !pf.ProcessKey(f4) || controller.keys == 0 {
		t.Fatal("F4 did not reach the plugin's ProcessKey after File.Edit stood down")
	}

	if !pf.PluginPanelStandsDown("File.Delete") || pf.PluginPanelStandsDown("App.Help") {
		t.Fatal("stand-down must cover File.* and nothing window-level")
	}

	labels := pf.GetKeyLabels()
	if labels.Normal[7] != "Kill" {
		t.Fatalf("F8 caption = %q, want the plugin's declared \"Kill\"", labels.Normal[7])
	}
	if labels.Normal[3] != "" {
		t.Fatalf("F4 caption = %q, want blank while File.Edit stands down", labels.Normal[3])
	}
	if labels.Normal[0] == "" {
		t.Fatal("F1 (App.Help) lost its caption under a panel plugin")
	}

	instance.Close()
	if pf.PluginPanelFocused() || pf.PluginPanelStandsDown("File.Delete") {
		t.Fatal("the stand-down outlived the plugin panel")
	}
}

// TestPanelPluginEscReturnsToFilePanel is Zeroes1's item 3 on f4#312: with
// Esc bound to Panel.Toggle (Esc:EscToggle, the shipped default), Esc on a
// panel plugin used to hide every panel instead of reaching the plugin, so
// there was no way back to the file panel. Esc now closes the plugin panel;
// a non-empty command line still takes it for clearing, and without a
// plugin panel Esc toggles the panels as before. Item 1 rides along: the
// plugin menu row is the provider's bare title, not "Open <title>".
func TestPanelPluginEscReturnsToFilePanel(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ResizeConsole(80, 25)
	defer pf.Close()
	vtui.FrameManager.Push(pf)

	previousEscToggle := config.App.EscTogglePanels
	config.App.EscTogglePanels = true
	shell := map[string]string{"Esc": "Panel.Toggle:EscToggle"}
	previousHotkeys, previousMacro := keymap.GlobalHotkeysMgr, macro.MacroMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{"Shell": shell},
		Bindings: map[string]map[string]string{"Shell": shell},
	}
	macro.MacroMgr = &macro.MacroManager{}
	t.Cleanup(func() {
		config.App.EscTogglePanels = previousEscToggle
		keymap.GlobalHotkeysMgr = previousHotkeys
		macro.MacroMgr = previousMacro
	})

	controller := &panelKeysTestController{}
	registration, err := (&coreAPI{}).RegisterPanelProvider(vfs.PanelProvider{
		ID:    "test.panel.esc",
		Title: "Esc panel",
		Open:  func(vfs.PanelContext) (vfs.PanelController, error) { return controller, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()

	var label string
	for _, command := range plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, nil) {
		if command.ID == "panel.test.panel.esc" {
			label = command.Label
		}
	}
	if label != "Esc panel" {
		t.Fatalf("plugin menu row = %q, want the bare title \"Esc panel\"", label)
	}

	open := func() *panel.PluginPanelInstance {
		t.Helper()
		panel.OpenRegisteredPanelProvider(pf, "test.panel.esc")
		instance, ok := pf.AltPanels[pf.ActiveIdx].(*panel.PluginPanelInstance)
		if !ok {
			t.Fatalf("active slot contains %T, want *panel.PluginPanelInstance", pf.AltPanels[pf.ActiveIdx])
		}
		instance.SetFocus(true)
		return instance
	}
	esc := func() bool {
		return pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	}

	open()
	pf.CmdLine.InsertString("typed")
	esc()
	if !pf.CmdLine.IsEmpty() || !pf.PluginPanelFocused() || !pf.ShowPanels {
		t.Fatalf("Esc over a non-empty command line: cmdline empty=%v, plugin open=%v, panels=%v; want the line cleared and the plugin kept",
			pf.CmdLine.IsEmpty(), pf.PluginPanelFocused(), pf.ShowPanels)
	}

	keysBefore := controller.keys
	if !esc() {
		t.Fatal("Esc on a panel plugin was not handled")
	}
	if !pf.ShowPanels {
		t.Fatal("Esc on a panel plugin hid the panels (Panel.Toggle) instead of closing the plugin")
	}
	if _, still := pf.AltPanels[pf.ActiveIdx].(*panel.PluginPanelInstance); still || !controller.closed {
		t.Fatal("Esc did not close the panel plugin")
	}
	if controller.keys <= keysBefore {
		t.Fatal("the plugin controller never saw Esc before the host closed it")
	}

	esc()
	if pf.ShowPanels {
		t.Fatal("with the plugin closed, Esc must toggle the panels again (Esc:EscToggle)")
	}
}

func TestIsFilePanelScopedAction(t *testing.T) {
	for name, want := range map[string]bool{
		"File.Delete":             true,
		"file.edit":               true,
		"File.View:SomeCond":      true,
		"Panel.SelectGroup":       true,
		"Panel.InvertSelection":   true,
		"Panel.Toggle":            false,
		"Panel.InfoPanel":         false,
		"App.Help":                false,
		"Plugin.Command.anything": false,
	} {
		if got := panel.IsFilePanelScopedAction(name); got != want {
			t.Errorf("IsFilePanelScopedAction(%q) = %v, want %v", name, got, want)
		}
	}
}

// A panel plugin carried to the other side by Ctrl+U has to close from there
// by every close key: its close hook used to look at the side it was opened
// on, so ProcList, Services, Network and Git status stayed (f4#1715).
func TestPluginPanelClosesAfterCtrlUSwap(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ResizeConsole(80, 25)
	defer pf.Close()

	closeKeys := map[string]*vtinput.InputEvent{
		"Esc":       {Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE},
		"F10":       {Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F10},
		"Ctrl+PgUp": {Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR, ControlKeyState: vtinput.LeftCtrlPressed},
	}
	for name, key := range closeKeys {
		for swaps := 1; swaps <= 2; swaps++ {
			controller := &panelKeysTestController{}
			id := "test.panel.swapclose"
			registration, err := (&coreAPI{}).RegisterPanelProvider(vfs.PanelProvider{
				ID:    id,
				Title: "Swap close panel",
				Open:  func(vfs.PanelContext) (vfs.PanelController, error) { return controller, nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			pf.ActiveIdx = 1
			panel.OpenRegisteredPanelProvider(pf, id)
			instance, ok := pf.AltPanels[1].(*panel.PluginPanelInstance)
			if !ok {
				registration.Unregister()
				t.Fatalf("%s: panel plugin did not open on the right", name)
			}
			for i := 0; i < swaps; i++ {
				pf.HandleCommand(appcmd.CmSwapPanels, nil)
			}
			if !instance.ProcessKey(key) {
				t.Errorf("%s after %d swap(s): key not handled", name, swaps)
			}
			if pf.AltPanels[0] != nil || pf.AltPanels[1] != nil {
				t.Errorf("%s after %d swap(s): the panel plugin is still open (left=%v right=%v)", name, swaps, pf.AltPanels[0], pf.AltPanels[1])
			}
			registration.Unregister()
		}
	}
}
