package app

import (
	"github.com/unxed/f4/internal/panel"
	"strings"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// The key router. It asks the macro engine first, which is why it used to live
// with it, but everything it asks afterwards — the hotkey manager, the action
// registry, the plugin actions, the panels — is the application's. The engine
// keeps the recording and playback; the dispatching is here.

func macroCurrentArea() string {
	if vtui.FrameManager == nil {
		return "Common"
	}
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		return "Common"
	}
	switch top.GetType() {
	case vtui.TypeDialog:
		return "Dialog"
	case vtui.TypeMenu:
		if menu, ok := top.(*vtui.VMenu); ok {
			if strings.Contains(menu.GetTitle(), "Drive") {
				return "Disks"
			}
		}
		return "Menu"
	case vtui.TypeUser + 1:
		if pf, ok := top.(*panel.PanelsFrame); ok {
			if !pf.ShowPanels {
				return "Terminal"
			}
		}
		return "Shell"
	case vtui.TypeUser + 2:
		return "Editor"
	case vtui.TypeUser + 3:
		return "Viewer"
	}
	return "Other"
}

// isPanelFastFindToggleKey identifies contextual panel-toggle keys owned by
// Fast Find. They must reach panel.PanelsFrame before macros and configurable
// hotkeys, otherwise Esc/Del -> Panel.Toggle hides the panels first.
func IsPanelFastFindToggleKey(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.KeyEventType || !e.KeyDown ||
		(e.VirtualKeyCode != vtinput.VK_ESCAPE && e.VirtualKeyCode != vtinput.VK_DELETE) {
		return false
	}
	mods := e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed |
		vtinput.LeftAltPressed | vtinput.RightAltPressed | vtinput.ShiftPressed)
	if mods != 0 || vtui.FrameManager == nil {
		return false
	}
	pf, ok := vtui.FrameManager.GetTopFrame().(*panel.PanelsFrame)
	if !ok || !pf.ShowPanels {
		return false
	}
	fsp := pf.GetActivePanel()
	return fsp != nil && fsp.FastFindMode
}

func IsPanelFastFindActive() bool {
	if vtui.FrameManager == nil {
		return false
	}
	pf, ok := vtui.FrameManager.GetTopFrame().(*panel.PanelsFrame)
	if !ok || !pf.ShowPanels {
		return false
	}
	fsp := pf.GetActivePanel()
	return fsp != nil && fsp.FastFindMode
}

func macroFilter(m *macro.MacroManager, e *vtinput.InputEvent) bool {
	if e.Type != vtinput.KeyEventType {
		return false
	}

	// The user's key remap (keymap.ini) substitutes the key before anything
	// else sees the event, so macros, plugin interception, configurable
	// hotkeys and the frames themselves all agree on which key was pressed.
	if !keymap.ApplyKeyRemap(macroCurrentArea(), e) {
		// The built-in Mac layout only sees what keymap.ini left behind, so
		// a rule the user wrote by hand still wins over it.
		keymap.ApplyMacKeys(macroCurrentArea(), e)
	}

	// Ctrl+. toggles recording. We check both VK and Char for better terminal compatibility.
	isCtrlDot := (e.VirtualKeyCode == vtinput.VK_OEM_PERIOD || e.Char == '.') &&
		(e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed)) != 0

	if isCtrlDot {
		if !e.KeyDown {
			return true // Consume KeyUp of the trigger
		}
		m.ToggleRecording(macroCurrentArea())
		return true // Trigger is ALWAYS consumed
	}

	// The command palette is the application-wide escape hatch for finding
	// commands. Resolve it before recording/assignment so the opening chord is
	// neither captured in the macro buffer nor shadowed by macro playback.
	if e.KeyDown {
		currentArea := macroCurrentArea()
		hotkeyStr := keymap.EventToHotkeyString(e)
		if hm := keymap.GlobalHotkeysMgr; hm != nil {
			if actionName := keymap.ConfiguredHotkeyAction(hm, currentArea, hotkeyStr); strings.EqualFold(actionName, CommandPaletteActionName) {
				RunAction(actionName)
				return true
			}
		}
		if CommandPaletteLegacyShortcut(currentArea, e) {
			RunAction(CommandPaletteActionName)
			return true
		}
	}

	// Once the palette is open, its remaining query and navigation keys belong
	// to the dialog. In particular, do not record them into a macro that the
	// user may be stopping from the palette itself. The palette chord itself was
	// handled above, so pressing it again is still consumed without adding a
	// second dialog.
	if vtui.FrameManager != nil {
		if _, open := vtui.FrameManager.GetTopFrame().(*CommandPaletteDialog); open {
			return false
		}
	}

	if m.Recording || m.Assigning {
		if e.KeyDown && m.Recording {
			switch e.VirtualKeyCode {
			case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
				vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
				vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU,
				vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
				// Ignore standalone modifier keys
			default:
				m.Buffer = append(m.Buffer, e)
			}
		}
		return false // Let it pass to the UI so user sees what they type / dialog catches it
	}

	if !e.KeyDown {
		return false
	}

	currentArea := macroCurrentArea()
	if currentArea == "Shell" && IsPanelFastFindActive() {
		// zoin-bot: Fast Find is a transient panel input mode, so Shell macros
		// must not consume keys before panel.PanelsFrame can update its query.
		return false
	}
	if currentArea == "Shell" &&
		((keymap.IsPanelBookmarkHotkey(e) && !keymap.ConfigurableHotkeyOwnsPanelBookmark(keymap.GlobalHotkeysMgr, currentArea, e)) ||
			IsPanelFastFindToggleKey(e)) {
		return false
	}

	// Check if this key triggers a macro
	keyStr := keymap.EventToFarString(e)

	if areaMacros, ok := m.Macros[currentArea]; ok {
		if seq, ok := areaMacros[keyStr]; ok {
			vtui.DebugLog("MACRO: Playing back macro for %s in area %s", keyStr, currentArea)
			vtui.FrameManager.InjectEvents(seq)
			return true
		}
	}

	if commonMacros, ok := m.Macros["Common"]; ok {
		if seq, ok := commonMacros[keyStr]; ok {
			vtui.DebugLog("MACRO: Playing back macro for %s in area Common", keyStr)
			vtui.FrameManager.InjectEvents(seq)
			return true
		}
	}

	// Recorded macros win over scripted ones, as they do in Far.
	if m.Lua != nil && m.Lua.Trigger(currentArea, e) {
		vtui.DebugLog("MACRO: Running Lua macro for %s in area %s", keyStr, currentArea)
		return true
	}

	// Plugin key interception: global plugin hotkeys and the active
	// panel's PanelController may override built-in hotkeys, so they
	// are consulted before the hotkey manager.
	if vtui.FrameManager != nil {
		if top := vtui.FrameManager.GetTopFrame(); top != nil {
			if pi, ok := top.(interface {
				InterceptPluginKey(*vtinput.InputEvent) bool
			}); ok && pi.InterceptPluginKey(e) {
				return true
			}
		}
	}

	// A frame in a modal input state (e.g. editor autocomplete, panel
	// fast-find) may veto system hotkey dispatch: the key then falls
	// through to the frame's own ProcessKey, which handles such states
	// before anything else.
	if vtui.FrameManager != nil {
		if top := vtui.FrameManager.GetTopFrame(); top != nil {
			if v, ok := top.(interface {
				VetoActionKey(*vtinput.InputEvent) bool
			}); ok && v.VetoActionKey(e) {
				return false
			}
		}
	}

	// Hotkey Manager evaluation (System actions)
	if hm := keymap.GlobalHotkeysMgr; hm != nil {
		keyStr := keymap.EventToHotkeyString(e)
		if actionName := keymap.ConfiguredHotkeyAction(hm, currentArea, keyStr); actionName != "" {
			// F4-assigned plugin shortcuts are menu accelerators, matching
			// far2l: the F11 plugin menu renders them as ampersand hotkeys.
			// They must not steal printable characters from the panel command
			// line while remaining persisted for that menu and its display.
			// A chord assigned in the hotkey dialog (Ctrl+F9 and friends) is a
			// real hotkey and keeps being dispatched here.
			if keymap.IsPluginActionName(actionName) && panel.IsPluginMenuHotkey(keyStr) {
				return false
			}
			if strings.EqualFold(actionName, "none") {
				return true // Intercept and silence (explicitly unbound)
			}
			vtui.DebugLog("HOTKEY: Executing action %s for %s in area %s", actionName, keyStr, currentArea)
			// A configured binding owns the event even when its action cannot
			// currently run. Letting the same key fall through to the frame
			// dispatcher can trigger an unrelated command and leave a stale
			// overlay behind (for example, when the selected panel entry is
			// the parent directory and an archive command has no target).
			RunAction(actionName)
			return true
		}
	}

	return false
}

// ToggleRecording is shared by the native Ctrl+. handler and the command
// palette. Stopping remains deferred so the assignment dialog is pushed only
// after the current input dispatch (or palette close) has completed.
func macroLookupHotkey(m *macro.MacroManager, e *vtinput.InputEvent) bool {
	if m == nil || e == nil || e.Type != vtinput.KeyEventType || !e.KeyDown {
		return false
	}
	hm := keymap.GlobalHotkeysMgr
	if hm == nil {
		return false
	}
	keyStr := keymap.EventToHotkeyString(e)
	if keyStr == "" {
		return false
	}
	area := macroCurrentArea()
	actionName := keymap.ConfiguredHotkeyAction(hm, area, keyStr)
	if actionName == "" {
		return false
	}
	// Plugin menu accelerators are activated by the F11 menu's ampersand
	// hotkeys. They are deliberately not global Shell hotkeys, so an injected
	// key (for example from a key-bar click) must not bypass that rule either.
	if keymap.IsPluginActionName(actionName) && panel.IsPluginMenuHotkey(keyStr) {
		return false
	}
	if strings.EqualFold(actionName, "none") {
		return true
	}
	vtui.DebugLog("HOTKEY: Injected %s → action %s in area %s", keyStr, actionName, area)
	// The injected path has the same ownership rule as Filter: once a
	// configured action matched, its result must not expose the key to the
	// frame below it.
	RunAction(actionName)
	return true
}
