package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Plugin menu entries are actions too, but their lifetime is controlled by a
// plugin registration rather than by the built-in action registry. Keeping a
// separate namespace lets hotkeys.ini refer to them without leaving stale
// Action values behind when an RPC plugin disconnects.
func pluginCommandActionName(id string) string { return "Plugin.Command." + id }

func legacyPluginActionName(index int) string {
	return "Plugin.Legacy." + strconv.Itoa(index)
}

func isPluginActionName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(name, "plugin.command.") || strings.HasPrefix(name, "plugin.legacy.")
}

func pluginActionForName(name string) (Action, bool) {
	rawName := strings.TrimSpace(name)
	lowerName := strings.ToLower(rawName)
	switch {
	case strings.HasPrefix(lowerName, "plugin.command."):
		id := strings.TrimSpace(rawName[len("Plugin.Command."):])
		pluginCommandRegistry.RLock()
		registered, ok := pluginCommandRegistry.byID[strings.ToLower(id)]
		if ok {
			registered.command = clonePluginCommand(registered.command)
		}
		pluginCommandRegistry.RUnlock()
		if !ok {
			return Action{}, false
		}
		command := registered.command
		actionName := pluginCommandActionName(command.ID)
		return Action{
			Name:        actionName,
			Area:        "Shell",
			Label:       pluginCommandDisplayLabel(command),
			Description: pluginCommandDisplayDescription(command),
			Handler:     func() bool { return runPluginHotkeyAction(actionName) },
		}, true
	case strings.HasPrefix(lowerName, "plugin.legacy."):
		index, err := strconv.Atoi(strings.TrimSpace(rawName[len("Plugin.Legacy."):]))
		if err != nil || index < 0 {
			return Action{}, false
		}
		items := pluginMenuItemsSnapshot()
		if index >= len(items) {
			return Action{}, false
		}
		item := items[index]
		actionName := item.ActionName
		if actionName == "" {
			actionName = legacyPluginActionName(index)
		}
		return Action{
			Name:        actionName,
			Area:        "Shell",
			Label:       item.Label,
			Description: "Run the selected plugin command",
			Handler:     func() bool { return runPluginHotkeyAction(actionName) },
		}, true
	default:
		return Action{}, false
	}
}

func runPluginHotkeyAction(name string) bool {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(strings.ToLower(name), "plugin.command.") {
		id := strings.TrimSpace(name[len("Plugin.Command."):])
		pluginCommandRegistry.RLock()
		registered, ok := pluginCommandRegistry.byID[strings.ToLower(id)]
		if ok {
			registered.command = clonePluginCommand(registered.command)
		}
		pluginCommandRegistry.RUnlock()
		if !ok {
			return false
		}
		pf := findPanelsFrame()
		if pf == nil {
			return false
		}
		return executeRegisteredPluginCommand(registered.command.Location, registered.command.ID, pf)
	}

	if strings.HasPrefix(strings.ToLower(name), "plugin.legacy.") {
		index, err := strconv.Atoi(strings.TrimSpace(name[len("Plugin.Legacy."):]))
		if err != nil || index < 0 {
			return false
		}
		items := pluginMenuItemsSnapshot()
		if index >= len(items) || items[index].Handler == nil {
			return false
		}
		if pf := findPanelsFrame(); pf != nil {
			items[index].Handler(pf)
			return true
		}
	}
	return false
}

func pluginActionShortcut(name string) string {
	if GlobalHotkeysMgr == nil {
		return ""
	}
	if key := GlobalHotkeysMgr.GetKeyForAction("Shell", name); key != "" {
		return FormatKeyForUI(key)
	}
	return ""
}

func pluginCommandShortcut(command vfs.PluginCommand) string {
	if shortcut := pluginActionShortcut(pluginCommandActionName(command.ID)); shortcut != "" {
		return shortcut
	}
	return command.Shortcut
}

// A plugin menu hot key is a single letter or digit. The F11 menu dispatches
// it through the same ampersand accelerator machinery every other vtui menu
// uses, so a key that cannot be typed as one character -- Del, Enter, Ctrl+F9
// -- is not a menu hot key at all and must never be stored as one.
func pluginMenuHotkeyRune(key string) rune {
	runes := []rune(strings.TrimSpace(key))
	if len(runes) != 1 {
		return 0
	}
	if unicode.IsLetter(runes[0]) || unicode.IsDigit(runes[0]) {
		return unicode.ToUpper(runes[0])
	}
	return 0
}

func isPluginMenuHotkey(key string) bool { return pluginMenuHotkeyRune(key) != 0 }

// pluginReservedKeys are bare keys the panels and the plugin menu need for
// themselves. The first version of the F4 dialog accepted any key at all, so a
// user could hand Del to a plugin and end up with no working Del anywhere --
// including the plugin menu, which needs Del to take that very binding back.
var pluginReservedKeys = map[string]bool{
	"Del": true, "NumDel": true, "Ins": true, "BS": true, "Tab": true,
	"Enter": true, "NumEnter": true, "Esc": true, "Space": true,
	"Up": true, "Down": true, "Left": true, "Right": true,
	"Home": true, "End": true, "PgUp": true, "PgDn": true,
}

func isReservedPluginHotkey(key string) bool {
	return pluginReservedKeys[strings.TrimSpace(key)]
}

// restoreDefaultBinding gives a key back to its built-in action. Bindings
// starts life as a copy of Defaults, so a plain Unbind on a key that has a
// default looks like a deliberate "None" to Save and would kill the built-in
// shortcut for good.
func restoreDefaultBinding(hm *HotkeyManager, area, key string) {
	if hm == nil {
		return
	}
	if def, ok := hm.Defaults[area][key]; ok {
		hm.Bind(area, key, def)
		return
	}
	hm.Unbind(area, key)
}

// dropReservedPluginBindings repairs configurations written by that first
// dialog. It runs on every load, so a hotkeys.ini carrying "Del=Plugin.*" stops
// shadowing the file deletion command without the user having to edit the file
// by hand.
func (hm *HotkeyManager) dropReservedPluginBindings() {
	if hm == nil {
		return
	}
	for area, binds := range hm.Bindings {
		for key, binding := range binds {
			name := strings.SplitN(binding, ":", 2)[0]
			if isPluginActionName(name) && isReservedPluginHotkey(key) {
				restoreDefaultBinding(hm, area, key)
			}
		}
	}
}

// pluginMenuEntry is one row of the F11 menu: the accelerator it owns is kept
// apart from a chord such as Ctrl+F9, which the hotkey manager dispatches
// globally and the menu only displays.
type pluginMenuEntry struct {
	Label      string
	ActionName string
	Declared   string // shortcut declared by the plugin itself
	Chord      string
	Hotkey     string
}

// applyBinding splits what is currently configured for the entry into an
// accelerator and a display-only chord.
func (e *pluginMenuEntry) applyBinding() {
	e.Hotkey, e.Chord = "", ""
	if key := pluginActionConfiguredKey(e.ActionName); key != "" {
		if r := pluginMenuHotkeyRune(key); r != 0 {
			e.Hotkey = string(r)
		} else {
			e.Chord = FormatKeyForUI(key)
		}
		return
	}
	declared := strings.TrimSpace(e.Declared)
	if r := pluginMenuHotkeyRune(declared); r != 0 {
		e.Hotkey = string(r)
		return
	}
	e.Chord = declared
}

// Shortcut is what the left-hand column shows for the entry.
func (e pluginMenuEntry) Shortcut() string {
	if e.Hotkey != "" {
		return e.Hotkey
	}
	return e.Chord
}

// resolvePluginMenuHotkeys makes every accelerator in the menu unique. Far can
// cycle through rows sharing a letter; a menu that prints the letters in their
// own column cannot, and two identical letters in that column are worse than
// one row without an accelerator.
func resolvePluginMenuHotkeys(entries []pluginMenuEntry) {
	used := make(map[rune]bool, len(entries))
	claim := func(key string) string {
		r := pluginMenuHotkeyRune(key)
		if r == 0 || used[r] {
			return ""
		}
		used[r] = true
		return string(r)
	}
	// Assigned letters go first: a letter the user picked with F4 must not be
	// lost to an ampersand that happens to sit higher up the menu.
	for i := range entries {
		entries[i].Hotkey = claim(entries[i].Hotkey)
	}
	// Ampersand markers coming from the plugin labels fill in the rest.
	for i := range entries {
		if entries[i].Hotkey != "" || entries[i].Chord != "" {
			continue
		}
		if _, hotkey, _ := vtui.ParseAmpersandString(entries[i].Label); hotkey != 0 {
			entries[i].Hotkey = claim(string(hotkey))
		}
	}
}

func buildPluginMenuEntries(items []PluginMenuItem, commands []vfs.PluginCommand) []pluginMenuEntry {
	entries := make([]pluginMenuEntry, 0, len(items)+len(commands))
	for index, item := range items {
		actionName := item.ActionName
		if actionName == "" {
			actionName = legacyPluginActionName(index)
		}
		entries = append(entries, pluginMenuEntry{Label: item.Label, ActionName: actionName})
	}
	for _, command := range commands {
		entries = append(entries, pluginMenuEntry{
			Label:      pluginCommandDisplayLabel(command),
			ActionName: pluginCommandActionName(command.ID),
			Declared:   command.Shortcut,
		})
	}
	refreshPluginMenuEntries(entries)
	return entries
}

// refreshPluginMenuEntries re-reads the bindings for the whole menu. Assigning
// a letter takes it away from whoever held it before, so a single row cannot be
// updated on its own.
func refreshPluginMenuEntries(entries []pluginMenuEntry) {
	for i := range entries {
		entries[i].applyBinding()
	}
	resolvePluginMenuHotkeys(entries)
}

// pluginMenuShortcutWidth keeps the column one character wide even when nothing
// is assigned yet, so the first F4 assignment does not shift every label.
func pluginMenuShortcutWidth(entries []pluginMenuEntry) int {
	width := 1
	for _, entry := range entries {
		if w := runewidth.StringWidth(entry.Shortcut()); w > width {
			width = w
		}
	}
	return width
}

// pluginMenuItemText renders the shortcut in a stable column before the
// command name. A single unmodified character remains an ampersand hotkey so
// it can also activate the item while the F11 menu is open. Longer chords are
// display-only metadata; their actual dispatch happens in the hotkey manager.
func pluginMenuItemText(label, shortcut string, shortcutWidth int) string {
	cleanLabel, _, _ := vtui.ParseAmpersandString(label)
	shortcut = strings.TrimSpace(shortcut)
	if shortcutWidth < runewidth.StringWidth(shortcut) {
		shortcutWidth = runewidth.StringWidth(shortcut)
	}
	prefix := strings.Repeat(" ", shortcutWidth-runewidth.StringWidth(shortcut))
	if shortcut != "" {
		if len([]rune(shortcut)) == 1 && !unicode.IsSpace([]rune(shortcut)[0]) {
			prefix += "&" + shortcut
		} else {
			prefix += shortcut
		}
	}
	return prefix + " " + cleanLabel
}

func configuredHotkeyBinding(hm *HotkeyManager, actionName string) (string, string) {
	if hm == nil {
		return "", ""
	}
	for _, area := range []string{"Shell", "Common"} {
		var keys []string
		for key, binding := range hm.Bindings[area] {
			namePart := strings.SplitN(binding, ":", 2)[0]
			if strings.EqualFold(namePart, actionName) {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		if len(keys) != 0 {
			return area, keys[0]
		}
	}
	return "", ""
}

func pluginActionConfiguredBinding(name string) (string, string) {
	return configuredHotkeyBinding(GlobalHotkeysMgr, name)
}

func pluginActionConfiguredKey(name string) string {
	_, key := pluginActionConfiguredBinding(name)
	return key
}

func pluginActionDefaultShortcut(name string) string {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	if !strings.HasPrefix(lowerName, "plugin.command.") {
		return ""
	}
	id := strings.TrimSpace(name[len("Plugin.Command."):])
	pluginCommandRegistry.RLock()
	registered, ok := pluginCommandRegistry.byID[strings.ToLower(id)]
	if ok {
		shortcut := registered.command.Shortcut
		pluginCommandRegistry.RUnlock()
		return shortcut
	}
	pluginCommandRegistry.RUnlock()
	return ""
}

func assignPluginHotkey(actionName, label string, onComplete func()) {
	hm := GlobalHotkeysMgr
	if hm == nil || vtui.FrameManager == nil || !isPluginActionName(actionName) {
		return
	}
	vtui.FrameManager.Push(NewPluginHotkeyAssignFrame(hm, actionName, label, onComplete))
}

// bindPluginMenuHotkey stores a letter for the entry. A letter identifies
// exactly one row, so it is taken away from whoever held it, and the entry
// loses whatever it held before: one hot key per plugin, as in Far.
func bindPluginMenuHotkey(hm *HotkeyManager, actionName string, r rune) bool {
	if hm == nil || r == 0 || !isPluginActionName(actionName) {
		return false
	}
	key := string(unicode.ToUpper(r))
	for _, area := range []string{"Shell", "Common"} {
		for boundKey, binding := range hm.Bindings[area] {
			name := strings.SplitN(binding, ":", 2)[0]
			if !isPluginActionName(name) {
				continue
			}
			if strings.EqualFold(boundKey, key) || strings.EqualFold(name, actionName) {
				restoreDefaultBinding(hm, area, boundKey)
			}
		}
	}
	hm.Bind("Shell", key, actionName)
	hm.Save()
	return true
}

func deletePluginHotkey(hm *HotkeyManager, area, key string) bool {
	if hm == nil || area == "" || key == "" {
		return false
	}
	restoreDefaultBinding(hm, area, key)
	hm.Save()
	return true
}

func pluginHotkeyDeleteQuestion(key, label string) string {
	cleanLabel, _, _ := vtui.ParseAmpersandString(label)
	return fmt.Sprintf(Msg("Plugins.HotkeyRemoveQuestion"), FormatKeyForUI(key), cleanLabel)
}

// pluginHotkeyEventRune reports the letter or digit a key event stands for, or
// zero when the event carries a modifier or is not a printable character.
func pluginHotkeyEventRune(e *vtinput.InputEvent) rune {
	if e == nil || e.Type != vtinput.KeyEventType {
		return 0
	}
	mods := normalizeMods(e.ControlKeyState)
	if mods.Contains(vtinput.LeftCtrlPressed) || mods.Contains(vtinput.LeftAltPressed) {
		return 0
	}
	if r := pluginMenuHotkeyRune(string(e.Char)); r != 0 {
		return r
	}
	vk := e.VirtualKeyCode
	if (vk >= 'A' && vk <= 'Z') || (vk >= '0' && vk <= '9') {
		return rune(vk)
	}
	return 0
}

// PluginHotkeyAssignFrame asks for a single letter or digit, the way Far does
// for its plugin menu: Del drops the current assignment, Esc leaves it alone,
// and a key that could never work as a menu accelerator is simply ignored.
type PluginHotkeyAssignFrame struct {
	*vtui.Window
	hm         *HotkeyManager
	actionName string
	onComplete func()
}

func NewPluginHotkeyAssignFrame(hm *HotkeyManager, actionName, label string, onComplete func()) *PluginHotkeyAssignFrame {
	width, height := 48, 9
	f := &PluginHotkeyAssignFrame{
		Window:     vtui.NewCenteredDialog(width, height, Msg("Plugins.HotkeyTitle")),
		hm:         hm,
		actionName: actionName,
		onComplete: onComplete,
	}

	cleanLabel, _, _ := vtui.ParseAmpersandString(label)
	current := Msg("Plugins.HotkeyNone")
	if _, key := configuredHotkeyBinding(hm, actionName); key != "" {
		current = FormatKeyForUI(key)
	}
	lines := []string{
		cleanLabel,
		Msg("Plugins.HotkeyPrompt"),
		fmt.Sprintf(Msg("Plugins.HotkeyCurrent"), current),
		Msg("Plugins.HotkeyHelp"),
	}

	vbox := vtui.NewVBoxLayout(f.X1+2, f.Y1+2, width-4, height-4)
	for i, line := range lines {
		text := vtui.NewText(0, 0, line, vtui.Palette[vtui.ColDialogText])
		f.AddItem(text)
		margins := vtui.Margins{}
		if i > 0 {
			margins.Top = 1
		}
		vbox.Add(text, margins, vtui.AlignCenter)
	}
	vbox.Apply()
	return f
}

func (f *PluginHotkeyAssignFrame) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		return f.Window.ProcessKey(e)
	}
	if !e.KeyDown {
		return false
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE:
		f.finish(false)
		return true
	case vtinput.VK_DELETE, vtinput.VK_BACK:
		changed := false
		if area, key := configuredHotkeyBinding(f.hm, f.actionName); key != "" {
			changed = deletePluginHotkey(f.hm, area, key)
		}
		f.finish(changed)
		return true
	}

	if r := pluginHotkeyEventRune(e); r != 0 {
		f.finish(bindPluginMenuHotkey(f.hm, f.actionName, r))
		return true
	}

	// F-keys, chords and bare modifiers are not menu accelerators. The dialog
	// stays open rather than storing a key the menu could never dispatch.
	return true
}

func (f *PluginHotkeyAssignFrame) finish(changed bool) {
	f.Close()
	if changed && f.onComplete != nil {
		f.onComplete()
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

func (f *PluginHotkeyAssignFrame) ProcessMouse(e *vtinput.InputEvent) bool { return true }
func (f *PluginHotkeyAssignFrame) GetType() vtui.FrameType                 { return vtui.TypeDialog }
func (f *PluginHotkeyAssignFrame) IsModal() bool                           { return true }

func pluginMenuKeyLabels(pf *PanelsFrame) *vtui.KeySet {
	if pf != nil && MacroMgr != nil {
		if base := pf.GetKeyLabels(); base != nil {
			labels := *base
			labels.Normal[3] = "F4"
			return &labels
		}
	}
	return &vtui.KeySet{Normal: vtui.KeyBarLabels{"", "", "", "F4"}}
}

// pluginHotkeyActionsSnapshot includes commands that are currently hidden from
// the F11 menu as well. A user can therefore assign a shortcut once and keep
// it when moving to another drive or when a plugin changes its visibility.
func pluginHotkeyActionsSnapshot() []Action {
	pluginCommandRegistry.RLock()
	commandIDs := append([]string(nil), pluginCommandRegistry.order...)
	pluginCommandRegistry.RUnlock()

	actions := make([]Action, 0, len(commandIDs)+len(pluginMenuItemsSnapshot()))
	for _, id := range commandIDs {
		pluginCommandRegistry.RLock()
		registered, ok := pluginCommandRegistry.byID[id]
		pluginCommandRegistry.RUnlock()
		if !ok {
			continue
		}
		if action, ok := pluginActionForName(pluginCommandActionName(registered.command.ID)); ok {
			actions = append(actions, action)
		}
	}
	for index, item := range pluginMenuItemsSnapshot() {
		name := item.ActionName
		if name == "" {
			name = legacyPluginActionName(index)
		}
		if action, ok := pluginActionForName(name); ok {
			actions = append(actions, action)
		}
	}
	return actions
}
