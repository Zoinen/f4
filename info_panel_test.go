package main

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// TestPanelsFrame_CtrlL_TogglesInfoPanel exercises far2l's Ctrl+L:
//   - first press installs an InfoPanel on the passive side, keeping
//     the file panel underneath alive and focus on the active side;
//   - second press removes it (toggle);
//   - Tab that lands on the alt slot keeps it open — the panel
//     visually becomes focused (as in far2l), but commands still
//     target the source file panel underneath.
func TestPanelsFrame_CtrlL_TogglesInfoPanel(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	send := func(vk uint16, mods vtinput.ControlKeyState) {
		pf.ProcessKey(&vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode:  vk,
			ControlKeyState: mods,
		})
	}

	// setupMockPanelsFrame sets activeIdx = 1 (right). Passive is left.
	if pf.altPanels[0] != nil || pf.altPanels[1] != nil {
		t.Fatal("precondition: no alt panels expected initially")
	}

	send(vtinput.VK_L, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] == nil {
		t.Fatal("Ctrl+L should install alt panel on passive (left) side")
	}
	if pf.altPanels[1] != nil {
		t.Error("active (right) side must not get an alt panel")
	}
	if _, ok := pf.altPanels[0].(*InfoPanel); !ok {
		t.Errorf("expected *InfoPanel, got %T", pf.altPanels[0])
	}
	if _, ok := pf.panels[0].(*FileSystemPanel); !ok {
		t.Error("file panel underneath must stay alive")
	}
	if pf.activeIdx != 1 {
		t.Errorf("Ctrl+L must not move active panel; got activeIdx=%d", pf.activeIdx)
	}

	// Source of the alt panel is the current active file panel.
	if src := pf.altPanels[0].Source(); src != pf.panels[1].(*FileSystemPanel) {
		t.Error("alt panel source should be the active file panel")
	}

	// Second press toggles it off.
	send(vtinput.VK_L, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] != nil {
		t.Error("second Ctrl+L should remove alt panel")
	}

	// Install again, then Tab to the alt side — Tab must keep the
	// alt panel visible AND flip its focused state so the frame
	// title recolors (matches far2l).
	send(vtinput.VK_L, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] == nil {
		t.Fatal("re-install: alt panel should be present again")
	}
	send(vtinput.VK_TAB, 0)
	if pf.activeIdx != 0 {
		t.Fatalf("Tab should switch active to left; got activeIdx=%d", pf.activeIdx)
	}
	if pf.altPanels[0] == nil {
		t.Error("Tab must NOT close the alt panel — it should stay visible")
	}
	// A render is required to propagate SetFocus into the alt panel;
	// call Show and then check the focus state was flipped.
	pf.Show(vtui.NewSilentScreenBuf())
	if !pf.altPanels[0].IsFocused() {
		t.Error("after Tab + render, alt panel should report focused=true")
	}

	// Ctrl+L while focus is ON the alt panel must close IT (matches
	// far2l), not open another one on the opposite side.
	send(vtinput.VK_L, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] != nil {
		t.Error("Ctrl+L on focused alt panel should close it")
	}
	if pf.altPanels[1] != nil {
		t.Error("Ctrl+L on focused alt must not spawn a second alt on the opposite side")
	}
}

// TestInfoPanel_ShowRenders verifies the panel renders without panic
// for a source panel with a couple of entries and clips to its width.
func TestInfoPanel_ShowRenders(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
	}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 19)
	// Should render without panic even on paths that fsSpace can't
	// resolve; fsSpace on tmp dirs works on unix, no crash otherwise.
	ip.Show(scr)

	if ip.Kind() != "info" {
		t.Errorf("Kind() = %q, want %q", ip.Kind(), "info")
	}
	if ip.Source() != fsp {
		t.Error("Source() should return the file panel we passed")
	}

	// SetFocus now tracks a visible focus marker (title recolour) —
	// used when Tab lands on the alt-panel slot.
	ip.SetFocus(true)
	if !ip.IsFocused() {
		t.Error("SetFocus(true) should be reflected by IsFocused")
	}
	ip.SetFocus(false)
	if ip.IsFocused() {
		t.Error("SetFocus(false) should clear the focus marker")
	}
}

// TestPanelsFrame_B_TogglesInfoPanelUnits verifies that `B` (plain,
// no modifiers) flips AppConfig.InfoPanelBytes while an info panel is
// visible, and falls through to fast-find otherwise.
func TestPanelsFrame_B_TogglesInfoPanelUnits(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	oldBytes := AppConfig.InfoPanelBytes
	defer func() { AppConfig.InfoPanelBytes = oldBytes }()
	AppConfig.InfoPanelBytes = false

	send := func(vk uint16) bool {
		return pf.ProcessKey(&vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode: vk,
		})
	}

	// No info panel yet: `B` must NOT touch the config (fast-find
	// path is expected to consume it).
	send(vtinput.VK_B)
	if AppConfig.InfoPanelBytes {
		t.Errorf("without info panel: B must not flip units, got InfoPanelBytes=true")
	}

	// Install info panel on passive side, then `B` should flip units.
	pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_L,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if pf.altPanels[0] == nil {
		t.Fatal("Ctrl+L should install info panel")
	}
	send(vtinput.VK_B)
	if !AppConfig.InfoPanelBytes {
		t.Errorf("with info panel: B should flip units to bytes")
	}
	send(vtinput.VK_B)
	if AppConfig.InfoPanelBytes {
		t.Errorf("second B should flip back to human")
	}
}

// TestFormatBytes_TogglesWithConfig verifies formatBytes routes to
// commas or human based on AppConfig.InfoPanelBytes.
func TestFormatBytes_TogglesWithConfig(t *testing.T) {
	old := AppConfig.InfoPanelBytes
	defer func() { AppConfig.InfoPanelBytes = old }()

	AppConfig.InfoPanelBytes = true
	if got := formatBytes(1024); got != formatBytesCommas(1024) {
		t.Errorf("bytes-mode: got %q, want commas form", got)
	}
	AppConfig.InfoPanelBytes = false
	if got := formatBytes(1024); got != formatBytesHuman(1024) {
		t.Errorf("human-mode: got %q, want human form", got)
	}
}

// TestShortUsername verifies the Windows machine/domain prefix is
// stripped so the info panel shows just the login name.
func TestShortUsername(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sogonov", "sogonov"},                 // unix
		{"INBOOK_X2_PLUS\\sogonov", "sogonov"}, // windows local
		{"MYDOMAIN\\alice.smith", "alice.smith"},
		{"forward/slash", "slash"}, // defensive: any known separator
		{"", ""},
		{"\\", ""},
	}
	for _, c := range cases {
		if got := shortUsername(c.in); got != c.want {
			t.Errorf("shortUsername(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFormatBytesCommas covers the raw-bytes-with-thousand-separator
// formatter used in the info panel. Matches far2l's InsertCommas.
func TestFormatBytesCommas(t *testing.T) {
	const nbsp = " "
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1" + nbsp + "000"},
		{1234567, "1" + nbsp + "234" + nbsp + "567"},
		{8191705088, "8" + nbsp + "191" + nbsp + "705" + nbsp + "088"},
	}
	for _, c := range cases {
		if got := formatBytesCommas(c.in); got != c.want {
			t.Errorf("formatBytesCommas(%d)=%q, want %q", c.in, got, c.want)
		}
	}
}
