// Package action is the registry of everything the user can trigger: a macro
// command, a menu entry, a help line and a default hotkey are all one Action.
//
// It is a leaf on purpose. The table that fills it reaches into every view in
// the application and stays in the composition root; what lives here is the
// mechanism, which knows nothing but names and func() bool.
package action

import (
	"sort"
	"strings"
)

// Action represents a bindable command in the application.
//
// An action is the single source of truth for everything the user can
// trigger interactively: it is a macro command (by Name), a menu/keybar
// entry (Label/MenuPath), a help topic line (Description) and a default
// hotkey (DefaultKeys) at the same time.
type Action struct {
	Name     string // stable ID and macro command, e.g. "Editor.Save"
	Area     string // primary area: "Shell", "Editor", "Viewer", "Terminal", "Common"
	Label    string // English fallback for menu/keybar
	LabelKey string // optional i18n key resolved via i18n.Msg()
	// SearchKeys are additional localization keys whose values are indexed by
	// discoverability surfaces but are not rendered as the action label.
	SearchKeys  []string
	Description string // English fallback for help
	DescKey     string // optional i18n key resolved via i18n.Msg()
	// DefaultKeys are Far-style key names ("F2", "CtrlIns") with an
	// optional ":Condition" suffix per key (e.g. "Esc:EscToggle").
	DefaultKeys []string
	// NativeKeys documents shortcuts owned by vtui's frame dispatcher rather
	// than HotkeyManager. Like DefaultKeys, they may carry a :Condition suffix.
	// They are shown in discoverability surfaces, but are
	// deliberately not installed as configurable defaults: claiming them in
	// EventFilter would run the action before the focused frame gets its normal
	// chance to consume the key (notably terminal Ctrl+W and Ctrl+N).
	NativeKeys []string
	// DefaultAreas lists extra areas (besides Area) that get the default
	// bindings too (e.g. Panel.Toggle works in both Shell and Terminal).
	DefaultAreas []string
	// MenuPath is the top-level menu the action appears in ("File",
	// "Edit", ...). Empty means the action is not listed in menus.
	MenuPath string
	// MenuSubPath nests the action one level deeper: every action sharing a
	// MenuPath and a MenuSubPath is collected under a single item of the
	// top-level menu, which opens them as a submenu. The submenu heading is
	// titled from "Menu.<area>.<MenuPath>.<MenuSubPath>", falling back to
	// the raw MenuSubPath. It is for the rarely reached commands: a menu
	// long enough to scroll hides its own contents, and folding a
	// recognizable group away buys room for the rest.
	MenuSubPath string
	// HideFromMenu keeps an action out of registry-generated menus while still
	// retaining MenuPath as its localized command-palette category. This is for
	// commands exposed by a custom menu, such as the fixed Left/Right panel
	// actions, whose exact command IDs must remain discoverable without creating
	// a second generated copy of the custom menu.
	HideFromMenu bool
	// MenuSeparatorBefore inserts a separator above this action's menu item.
	// On the first action of a MenuSubPath it goes above the submenu
	// heading, since that is where the item lands; on the later ones it
	// divides the submenu itself.
	MenuSeparatorBefore bool
	// MenuLast pins this action's menu item to the very end of its MenuPath
	// group, after every other item (including Common-area ones), regardless
	// of registration order. Combine with MenuSeparatorBefore to set it off
	// from the rest of the menu.
	MenuLast bool
	// Checked, when set, reports the toggle state shown in menus ("√ ").
	Checked func() bool
	// Visible, when set, decides whether the action appears in a menu at
	// all. It is asked every time the menu is built, so it can depend on
	// what the panel is showing right now: an action the current file
	// system cannot perform is better left out than offered and refused.
	// It does not affect key bindings, which reach the handler regardless.
	//
	// It must not test the top frame. Menus are rebuilt on every
	// GetMenuBar call, and by the time the user is reading a dropdown that
	// dropdown is itself the top frame, so a predicate asking for panels
	// (or an editor, or a viewer) on top answers "no" precisely while the
	// menu it belongs to is on screen, and the item deletes itself from
	// the list being drawn. Walk the frame stack instead:
	// GetActiveFrames(FrameManager.ActiveIdx) holds the workspace below
	// the popup. A top-frame test is only safe on a HideFromMenu action,
	// where Visible is consulted by the command palette alone.
	Visible func() bool
	Handler func() bool
}

// DisplayLabel returns the localized label, falling back to the English one.
func (a Action) DisplayLabel() string {
	if a.LabelKey != "" {
		if s := Localize(a.LabelKey); !strings.HasPrefix(s, "{") {
			return s
		}
	}
	return a.Label
}

// DisplayDescription returns the localized description, falling back to English.
func (a Action) DisplayDescription() string {
	if a.DescKey != "" {
		if s := Localize(a.DescKey); !strings.HasPrefix(s, "{") {
			return s
		}
	}
	return a.Description
}

// Localize resolves a message key to display text. The composition root sets
// it to the message catalogue's lookup; the default reproduces that lookup's
// missing-key form, because DisplayLabel tests for exactly that shape and an
// identity default would silently render raw keys as labels.
var Localize = func(key string) string { return "{" + key + "}" }

var actionRegistry = make(map[string]Action)

// actionOrder keeps registration order. It decides presentation order only for
// actions actionMenuOrder does not name — plugins, and anything registered at
// runtime.
var actionOrder []string

// RegisterAction adds an action to the global registry.
func RegisterAction(action Action) {
	key := strings.ToLower(action.Name)
	if _, exists := actionRegistry[key]; !exists {
		actionOrder = append(actionOrder, key)
	}
	actionRegistry[key] = action
}

// GetActions returns a list of all registered actions, sorted by name.
// AllSorted returns every registered action ordered by name.
func AllSorted() []Action {
	var actions []Action
	for _, a := range actionRegistry {
		actions = append(actions, a)
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Name < actions[j].Name
	})
	return actions
}

// GetOrderedActions returns all registered actions in presentation order:
// actionMenuOrder's order for the actions it names, then everything else in
// registration order. This is what the user sees in the menu, so it must not
// depend on which file or which package a RegisterAction call happens to sit
// in — see actionMenuOrder.
// All returns every registered action in presentation order.
func All() []Action {
	keys := append([]string(nil), actionOrder...)
	sort.SliceStable(keys, func(i, j int) bool {
		return actionMenuRank(keys[i]) < actionMenuRank(keys[j])
	})
	actions := make([]Action, 0, len(keys))
	for _, key := range keys {
		actions = append(actions, actionRegistry[key])
	}
	return actions
}

// PlainLabel strips hotkey markers ('&') from a menu label for contexts that
// cannot render them (keybar, plain lists). '&&' unescapes to '&'.
func PlainLabel(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '&' {
			if i+1 < len(s) && s[i+1] == '&' {
				b.WriteByte('&')
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Lookup returns a registered action by name, matched case-insensitively.
// A name the registry does not know is not an error here: the caller decides
// whether to ask a plugin next.
func Lookup(name string) (Action, bool) {
	a, ok := actionRegistry[strings.ToLower(name)]
	return a, ok
}

// Len reports how many actions are registered.
func Len() int { return len(actionRegistry) }

// Snapshot copies the registry and returns a function that puts it back. It is
// the seam a test uses to register synthetic actions without leaking them into
// the next test, and it lives here because the maps are unexported.
func Snapshot() (restore func()) {
	oldRegistry := actionRegistry
	oldOrder := actionOrder
	actionRegistry = make(map[string]Action, len(oldRegistry))
	for key, a := range oldRegistry {
		actionRegistry[key] = a
	}
	actionOrder = append([]string(nil), oldOrder...)
	return func() {
		actionRegistry = oldRegistry
		actionOrder = oldOrder
	}
}
