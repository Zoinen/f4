package app

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
	"sort"
	"strings"
	"testing"
)

func TestCommandPaletteResolvesEveryActionGeneratedMenuLeafByID(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	areas := make(map[string]bool)
	for _, act := range action.All() {
		if act.MenuPath != "" && !act.HideFromMenu && !strings.EqualFold(act.Area, "Common") {
			areas[act.Area] = true
		}
	}
	orderedAreas := make([]string, 0, len(areas))
	for area := range areas {
		orderedAreas = append(orderedAreas, area)
	}
	sort.Strings(orderedAreas)

	for _, area := range orderedAreas {
		area := area
		t.Run(area, func(t *testing.T) {
			expected := commandPaletteAuditedActionMenuGroups(area)
			actual := BuildMenuBarItems(area)
			if len(actual) != len(expected) {
				t.Fatalf("top-level act menus = %d, audited act groups = %d", len(actual), len(expected))
			}

			paletteByID := make(map[string]commandPaletteEntry)
			for _, entry := range commandPaletteActionEntries(area) {
				if entry.source == commandPaletteSourceAction {
					paletteByID[strings.ToLower(entry.ID)] = entry
				}
			}

			for groupIndex, group := range expected {
				var leaves []vtui.MenuItem
				// A submenu heading is not a leaf of its own: it stands for
				// the actions folded under it, which the palette must still
				// resolve one by one.
				var collect func(items []vtui.MenuItem)
				collect = func(items []vtui.MenuItem) {
					for _, item := range items {
						if item.Separator {
							continue
						}
						if len(item.SubItems) > 0 {
							collect(item.SubItems)
							continue
						}
						leaves = append(leaves, item)
					}
				}
				collect(actual[groupIndex].SubItems)
				if len(leaves) != len(group.actions) {
					t.Fatalf("menu group %q has %d non-separator leaves, want %d act leaves", group.path, len(leaves), len(group.actions))
				}
				for index, act := range group.actions {
					if leaves[index].OnClick == nil {
						t.Errorf("menu group %q leaf %d for act %q has no executor", group.path, index, act.Name)
					}
					gotLabel := action.PlainLabel(strings.TrimPrefix(leaves[index].Text, "√ "))
					wantLabel := action.PlainLabel(act.DisplayLabel())
					if gotLabel != wantLabel {
						t.Errorf("menu group %q leaf %d = %q, want act %q label %q", group.path, index, gotLabel, act.Name, wantLabel)
					}
					entry, ok := paletteByID[strings.ToLower(act.Name)]
					if !ok {
						t.Errorf("menu act %q has no command-palette entry in area %q", act.Name, area)
						continue
					}
					wantKey := "act:" + strings.ToLower(act.Name)
					if entry.ID != act.Name || entry.Key != wantKey {
						t.Errorf("menu act %q resolves to palette ID/key %q/%q, want %q/%q", act.Name, entry.ID, entry.Key, act.Name, wantKey)
					}
				}
			}
		})
	}
}

type commandPaletteActionMenuGroup struct {
	path    string
	actions []action.Action
	pinned  []action.Action
}

// commandPaletteAuditedActionMenuGroups mirrors BuildMenuBarItems' grouping
// and ordering rules, including MenuLast pinning, so the coverage test can
// compare it against the actual generated menu leaf-by-leaf.
func commandPaletteAuditedActionMenuGroups(area string) []commandPaletteActionMenuGroup {
	var groups []commandPaletteActionMenuGroup
	byPath := make(map[string]int)
	appendAction := func(action action.Action) {
		if action.Visible != nil && !action.Visible() {
			return
		}
		index, ok := byPath[action.MenuPath]
		if !ok {
			index = len(groups)
			byPath[action.MenuPath] = index
			groups = append(groups, commandPaletteActionMenuGroup{path: action.MenuPath})
		}
		if action.MenuLast {
			groups[index].pinned = append(groups[index].pinned, action)
			return
		}
		groups[index].actions = append(groups[index].actions, action)
	}

	for _, action := range action.All() {
		if action.Name != "Settings.Open" && action.MenuPath != "" && !action.HideFromMenu && action.Area == area {
			appendAction(action)
		}
	}
	if a, ok := GetAction("Settings.Open"); ok {
		appendAction(a)
	}
	for _, action := range action.All() {
		if action.Name == "Settings.Open" || action.MenuPath == "" || action.HideFromMenu || !strings.EqualFold(action.Area, "Common") {
			continue
		}
		if _, exists := byPath[action.MenuPath]; exists {
			appendAction(action)
		}
	}
	for i := range groups {
		groups[i].actions = append(groups[i].actions, groups[i].pinned...)
		for j, a := range groups[i].actions {
			if a.Name == "Settings.Open" {
				copy(groups[i].actions[1:j+1], groups[i].actions[:j])
				groups[i].actions[0] = a
				break
			}
		}

	}
	return groups
}
