package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unxed/vtui"
)

// HotkeyManager handles mapping of key combinations to application actions.
type HotkeyManager struct {
	Bindings map[string]map[string]string // Area -> Key -> ActionName
	Defaults map[string]map[string]string // Area -> Key -> ActionName
	iniPath  string
}

var conditionRegistry = map[string]func() bool{
	"emptycommandline": func() bool {
		if pf := findPanelsFrameAnyScreen(); pf != nil {
			return pf.cmdLine.IsEmpty()
		}
		return false
	},
	"commandlinenotempty": func() bool {
		if pf := findPanelsFrameAnyScreen(); pf != nil {
			return !pf.cmdLine.IsEmpty()
		}
		return false
	},
	"esctoggle": func() bool {
		if !AppConfig.EscTogglePanels {
			return false
		}
		if pf := findPanelsFrameAnyScreen(); pf != nil {
			if !pf.cmdLine.IsEmpty() {
				return false
			}
			if pf.showPanels {
				return true
			}
			if pf.termView == nil {
				return false
			}
			return !pf.termView.UseAltScreen && !pf.isPtyBusy()
		}
		return false
	},
	// noaltscreenapp gates keys that must reach an interactive AltScreen
	// application (mc, htop) instead of triggering f4's own actions.
	"noaltscreenapp": func() bool {
		if pf := findPanelsFrameAnyScreen(); pf != nil {
			return pf.showPanels || (pf.termView != nil && !pf.termView.UseAltScreen)
		}
		return false
	},
	// noterminalapp is the stricter sibling of noaltscreenapp: it also
	// stands down for a plain child process that is merely busy (a shell
	// command, a REPL). With the panels hidden such a process owns the
	// keyboard and the command line is not even drawn, so actions that
	// type into it must not fire. With the panels shown nothing is in the
	// way, which keeps the Shell binding of such a key unconditional.
	"noterminalapp": func() bool {
		if pf := findPanelsFrameAnyScreen(); pf != nil {
			return pf.showPanels || (pf.termView != nil && !pf.termView.UseAltScreen && !pf.isPtyBusy())
		}
		return false
	},
	// terminalquiet reports a hidden-panels terminal with no AltScreen app
	// and no busy PTY, so F3/F4 may open the terminal log instead of
	// being forwarded to the running application.
	"terminalquiet": func() bool {
		if pf := findPanelsFrameAnyScreen(); pf != nil {
			return pf.termView != nil && !pf.termView.UseAltScreen && !pf.isPtyBusy()
		}
		return false
	},
	// altpanelvisible reports that an info or quick-view panel is shown,
	// gating the plain-letter toggles that belong to those panels.
	"altpanelvisible": func() bool {
		if pf := findPanelsFrameAnyScreen(); pf != nil {
			for _, a := range pf.altPanels {
				if a != nil && (a.Kind() == "info" || a.Kind() == "quick_view") {
					return true
				}
			}
		}
		return false
	},
}

// GetConditions returns the user-friendly names of all registered conditions.
func GetConditions() []string {
	return []string{"None", "EmptyCommandLine", "CommandLineNotEmpty", "EscToggle", "TerminalQuiet", "AltPanelVisible", "NoAltScreenApp", "NoTerminalApp"}
}

// RegisterCondition adds a dynamic boolean check accessible by hotkey bindings.
func RegisterCondition(name string, fn func() bool) {
	conditionRegistry[strings.ToLower(name)] = fn
}

var GlobalHotkeysMgr *HotkeyManager

func NewHotkeyManager(iniPath string) *HotkeyManager {
	hm := &HotkeyManager{
		Bindings: make(map[string]map[string]string),
		Defaults: make(map[string]map[string]string),
		iniPath:  iniPath,
	}
	hm.initDefaults()
	hm.Load()
	return hm
}

// GetActiveBindings returns a map of Area -> Key -> ActionName containing all active bindings.
func (hm *HotkeyManager) GetActiveBindings() map[string]map[string]string {
	res := make(map[string]map[string]string)
	for area, binds := range hm.Defaults {
		res[area] = make(map[string]string)
		for k, v := range binds {
			res[area][k] = v
		}
	}
	for area, binds := range hm.Bindings {
		if res[area] == nil {
			res[area] = make(map[string]string)
		}
		for k, v := range binds {
			if v == "None" || v == "" {
				delete(res[area], k)
			} else {
				res[area][k] = v
			}
		}
	}
	return res
}

// GetKeyForAction searches for a key combination bound to the given action in an area.
func (hm *HotkeyManager) GetKeyForAction(area, actionName string) string {
	areas := []string{area}
	if area != "Common" {
		areas = append(areas, "Common")
	}

	// Preserve the action author's declared preference. Several terminal
	// actions deliberately list a conditional plain function key first and a
	// modified always-available fallback second. Iterating the binding map made
	// the menu shortcut alternate randomly on every semantic scene rebuild.
	if action, ok := actionRegistry[strings.ToLower(actionName)]; ok {
		for _, candidateArea := range areas {
			binds := hm.Bindings[candidateArea]
			for _, keySpec := range action.DefaultKeys {
				key, _, _ := strings.Cut(keySpec, ":")
				if key != "" && strings.EqualFold(activeBindingAction(binds[key]), actionName) {
					return key
				}
			}
		}
	}

	// User-defined bindings have no registry order. Sort them so their menu
	// representation is stable across rebuilds and process runs.
	for _, candidateArea := range areas {
		binds := hm.Bindings[candidateArea]
		keys := make([]string, 0, len(binds))
		for key := range binds {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if strings.EqualFold(activeBindingAction(binds[key]), actionName) {
				return key
			}
		}
	}
	return ""
}

func activeBindingAction(binding string) string {
	if binding == "" {
		return ""
	}
	parts := strings.SplitN(binding, ":", 2)
	if len(parts) == 2 {
		condName := strings.ToLower(strings.TrimSpace(parts[1]))
		if condFn, ok := conditionRegistry[condName]; ok && !condFn() {
			return ""
		}
	}
	return parts[0]
}

// FormatKeyForUI converts a raw key string (like CtrlShiftF5) into a pretty UI string (Ctrl+Shift+F5).
func FormatKeyForUI(key string) string {
	if key == "" {
		return ""
	}
	var parts []string
	if strings.HasPrefix(key, "Ctrl") {
		parts = append(parts, "Ctrl")
		key = key[4:]
	}
	if strings.HasPrefix(key, "Alt") {
		parts = append(parts, "Alt")
		key = key[3:]
	}
	if strings.HasPrefix(key, "Shift") {
		parts = append(parts, "Shift")
		key = key[5:]
	}
	if key != "" {
		parts = append(parts, key)
	}
	return strings.Join(parts, "+")
}

// NativeShortcutsForAction returns framework-owned shortcuts that still reach
// action in area. Native keys are intentionally absent from Defaults, because
// vtui must offer them to the focused frame before running its fallback. An
// explicit user binding on the same key can nevertheless override or silence
// that fallback, so do not advertise a native shortcut that is currently
// claimed by another action (or by None).
func NativeShortcutsForAction(area string, action Action) []string {
	seen := make(map[string]bool)
	var shortcuts []string
	for _, spec := range action.NativeKeys {
		key, condition, _ := strings.Cut(spec, ":")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !nativeShortcutConditionTrue(area, condition) {
			continue
		}
		if nativeShortcutOwnedByCurrentContext(action.Name, key) {
			continue
		}
		if GlobalHotkeysMgr != nil {
			bound := GlobalHotkeysMgr.GetAction(area, key)
			if bound != "" && !strings.EqualFold(bound, action.Name) {
				continue
			}
		}
		formatted := FormatKeyForUI(key)
		if formatted == "" || seen[formatted] {
			continue
		}
		seen[formatted] = true
		shortcuts = append(shortcuts, formatted)
	}
	sort.Strings(shortcuts)
	return shortcuts
}

// nativeShortcutOwnedByCurrentContext filters framework fallbacks that never
// reach the advertised action in the active frame. This is separate from
// HotkeyManager overrides: these keys are consumed directly by the frame
// before vtui gets a chance to apply its workspace/help fallback.
func nativeShortcutOwnedByCurrentContext(actionName, key string) bool {
	if vtui.FrameManager == nil {
		return false
	}
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		return false
	}
	// TypeUser modal frames (notably Help and Screen Grabber) own their input
	// before vtui's framework fallbacks. Some deliberately release individual
	// keys, but there is no ownership API that can prove that generically; omit
	// native hints rather than advertise a chord the current modal may swallow.
	if top.IsModal() {
		return true
	}

	// Ctrl+N's native implementation is an active-stack CmResize broadcast.
	// It is truthful only when the current screen actually contains a panels
	// frame. The palette action itself can still fork the nearest panels frame
	// from editor/viewer/image/queue screens via actionWorkspaceNew.
	if strings.EqualFold(actionName, "Workspace.New") && strings.EqualFold(key, "CtrlN") {
		frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
		hasPanels := false
		for _, frame := range frames {
			if panels, ok := frame.(*PanelsFrame); ok && !panels.closed {
				hasPanels = true
				break
			}
		}
		if !hasPanels {
			return true
		}
	}

	switch frame := top.(type) {
	case *ImageView:
		// F12 belongs to the gallery while the image viewer is active, not to
		// vtui's workspace list fallback.
		return strings.EqualFold(key, "F12")
	case *QueueFrame:
		// An active queue swallows Ctrl+W to preserve running operations.
		return strings.EqualFold(key, "CtrlW") && queueHasActiveTasks()
	case *PanelsFrame:
		terminalOwnsInput := !frame.showPanels &&
			((frame.termView != nil && frame.termView.UseAltScreen) || frame.isPtyBusy())
		if !terminalOwnsInput {
			return false
		}
		// PanelsFrame explicitly releases workspace cycling and, when the
		// preference is enabled, Ctrl+N before raw terminal forwarding.
		if strings.EqualFold(key, "CtrlTab") || strings.EqualFold(key, "CtrlShiftTab") {
			return false
		}
		if strings.EqualFold(key, "CtrlN") && AppConfig.TerminalCtrlNWorkspace {
			return false
		}
		return true
	}
	return false
}

func nativeShortcutConditionTrue(area, condition string) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true
	}
	switch strings.ToLower(condition) {
	case "frameworknoterminalapp":
		return !strings.EqualFold(area, "Terminal") || commandPaletteConditionTrue("NoTerminalApp")
	case "terminalctrlnworkspace":
		return !strings.EqualFold(area, "Terminal") || AppConfig.TerminalCtrlNWorkspace
	default:
		return commandPaletteConditionTrue(condition)
	}
}

// initDefaults builds the default bindings from the action registry.
// The registry is the single source of truth: every action carrying
// DefaultKeys gets them bound in its Area (plus any DefaultAreas).
// A key entry may carry a ":Condition" suffix (e.g. "Esc:EscToggle").
func (hm *HotkeyManager) initDefaults() {
	hm.Defaults = make(map[string]map[string]string)
	for _, a := range GetOrderedActions() {
		if len(a.DefaultKeys) == 0 {
			continue
		}
		areas := append([]string{a.Area}, a.DefaultAreas...)
		for _, area := range areas {
			if area == "" {
				continue
			}
			for _, keySpec := range a.DefaultKeys {
				key, cond, _ := strings.Cut(keySpec, ":")
				if key == "" {
					continue
				}
				binding := a.Name
				if cond != "" {
					binding += ":" + cond
				}
				if hm.Defaults[area] == nil {
					hm.Defaults[area] = make(map[string]string)
				}
				hm.Defaults[area][key] = binding
			}
		}
	}
}

// Load reads bindings from the INI file, overlaying them onto the defaults.
func (hm *HotkeyManager) Load() {
	hm.Bindings = make(map[string]map[string]string)

	// Copy defaults
	for area, binds := range hm.Defaults {
		hm.Bindings[area] = make(map[string]string)
		for k, v := range binds {
			hm.Bindings[area][k] = v
		}
	}

	if hm.iniPath == "" {
		return
	}

	ini := LoadIni(hm.iniPath)
	for area, binds := range ini.data {
		if hm.Bindings[area] == nil {
			hm.Bindings[area] = make(map[string]string)
		}
		for key, action := range binds {
			if action == "" {
				delete(hm.Bindings[area], key)
			} else if strings.EqualFold(action, "none") {
				hm.Bindings[area][key] = "None"
			} else {
				hm.Bindings[area][key] = action
			}
		}
	}
}

// Save writes only overridden or new bindings to the INI file.
func (hm *HotkeyManager) Save() {
	if hm.iniPath == "" {
		return
	}

	var sb strings.Builder
	for area, binds := range hm.Bindings {
		diffs := make(map[string]string)

		// Find overrides and additions
		for key, action := range binds {
			if defAction, ok := hm.Defaults[area][key]; !ok || defAction != action {
				diffs[key] = action
			}
		}

		// Find removals
		if defArea, ok := hm.Defaults[area]; ok {
			for key := range defArea {
				if _, exists := binds[key]; !exists {
					diffs[key] = "None"
				}
			}
		}

		if len(diffs) > 0 {
			sb.WriteString(fmt.Sprintf("[%s]\n", area))
			for key, action := range diffs {
				sb.WriteString(fmt.Sprintf("%s=%s\n", key, action))
			}
			sb.WriteString("\n")
		}
	}

	os.MkdirAll(filepath.Dir(hm.iniPath), 0755)
	os.WriteFile(hm.iniPath, []byte(sb.String()), 0644)
}

// delKeyAlias returns the other spelling of a Del key string, or "" when the
// key is not a Del key. "ShiftDel" <-> "ShiftNumDel", "Del" <-> "NumDel".
//
// EventToFarString derives the Num prefix from the EnhancedKey flag, but no
// input backend f4 supports reports that flag consistently for Delete: the
// GUI hosts (ebiten, gogpu, x11, wayland) build events with plain Shift/Ctrl/
// Alt state and never set it, and CSI 3~ carries no such flag either, so the
// navigation Del arrives named "NumDel" and every "…Del" binding silently
// misses. far2l has the same two names and binds them to one handler
// (editor.cpp: KEY_SHIFTDEL/KEY_SHIFTNUMDEL/KEY_SHIFTDECIMAL); resolving the
// alias here keeps a binding working whichever name the backend produced.
func delKeyAlias(key string) string {
	if strings.HasSuffix(key, "NumDel") {
		return strings.TrimSuffix(key, "NumDel") + "Del"
	}
	if strings.HasSuffix(key, "Del") {
		return strings.TrimSuffix(key, "Del") + "NumDel"
	}
	return ""
}

// GetAction returns the action name mapped to the key in the given area.
func (hm *HotkeyManager) GetAction(area, key string) string {
	if binds, ok := hm.Bindings[area]; ok {
		if binding, ok := binds[key]; ok {
			if action := activeBindingAction(binding); action != "" {
				return action
			}
		}
	}
	if area != "Common" {
		if binds, ok := hm.Bindings["Common"]; ok {
			if binding, ok := binds[key]; ok {
				if action := activeBindingAction(binding); action != "" {
					return action
				}
			}
		}
	}

	// Nothing is bound under this exact name. Before giving up, try the other
	// spelling of a Del key (see delKeyAlias): an explicit binding always wins,
	// this only fills in the name the backend did not produce.
	if alias := delKeyAlias(key); alias != "" {
		if binds, ok := hm.Bindings[area]; ok {
			if binding, ok := binds[alias]; ok {
				if action := activeBindingAction(binding); action != "" {
					return action
				}
			}
		}
		if area != "Common" {
			if binds, ok := hm.Bindings["Common"]; ok {
				if binding, ok := binds[alias]; ok {
					if action := activeBindingAction(binding); action != "" {
						return action
					}
				}
			}
		}
	}
	return ""
}

// Bind assigns an action to a key in a specific area.
func (hm *HotkeyManager) Bind(area, key, action string) {
	if hm.Bindings[area] == nil {
		hm.Bindings[area] = make(map[string]string)
	}
	hm.Bindings[area][key] = action
}

// Unbind removes a hotkey binding.
func (hm *HotkeyManager) Unbind(area, key string) {
	if binds, ok := hm.Bindings[area]; ok {
		delete(binds, key)
	}
}

// KeyBarLabelsForArea resolves F1-F12 keybar labels for the given area
// through the active hotkey bindings, falling back to the provided
// defaults when a key has no binding. A key explicitly unbound ("None")
// gets an empty label.
func KeyBarLabelsForArea(area string, fallbacks *vtui.KeySet) *vtui.KeySet {
	var fbNormal, fbShift, fbAlt, fbCtrl vtui.KeyBarLabels
	var fbNormalIcons, fbShiftIcons, fbAltIcons, fbCtrlIcons vtui.KeyBarIconNames
	if fallbacks != nil {
		fbNormal, fbShift, fbAlt, fbCtrl = fallbacks.Normal, fallbacks.Shift, fallbacks.Alt, fallbacks.Ctrl
		fbNormalIcons, fbShiftIcons = fallbacks.NormalIcons, fallbacks.ShiftIcons
		fbAltIcons, fbCtrlIcons = fallbacks.AltIcons, fallbacks.CtrlIcons
	}
	resolve := func(prefix, keyNum, fb, fbIcon string) (string, string) {
		if hm := GlobalHotkeysMgr; hm != nil {
			if actName := hm.GetAction(area, prefix+keyNum); actName != "" {
				if strings.EqualFold(actName, "none") {
					return "", ""
				}
				if act, ok := GetAction(actName); ok {
					return plainLabel(act.DisplayLabel()), keyBarIconForAction(act.Name)
				}
			}
		}
		return fb, fbIcon
	}

	set := &vtui.KeySet{}
	for i := 0; i < 12; i++ {
		keyNum := fmt.Sprintf("F%d", i+1)
		set.Normal[i], set.NormalIcons[i] = resolve("", keyNum, fbNormal[i], fbNormalIcons[i])
		set.Shift[i], set.ShiftIcons[i] = resolve("Shift", keyNum, fbShift[i], fbShiftIcons[i])
		set.Alt[i], set.AltIcons[i] = resolve("Alt", keyNum, fbAlt[i], fbAltIcons[i])
		set.Ctrl[i], set.CtrlIcons[i] = resolve("Ctrl", keyNum, fbCtrl[i], fbCtrlIcons[i])
	}
	return set
}
