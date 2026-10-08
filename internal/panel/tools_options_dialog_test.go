package panel

import (
	"os"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func findToolsOptionsDialog(t *testing.T) (*vtui.Window, *toolsOptionsList, *vtui.Button) {
	t.Helper()
	pumpTasks(t)
	var win *vtui.Window
	for _, screen := range vtui.FrameManager.Screens {
		for _, frame := range screen.Frames {
			if w, ok := frame.(*vtui.Window); ok {
				win = w
			}
		}
	}
	if win == nil {
		t.Fatalf("the tools window is not open; top frame %T", vtui.FrameManager.GetTopFrame())
	}
	var list *toolsOptionsList
	var button *vtui.Button
	for _, item := range win.GetChildren() {
		switch it := item.(type) {
		case *toolsOptionsList:
			list = it
		case *vtui.Button:
			button = it
		}
	}
	if list == nil || button == nil {
		t.Fatalf("the tools window lacks its list or its Settings button: %T", win.GetChildren())
	}
	return win, list, button
}

// The tools window lists every F11 entry with a check box that works at once,
// and a Settings button that is live only for a tool that has settings
// (f4#918).
func TestToolsOptionsWindowTogglesToolsAndKnowsWhichHaveSettings(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)

	saved := plughost.PluginMenuItems
	t.Cleanup(func() { plughost.PluginMenuItems = saved })
	plughost.PluginMenuItems = []plughost.PluginMenuItem{{ActionName: "test.legacy", Label: "Legacy tool"}}

	var configured int
	regs := []vfs.Registration{}
	for _, command := range []vfs.PluginCommand{
		{ID: "test.alpha.open", Location: vfs.PluginCommandPanel, Label: "Alpha tool", Run: func(vfs.App) {}},
		{ID: "test.alpha.configure", Location: vfs.PluginCommandConfig, Label: "Alpha", Run: func(vfs.App) { configured++ }},
		{ID: "test.beta.open", Location: vfs.PluginCommandPanel, Label: "Beta tool", Run: func(vfs.App) {}},
	} {
		reg, err := plughost.RegisterPluginCommand(command)
		if err != nil {
			t.Fatal(err)
		}
		regs = append(regs, reg)
	}
	t.Cleanup(func() {
		for _, reg := range regs {
			reg.Unregister()
		}
	})

	path := PluginMenuVisibilityFilePath()
	before, readErr := os.ReadFile(path)
	t.Cleanup(func() {
		if readErr != nil {
			_ = os.Remove(path)
		} else {
			_ = os.WriteFile(path, before, 0o600) // #nosec G703 -- the test's own profile file, put back.
		}
	})
	if err := SavePluginMenuHidden(nil); err != nil {
		t.Fatal(err)
	}

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)
	pf.ShowToolsOptions()
	_, list, button := findToolsOptionsDialog(t)

	var texts []string
	alpha := -1
	for i, item := range list.Items {
		texts = append(texts, item)
		if strings.Contains(item, "Alpha tool") {
			alpha = i
		}
	}
	if len(texts) != 3 || alpha < 0 || !strings.HasPrefix(texts[alpha], "[x] ") {
		t.Fatalf("rows = %q, want all three tools checked", texts)
	}

	// The Settings button follows the selection.
	list.SetSelectPos(alpha)
	list.onSelect(alpha)
	if button.IsDisabled() {
		t.Error("Alpha has settings, but the Settings button is dimmed")
	}
	for i, item := range list.Items {
		if i != alpha {
			list.SetSelectPos(i)
			list.onSelect(i)
			if !button.IsDisabled() {
				t.Errorf("%q has no settings, but the Settings button is live", item)
			}
		}
	}
	list.SetSelectPos(alpha)
	list.onSelect(alpha)
	button.OnClick()
	if configured != 1 {
		t.Errorf("the Settings button ran the configuration %d times, want 1", configured)
	}

	// Space turns the tool off and the choice is stored at once.
	list.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE, Char: ' '})
	if !strings.HasPrefix(list.Items[alpha], "[ ] ") {
		t.Errorf("Space left the row as %q", list.Items[alpha])
	}
	hidden, err := LoadPluginMenuHidden()
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 1 || !strings.Contains(hidden[0], "alpha") {
		t.Errorf("stored hidden names = %v, want the alpha tool", hidden)
	}

	// A click on the box turns it back on; a click on the name only selects.
	row := alpha
	click := func(dx int) {
		// #nosec G115 -- cells of an 80x25 test screen.
		list.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
			MouseX: int16(list.X1 + dx), MouseY: int16(list.Y1 + row - list.TopPos),
		})
	}
	click(1)
	if !strings.HasPrefix(list.Items[alpha], "[x] ") {
		t.Errorf("a click on the box left the row as %q", list.Items[alpha])
	}
	click(10)
	if !strings.HasPrefix(list.Items[alpha], "[x] ") {
		t.Errorf("a click on the name changed the row to %q", list.Items[alpha])
	}
}
