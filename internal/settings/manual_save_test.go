package settings

import (
	"testing"
)

func TestManualSaveCommands(t *testing.T) {
	found := map[string]bool{}
	for _, command := range (settingsOperationsProvider{}).Catalog().Commands {
		if command.Category == "workspaces" && command.Group == "Manual saving" {
			found[command.ID] = true
		}
	}
	for _, id := range []string{"save.preferences", "save.session", "save.geometry"} {
		if !found[id] {
			t.Fatalf("missing manual saving command %s", id)
		}
	}
}
