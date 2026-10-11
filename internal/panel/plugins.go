package panel

import (
	"fmt"
	"strings"
	"sync"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func OpenRegisteredPanelProvider(app vfs.App, providerID string) {
	pf, ok := app.(*PanelsFrame)
	if !ok || pf == nil || !pf.ShowPanels || pf.Closed {
		return
	}
	provider, ok := plughost.LookupPanelProvider(providerID)
	if !ok {
		return
	}

	slot := pf.ActiveIdx
	ctx := pf.panelPluginContext(slot)
	controller, err := provider.Open(ctx)
	if err != nil {
		vtui.DebugLog("PANEL [%s]: open failed: %v", provider.ID, err)
		toast.Show(fmt.Sprintf("Panel %s: %v", provider.Title, err), 3e9)
		return
	}
	if controller == nil {
		vtui.DebugLog("PANEL [%s]: open returned nil controller", provider.ID)
		return
	}

	if old := pf.AltPanels[slot]; old != nil {
		if closer, ok := old.(interface{ Close() }); ok {
			closer.Close()
		} else {
			pf.AltPanels[slot] = nil
		}
	}
	instance := &PluginPanelInstance{
		providerID: provider.ID,
		title:      provider.Title,
		host:       pf,
		slot:       slot,
		controller: controller,
	}
	// The side the panel is on right now is instance.slot, not the side it was
	// opened on: Ctrl+U moves it (swapPluginPanels), and closing through the
	// stale opening side left the panel open and unclosable (f4#1715).
	instance.closeFn = func() {
		if cur := instance.slot; cur >= 0 && cur < len(pf.AltPanels) && pf.AltPanels[cur] == instance {
			pf.AltPanels[cur] = nil
			// PanelsFrame.Close holds ptyMutex while closing overlays. Do not
			// re-enter ResizeConsole from that path.
			if !pf.Closed {
				pf.ResizeConsole(pf.LastW, pf.LastH)
				vtui.FrameManager.HardRefresh()
			}
		}
	}
	instance.SetPositionForPanel()
	instance.SetContext(ctx)
	pf.AltPanels[slot] = instance
	if slot == 0 {
		pf.ShowLeftPanel = true
	} else {
		pf.ShowRightPanel = true
	}
	pf.ResizeConsole(pf.LastW, pf.LastH)
	vtui.FrameManager.HardRefresh()
}

// pluginPanelInstance adapts the public vfs panel contract to f4's existing
// AltPanel slot. The FileSystemPanel remains underneath as the logical panel,
// so ordinary file actions and VFS providers keep their existing assumptions.
type PluginPanelInstance struct {
	providerID string
	title      string
	host       *PanelsFrame
	slot       int
	controller vfs.PanelController
	closeFn    func()
	closeOnce  sync.Once
}

func (p *PluginPanelInstance) Source() *FileSystemPanel {
	if p == nil || p.host == nil || p.slot < 0 || p.slot >= len(p.host.Panels) {
		return nil
	}
	fsp, _ := p.host.Panels[p.slot].(*FileSystemPanel)
	return fsp
}

func (p *PluginPanelInstance) Kind() string { return "plugin:" + p.providerID }

func (p *PluginPanelInstance) Show(scr *vtui.ScreenBuf) {
	if p == nil || p.controller == nil {
		return
	}
	if p.host != nil {
		p.SetContext(p.host.panelPluginContext(p.slot))
	}
	p.controller.Show(scr)
}

// isPluginPanelCloseKey reports a plain F10 or Ctrl+PgUp key press. Under a
// panel plugin F10 closes the panel and returns to the file panel, the same
// way Esc does (f4#312, reviewer request), and Ctrl+PgUp does too: it is
// "go up" on the file panel, and going up from a panel plugin's root leaves
// the panel (owner decision on f4#312). This deliberately takes F10 from the
// window-level App.Quit (F10 in the Shell area): a panel plugin is a
// transient view over the file panel, and the project convention for such
// transient views is that Esc/F10 close them (the viewer and editor close on
// F10 as well), whereas an unintended quit of the whole file manager loses
// the user's session. Quit stays reachable through Esc then F10 and the menu;
// a controller that declares or handles either key itself still wins.
func isPluginPanelCloseKey(e *vtinput.InputEvent) bool {
	if e == nil || e.Type != vtinput.KeyEventType || !e.KeyDown {
		return false
	}
	const ctrl = vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed
	const others = vtinput.LeftAltPressed | vtinput.RightAltPressed | vtinput.ShiftPressed
	switch e.VirtualKeyCode {
	case vtinput.VK_F10:
		return e.ControlKeyState&(ctrl|others) == 0
	case vtinput.VK_PRIOR:
		return e.ControlKeyState&ctrl != 0 && e.ControlKeyState&others == 0
	}
	return false
}

func (p *PluginPanelInstance) ProcessKey(e *vtinput.InputEvent) bool {
	if p == nil || p.controller == nil {
		return false
	}
	if p.host != nil {
		p.SetContext(p.host.panelPluginContext(p.slot))
	}
	// Declared keys first: a real key event already went through
	// PanelsFrame.InterceptPluginKey (which runs the same dispatch ahead of
	// every hotkey), but a key injected by a keybar click or InjectEvents
	// bypasses that filter and arrives here directly.
	if vfs.DispatchPanelKey(p.PanelKeys(), e) {
		return true
	}
	// Ctrl+PgUp is asked before the controller: a list-like plugin panel
	// (ProcList) reads it as "page up" and consumes it, so it never reached the
	// close check below (f4#312). A key the controller declares still won above.
	if e != nil && e.VirtualKeyCode == vtinput.VK_PRIOR && isPluginPanelCloseKey(e) {
		p.Close()
		return true
	}
	if p.controller.ProcessKey(e) {
		return true
	}
	// F10 and Ctrl+PgUp leave the panel too (see isPluginPanelCloseKey); they
	// are checked here as well so a keybar click, which is injected straight into
	// ProcessKey, closes the panel just as the key does.
	if isPluginPanelCloseKey(e) {
		p.Close()
		return true
	}
	// Escape is the common close gesture for a panel plugin that does not
	// claim it. A plugin that wants to own Escape simply returns true.
	// While the command line holds text, Esc clears it instead, as it does
	// over a file panel.
	if e != nil && e.KeyDown && e.VirtualKeyCode == vtinput.VK_ESCAPE {
		if p.host != nil && p.host.escClearsCommandLine() {
			return false
		}
		p.Close()
		return true
	}
	return false
}

// PanelKeys returns the controller's declared keys (vfs.PanelKeyProvider),
// or nil for a controller that declares none.
func (p *PluginPanelInstance) PanelKeys() []vfs.PanelKey {
	if p == nil || p.controller == nil {
		return nil
	}
	if kp, ok := p.controller.(vfs.PanelKeyProvider); ok {
		return kp.PanelKeys()
	}
	return nil
}

func (p *PluginPanelInstance) ProcessMouse(e *vtinput.InputEvent) bool {
	if p == nil || p.controller == nil {
		return false
	}
	if p.host != nil {
		p.SetContext(p.host.panelPluginContext(p.slot))
	}
	return p.controller.ProcessMouse(e)
}

func (p *PluginPanelInstance) SetFocus(focused bool) {
	if p != nil && p.controller != nil {
		p.controller.SetFocus(focused)
	}
}

func (p *PluginPanelInstance) IsFocused() bool {
	return p != nil && p.controller != nil && p.controller.IsFocused()
}

func (p *PluginPanelInstance) SetPosition(x1, y1, x2, y2 int) {
	if p != nil && p.controller != nil {
		p.controller.SetPosition(x1, y1, x2, y2)
	}
}

func (p *PluginPanelInstance) SetPositionForPanel() {
	if p == nil || p.host == nil || p.controller == nil {
		return
	}
	x1, y1, x2, y2 := p.host.Panels[p.slot].GetPosition()
	p.controller.SetPosition(x1, y1, x2, y2)
}

func (p *PluginPanelInstance) GetPosition() (int, int, int, int) {
	if p == nil || p.controller == nil {
		return 0, 0, 0, 0
	}
	return p.controller.GetPosition()
}

func (p *PluginPanelInstance) GetSelectedName() string {
	if p == nil || p.controller == nil {
		return ""
	}
	return p.controller.GetSelectedName()
}

func (p *PluginPanelInstance) SetContext(ctx vfs.PanelContext) {
	if p != nil && p.controller != nil {
		p.controller.SetContext(ctx)
	}
}

func (p *PluginPanelInstance) Close() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		if p.controller != nil {
			_ = p.controller.Close()
		}
		if p.closeFn != nil {
			p.closeFn()
		}
	})
}

func (pf *PanelsFrame) panelPluginContext(slot int) vfs.PanelContext {
	ctx := vfs.PanelContext{Side: slot, ActiveSide: pf.ActiveIdx}
	if slot == pf.ActiveIdx {
		ctx.Current = pf.panelPluginState(slot, true)
		ctx.Other = pf.panelPluginState(1-slot, false)
	} else {
		ctx.Current = pf.panelPluginState(slot, false)
		ctx.Other = pf.panelPluginState(1-slot, true)
	}
	if alt, ok := pf.AltPanels[slot].(*PluginPanelInstance); ok && alt.controller != nil {
		ctx.Bounds[0], ctx.Bounds[1], ctx.Bounds[2], ctx.Bounds[3] = alt.controller.GetPosition()
	} else if panel := pf.Panels[slot]; panel != nil {
		ctx.Bounds[0], ctx.Bounds[1], ctx.Bounds[2], ctx.Bounds[3] = panel.GetPosition()
	}
	return ctx
}

func (pf *PanelsFrame) panelPluginState(slot int, active bool) vfs.PanelState {
	state := vfs.PanelState{Side: slot, Active: active}
	if slot < 0 || slot >= len(pf.Panels) {
		return state
	}
	fsp, ok := pf.Panels[slot].(*FileSystemPanel)
	if !ok || fsp == nil || fsp.Vfs == nil {
		return state
	}
	state.Path = fsp.Vfs.GetPath()
	state.SelectedName = fsp.GetSelectedName()
	state.SelectedNames = append([]string(nil), fsp.GetSelectedNames()...)
	return state
}

// focusedPluginPanel returns the panel plugin that owns the keyboard right
// now: the one in the active slot, focused, with the panels shown and the
// command line not holding an explicit search-first focus. Every
// vfs.PanelKeyProvider guarantee (vfs/contributions.go) is keyed off this
// single predicate so dispatch, stand-down and the keybar cannot disagree
// about who owns a key.
func (pf *PanelsFrame) focusedPluginPanel() *PluginPanelInstance {
	if pf == nil || !pf.ShowPanels || pf.ActiveIdx < 0 || pf.ActiveIdx >= len(pf.AltPanels) {
		return nil
	}
	if pf.SearchFirstMode() && pf.CommandLineFocused {
		return nil
	}
	inst, ok := pf.AltPanels[pf.ActiveIdx].(*PluginPanelInstance)
	if !ok || inst == nil || !inst.IsFocused() {
		return nil
	}
	return inst
}

// PluginPanelFocused reports whether a panel plugin owns the keyboard (see
// focusedPluginPanel).
func (pf *PanelsFrame) PluginPanelFocused() bool { return pf.focusedPluginPanel() != nil }

// filePanelSelectionActions are the Panel.* actions that, like every File.*
// action, act on the file panel's cursor or selection rather than on the
// window as a whole.
var filePanelSelectionActions = map[string]bool{
	"panel.selectgroup":              true,
	"panel.deselectgroup":            true,
	"panel.selectcurrentextension":   true,
	"panel.deselectcurrentextension": true,
	"panel.invertselection":          true,
	"panel.restoreselection":         true,
}

// filePanelNavigationActions are the Panel.* actions that navigate or read
// the file panel under a panel plugin: directory changes (Ctrl+PgUp/Ctrl+PgDn,
// Ctrl+Del, Alt+Left/Right), name scrolling, path/name clipboard and insert
// gestures, and the ones that open or transform the entry under its cursor.
// Same reason as filePanelSelectionActions: the file panel is hidden, so these
// would silently change a directory or copy a name the user cannot see
// (f4#312, Ctrl+PgUp/Ctrl+PgDn kept switching directories under ProcList).
var filePanelNavigationActions = map[string]bool{
	"panel.goparent":              true,
	"panel.goroot":                true,
	"panel.enterdirectory":        true,
	"panel.historyback":           true,
	"panel.historyforward":        true,
	"panel.scrollnamesleft":       true,
	"panel.scrollnamesright":      true,
	"panel.scrollnameshome":       true,
	"panel.scrollnamesend":        true,
	"panel.copypath":              true,
	"panel.insertpath":            true,
	"panel.copyname":              true,
	"panel.copyselectednames":     true,
	"panel.copyselectedpaths":     true,
	"panel.copyselectedrealpaths": true,
	"panel.insertfilename":        true,
	"panel.systemexplorer":        true,
	"panel.selectnavigation":      true,
	"panel.fileassociations":      true,
	"panel.base64encodefile":      true,
	"panel.base64decodefile":      true,
	"panel.comparefilesbycontent": true,
}

// IsFilePanelScopedAction reports whether an action operates on the file
// panel's cursor, selection or directory. Such bindings stand down while a panel plugin
// owns the keyboard: the file panel is hidden under the plugin, so F8 must
// not delete, F4 must not edit and Gray+ must not select a file the user
// cannot see (f4#312). The rule is the File.* namespace plus
// filePanelSelectionActions and filePanelNavigationActions, deliberately by name and not per key, so a
// user's own rebinding (File.Delete on another key) stands down too and a
// window-level action on an F-key (Help, menus, quit) keeps working.
func IsFilePanelScopedAction(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if i := strings.IndexByte(name, ':'); i >= 0 {
		name = name[:i]
	}
	return strings.HasPrefix(name, "file.") || filePanelSelectionActions[name] || filePanelNavigationActions[name]
}

// PluginPanelStandsDown reports whether a configured hotkey resolving to
// actionName must not run because a panel plugin owns the keyboard. The key
// router (internal/app/macro_dispatch.go) then hands the key on to the frame
// instead, so it reaches the plugin's own ProcessKey.
func (pf *PanelsFrame) PluginPanelStandsDown(actionName string) bool {
	return pf.focusedPluginPanel() != nil && IsFilePanelScopedAction(actionName)
}

// pluginPanelKeyLabels is the keybar while a panel plugin owns the keyboard:
// shell's own resolved captions with every file-panel-scoped one blanked
// (IsFilePanelScopedAction; the archive plugins' Shift+F1..F3 fallback
// captions go too, because global plugin hotkeys stand down as well), then
// the plugin's declared captions on top. Only F1..F12 with no modifier or
// with exactly one of Shift/Ctrl/Alt has a keybar row to show.
func (pf *PanelsFrame) pluginPanelKeyLabels(inst *PluginPanelInstance, fallbacks *vtui.KeySet) *vtui.KeySet {
	if fallbacks != nil {
		trimmed := *fallbacks
		for i := 0; i < 3; i++ {
			trimmed.Shift[i] = ""
		}
		fallbacks = &trimmed
	}
	set := keymap.KeyBarLabelsForAreaExcept("Shell", fallbacks, IsFilePanelScopedAction)
	// F10 closes the panel (isPluginPanelCloseKey) instead of quitting; a
	// plugin that declares its own F10 overrides this caption below.
	set.Normal[vtinput.VK_F10-vtinput.VK_F1], set.NormalDisabled[vtinput.VK_F10-vtinput.VK_F1] = i18n.Msg("PanelPlugin.KeyBar.Close"), false
	for _, k := range inst.PanelKeys() {
		if k.VK < vtinput.VK_F1 || k.VK > vtinput.VK_F12 || k.Label == "" {
			continue
		}
		i := int(k.VK - vtinput.VK_F1)
		disabled := k.Enabled != nil && !k.Enabled()
		ctrl := k.Mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
		alt := k.Mods&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
		shift := k.Mods&vtinput.ShiftPressed != 0
		switch {
		case !ctrl && !alt && !shift:
			set.Normal[i], set.NormalDisabled[i] = k.Label, disabled
		case shift && !ctrl && !alt:
			set.Shift[i], set.ShiftDisabled[i] = k.Label, disabled
		case ctrl && !alt && !shift:
			set.Ctrl[i], set.CtrlDisabled[i] = k.Label, disabled
		case alt && !ctrl && !shift:
			set.Alt[i], set.AltDisabled[i] = k.Label, disabled
		}
	}
	return set
}

// swapPluginPanels moves panel plugins (ProcList and the others) to the other
// side together with the file panels, on Ctrl+U: their AltPanels slot and the
// slot they remember for themselves change sides, so a plugin shown on the
// right stays with the panel that was on the right (f4#312). Other alternative
// panels (quick view, chat, player) are bound to their position and are left
// where they are; when one of them shares the pair with a plugin panel, nothing
// moves.
func (pf *PanelsFrame) swapPluginPanels() {
	var inst [2]*PluginPanelInstance
	found := false
	for i := range inst {
		switch a := pf.AltPanels[i].(type) {
		case nil:
		case *PluginPanelInstance:
			inst[i] = a
			found = true
		default:
			return
		}
	}
	if !found {
		return
	}
	pf.AltPanels[0], pf.AltPanels[1] = pf.AltPanels[1], pf.AltPanels[0]
	for i, a := range inst {
		if a != nil {
			a.slot = 1 - i
		}
	}
}
