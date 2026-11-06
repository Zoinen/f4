package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/panel"
)

func TestFileGroupCommittedDispatchAllSixSlots(t *testing.T) {
	old := panel.AssociationGroups
	t.Cleanup(func() { panel.AssociationGroups = old })
	for slot := panel.AssocExecute; slot < panel.AssocKindCount; slot++ {
		t.Run(fmt.Sprint(slot), func(t *testing.T) {
			command := filegroups.CommandSet{ID: "app", Name: "Test application"}
			command.Commands[slot] = "test-command !.!"
			command.Enabled[slot] = true
			groups := []filegroups.Group{{ID: "png", Fields: map[string]string{"Mask": "*.png", "Associations.Enabled": "true"}, Commands: []filegroups.CommandSet{command}}}
			runtime, err := filegroups.NewRuntime(groups, nil)
			if err != nil {
				t.Fatal(err)
			}
			panel.AssociationGroups = runtime.Committed
			groups[0].Commands[0].Commands[slot] = "unapplied-command !.!"
			if err := runtime.Preview(groups); err != nil {
				t.Fatal(err)
			}
			pf, pty := setupPanelWithFile(t, "pic.png")
			if !panel.TryFileAssociation(pf, slot) {
				t.Fatal("command not dispatched")
			}
			written := string(pty.Written)
			if !strings.Contains(written, "test-command pic.png") || strings.Contains(written, "unapplied-command") {
				t.Fatalf("preview executed or command lost: %q", written)
			}
		})
	}
	for _, name := range []string{"File.AssociationAltExecute", "File.AssociationAltView", "File.AssociationAltEdit"} {
		action, ok := GetAction(name)
		if !ok || len(action.DefaultKeys) != 0 {
			t.Fatalf("alternate action missing or took a shortcut: %+v", action)
		}
	}
}
