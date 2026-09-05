package main

import (
	"sort"
	"strings"
	"testing"
)

// TestDefaultKeysAreUniquePerArea guards the registry against two actions in
// the same area claiming the same default shortcut.
//
// Only one of them can win the binding, and the loser silently ends up with no
// shortcut at all. That is how App.Spreadsheet took Shift+F11 from
// Settings.PluginConfiguration: nothing complained at registration, and the
// first sign of it was a menu test noticing an empty shortcut two areas away
// from the action that had actually changed.
//
// Areas are namespaces, so the same key in Shell and in Editor is not a
// conflict and this only compares within one.
func TestDefaultKeysAreUniquePerArea(t *testing.T) {
	type owner struct{ area, key string }
	owners := map[owner][]string{}

	for _, action := range GetOrderedActions() {
		// DefaultAreas receive the same bindings as Area, and a key may
		// carry a ":Condition" suffix that does not change which physical
		// key is claimed, so both are resolved before comparing.
		for _, area := range append([]string{action.Area}, action.DefaultAreas...) {
			if area == "" {
				continue
			}
			for _, spec := range action.DefaultKeys {
				key, _, _ := strings.Cut(spec, ":")
				if key == "" {
					continue
				}
				id := owner{area: area, key: key}
				owners[id] = append(owners[id], action.Name)
			}
		}
	}

	var conflicts []string
	for id, names := range owners {
		if len(names) < 2 {
			continue
		}
		sort.Strings(names)
		conflicts = append(conflicts, id.area+" "+id.key+": "+strings.Join(names, ", "))
	}
	sort.Strings(conflicts)

	for _, c := range conflicts {
		t.Errorf("default shortcut claimed more than once in the same area: %s", c)
	}
}
