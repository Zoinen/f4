package main

import (
	"strings"
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

// TestInfoPanel_CursorSkipsNonCopyable makes sure Up/Down land on
// copyable rows only — never on a section header or blank line.
func TestInfoPanel_CursorSkipsNonCopyable(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr) // populates rows and seeds cursor

	if ip.cursor < 0 || ip.cursor >= len(ip.rows) {
		t.Fatalf("expected cursor to be seeded on a row, got %d of %d", ip.cursor, len(ip.rows))
	}
	if !ip.rows[ip.cursor].copyable {
		t.Errorf("cursor landed on a non-copyable row (label=%q)", ip.rows[ip.cursor].label)
	}

	seen := 0
	for i := 0; i < 200; i++ {
		prev := ip.cursor
		ip.moveCursor(+1)
		if ip.cursor == prev {
			break
		}
		if !ip.rows[ip.cursor].copyable {
			t.Fatalf("Down landed on non-copyable row (label=%q)", ip.rows[ip.cursor].label)
		}
		seen++
	}
	if seen == 0 {
		t.Fatal("cursor didn't advance at all on repeated Down")
	}
	for i := 0; i < 200; i++ {
		prev := ip.cursor
		ip.moveCursor(-1)
		if ip.cursor == prev {
			break
		}
		if !ip.rows[ip.cursor].copyable {
			t.Fatalf("Up landed on non-copyable row (label=%q)", ip.rows[ip.cursor].label)
		}
	}
}

// TestInfoPanel_CopyCopiesValue verifies that 'C' while focused
// writes the current row's value (not the label) to vtui.SetClipboard.
func TestInfoPanel_CopyCopiesValue(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr)

	// Force cursor onto a known row (first copyable) and copy.
	ip.setCursorToFirstCopyable()
	if ip.cursor < 0 {
		t.Fatal("no copyable row found on default info panel — layout regression?")
	}
	wantValue := ip.rows[ip.cursor].value
	if wantValue == "" {
		t.Fatal("first copyable row has empty value; test premise broken")
	}
	ip.copyCurrent()
	if got := vtui.GetClipboard(); got != wantValue {
		t.Errorf("SetClipboard got %q, want %q", got, wantValue)
	}
}

// TestInfoPanel_ProcessKey_UnfocusedIgnoresC verifies the C copy
// hotkey only fires when the panel is focused — otherwise it must
// fall through so the file panel's fast-find still sees it.
func TestInfoPanel_ProcessKey_UnfocusedIgnoresC(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.Show(scr)
	// Not focused.
	handled := ip.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_C,
	})
	if handled {
		t.Error("unfocused info panel must not consume C")
	}
}

// TestInfoPanel_ShiftUpDownSelectsAndCCopiesLabelValue checks the
// multi-row selection UX: Shift+Down toggles the current row's
// selection and moves the cursor down; a subsequent C copies every
// selected row as "label: value" per line (in on-screen order),
// with a two-line minimum so the copy joiner is actually exercised.
func TestInfoPanel_ShiftUpDownSelectsAndCCopiesLabelValue(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr)
	ip.setCursorToFirstCopyable()

	// First row: Shift+Down should toggle current selection then move.
	firstLabel := ip.rows[ip.cursor].label
	firstValue := ip.rows[ip.cursor].value
	ip.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_DOWN,
		ControlKeyState: vtinput.ShiftPressed,
	})
	// Second row (post-move): Shift+Down again, adds it too.
	secondLabel := ip.rows[ip.cursor].label
	secondValue := ip.rows[ip.cursor].value
	ip.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_DOWN,
		ControlKeyState: vtinput.ShiftPressed,
	})
	firstSection := ip.rows[0].section
	// After the second Shift+Down the cursor sits on the second
	// selectable row; recover its section from the row list.
	secondSection := ""
	for _, r := range ip.rows {
		if r.label == secondLabel && r.copyable {
			secondSection = r.section
			break
		}
	}
	if !ip.selection[rowKey(firstSection, firstLabel)] ||
		!ip.selection[rowKey(secondSection, secondLabel)] {
		t.Fatalf("expected both %q and %q to be selected; got selection=%v",
			firstLabel, secondLabel, ip.selection)
	}

	// C copies both rows as label: value per line, in on-screen order.
	ip.copyCurrent()
	want := firstLabel + ": " + firstValue + "\n" + secondLabel + ": " + secondValue
	if got := vtui.GetClipboard(); got != want {
		t.Errorf("clipboard = %q, want %q", got, want)
	}

	// Selection persists across a rebuild — the highlight must
	// survive the next Show, which walks the row list from scratch.
	ip.Show(scr)
	for _, r := range ip.rows {
		if r.label == firstLabel || r.label == secondLabel {
			if !r.selected {
				t.Errorf("row %q lost its selected flag after rebuild", r.label)
			}
		}
	}
}

// TestInfoPanel_WrapRowContinuationInheritsSelection checks that
// when a value overflows the panel width and wrapRow spills it onto
// hanging continuation lines, selecting the row highlights ALL of
// its screen lines — not just the first. The line break is a
// display artifact, not a selection boundary. Realised via
// section+label tagging of continuation rows so ip.selection lights
// every line owned by the parent label.
func TestInfoPanel_WrapRowContinuationInheritsSelection(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	// Narrow panel so any real Flags-style row spills onto at least
	// two hanging lines.
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr)

	// Directly select the Flags label if a wrapped row exists.
	// If it doesn't (fs has no flags string), simulate one by
	// invoking wrapRow via a synthesised call. Simpler: iterate
	// existing rows for two consecutive rows sharing (section,
	// label) — that's a wrap. If none exist skip the test.
	var parentIdx int = -1
	for i := 0; i+1 < len(ip.rows); i++ {
		r, next := ip.rows[i], ip.rows[i+1]
		if r.copyable && !next.copyable && r.label != "" &&
			next.label == r.label && next.section == r.section {
			parentIdx = i
			break
		}
	}
	if parentIdx < 0 {
		t.Skip("no wrapped row present in this environment — nothing to verify")
	}
	parent := ip.rows[parentIdx]
	ip.selection[rowKey(parent.section, parent.label)] = true
	ip.Show(scr) // triggers the restore loop

	// Every row sharing (section, label) with the parent must now
	// carry selected=true, contiguous continuation and all.
	for _, r := range ip.rows {
		if r.section == parent.section && r.label == parent.label {
			if !r.selected {
				t.Errorf("row (section=%q label=%q, text=%q) should be selected",
					r.section, r.label, r.text)
			}
		}
	}
}

// TestInfoPanel_InsTogglesSelectionAndMoves verifies Ins behaves
// like the file-panel Ins: toggle current, advance one row.
func TestInfoPanel_InsTogglesSelectionAndMoves(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 39, 24)
	ip.SetFocus(true)
	ip.Show(scr)
	ip.setCursorToFirstCopyable()

	startRow := ip.rows[ip.cursor]
	startCursor := ip.cursor
	ip.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_INSERT,
	})
	if !ip.selection[rowKey(startRow.section, startRow.label)] {
		t.Errorf("Ins should have selected %q", startRow.label)
	}
	if ip.cursor == startCursor {
		t.Error("Ins should have advanced the cursor by one copyable row")
	}
}

// TestInfoPanel_CPUSectionRespectsOption checks that the CPU/GPU
// section is opt-in — hidden when AppConfig.InfoPanelCPUGPU is off,
// present when it's on. Guards the maintainer's off-by-default ask.
func TestInfoPanel_CPUSectionRespectsOption(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 40)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 60, 40, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	fsp.Refresh()

	old := AppConfig.InfoPanelCPUGPU
	defer func() { AppConfig.InfoPanelCPUGPU = old }()

	ip := NewInfoPanel(fsp)
	ip.SetPosition(0, 0, 59, 39)

	hasLabelPrefix := func(prefix string) bool {
		for _, r := range ip.rows {
			if strings.HasPrefix(r.label, prefix) {
				return true
			}
		}
		return false
	}

	AppConfig.InfoPanelCPUGPU = false
	ip.Show(scr)
	if hasLabelPrefix("Model") || hasLabelPrefix("Cores") {
		t.Error("CPU rows must not render when InfoPanelCPUGPU is off")
	}

	AppConfig.InfoPanelCPUGPU = true
	ip.Show(scr)
	// Cores is always populated (runtime.NumCPU seeds LogicalCores
	// on every OS). Label depends on HT — plain "Cores / threads"
	// when different, or on any i18n change the prefix "Cores"
	// still matches.
	if !hasLabelPrefix("Cores") {
		t.Error("expected CPU 'Cores' row once InfoPanelCPUGPU is enabled")
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
