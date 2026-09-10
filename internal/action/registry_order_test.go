package action

import (
	"strings"
	"testing"
)

// The order actions are presented in is what the user reads in the menu, so it
// is behaviour and not an implementation detail. The golden list of that order
// lives with the table that produces it, in the composition root; what is
// checked here is the mechanism underneath it.

func TestActionOrderCoversRegistry(t *testing.T) {
	if len(actionOrder) != len(actionRegistry) {
		t.Fatalf("actionOrder holds %d keys, actionRegistry %d", len(actionOrder), len(actionRegistry))
	}
	seen := make(map[string]int, len(actionOrder))
	for _, key := range actionOrder {
		seen[key]++
		if _, registered := actionRegistry[key]; !registered {
			t.Errorf("actionOrder names %q, which is not registered", key)
		}
	}
	for key, count := range seen {
		if count != 1 {
			t.Errorf("actionOrder names %q %d times, want once", key, count)
		}
	}
	for key := range actionRegistry {
		if seen[key] == 0 {
			t.Errorf("registered action %q is missing from actionOrder", key)
		}
	}
}

// TestActionOrderIndependentOfRegistrationSequence is the property the split
// depends on: once the registration calls live in different packages, Go
// decides their sequence from the import graph, and the answer must not change.
func TestActionOrderIndependentOfRegistrationSequence(t *testing.T) {
	t.Cleanup(Snapshot())

	forward := []Action{
		{Name: "Workspace.New", Area: "Shell"},
		{Name: "App.Help", Area: "Common"},
		{Name: "App.ScreenGrab", Area: "Common"},
	}
	reversed := []Action{forward[2], forward[1], forward[0]}

	registerOnly := func(actions []Action) []string {
		actionRegistry = make(map[string]Action, len(actions))
		actionOrder = nil
		for _, a := range actions {
			RegisterAction(a)
		}
		var names []string
		for _, a := range All() {
			names = append(names, a.Name)
		}
		return names
	}

	first := registerOnly(forward)
	second := registerOnly(reversed)
	if strings.Join(first, ",") != strings.Join(second, ",") {
		t.Fatalf("registration sequence decides presentation order: %v vs %v", first, second)
	}
	if want := "App.ScreenGrab,App.Help,Workspace.New"; strings.Join(first, ",") != want {
		t.Fatalf("presentation order = %v, want %s (actionMenuOrder's order)", first, want)
	}
}

// TestDisplayLabelFallsBackToEnglish covers the shape of Localize's default.
// DisplayLabel keeps a localized label only when the lookup did not answer with
// its missing-key form, so a default that returned the key unchanged would
// render raw keys as menu labels.
func TestDisplayLabelFallsBackToEnglish(t *testing.T) {
	a := Action{Label: "Screen grab", LabelKey: "Action.App.ScreenGrab",
		Description: "Capture the screen", DescKey: "Action.App.ScreenGrab.Desc"}
	if got := a.DisplayLabel(); got != "Screen grab" {
		t.Errorf("DisplayLabel with no localizer = %q, want the English label", got)
	}
	if got := a.DisplayDescription(); got != "Capture the screen" {
		t.Errorf("DisplayDescription with no localizer = %q, want the English text", got)
	}

	old := Localize
	t.Cleanup(func() { Localize = old })
	Localize = func(key string) string { return "translated:" + key }
	if got := a.DisplayLabel(); got != "translated:Action.App.ScreenGrab" {
		t.Errorf("DisplayLabel with a localizer = %q", got)
	}
}
