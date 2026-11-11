package settings

import (
	"context"
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func TestFileGroupActionsEditIndependently(t *testing.T) {
	parent, overview := setupFileGroupColorChild(t)
	group := &parent.Records["filegroups"][0]
	group.Values["group.Associations.Enabled"] = "true"
	parent.Records[fileGroupCommands] = []f4settings.Record{
		{ID: group.ID + "/legacy", Values: map[string]string{
			"command.Group": group.ID, "command.Name": "Legacy",
			"command.Execute": "open !.!", "command.Execute.Enabled": "true",
			"command.View": "view !.!", "command.View.Enabled": "true",
			"command.Edit": "edit !.!", "command.Edit.Enabled": "false",
			"extra.Future": "keep",
		}},
	}
	original := f4settings.NewDraft(nil, parent.Records).Records[fileGroupCommands]
	child := overview.childEditor(0)
	draft := child.sessions[0].draft
	defer draft.Close()
	if !reflect.DeepEqual(draft.Records[fileGroupCommands], original) {
		t.Fatal("opening the editor rewrote the legacy record")
	}
	for _, row := range child.page.rows {
		if row.field.ID == "command.Group" {
			t.Fatal("the group editor exposed command ownership")
		}
	}

	viewID := fileGroupActionPrefix + "View"
	var add *settingsButtonRow
	for _, row := range child.page.rows {
		if bar, ok := row.control.(*settingsButtonRow); ok && bar.GetId() == "collection-actions:"+viewID {
			add = bar
			break
		}
	}
	if add == nil || len(add.buttons) == 0 || add.buttons[0].IsDisabled() {
		t.Fatal("View has no enabled Add command")
	}
	add.buttons[0].OnClick()
	viewRows := draft.Records[viewID]
	if len(viewRows) != 2 || viewRows[1].Values["command.View.Enabled"] != "true" {
		t.Fatalf("View records after Add = %+v", viewRows)
	}
	viewRows[0].Values["command.Name"] = "First viewer"
	viewRows[1].Values["command.View"] = "other !.!"
	if result := draft.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatalf("accepting associations: %v", result.Errors)
	}
	groups, err := recordsToGroups(parent.Records)
	if err != nil {
		t.Fatal(err)
	}
	commands := groups[0].Commands
	if len(commands) != 4 {
		t.Fatalf("persisted commands = %+v", commands)
	}
	if commands[0].Name != "Legacy" || commands[0].Commands[0] != "open !.!" ||
		commands[1].Name != "First viewer" || commands[1].Commands[2] != "view !.!" ||
		commands[2].Commands[2] != "other !.!" || !commands[2].Enabled[2] ||
		commands[3].Commands[4] != "edit !.!" || commands[3].Enabled[4] ||
		commands[1].Extra["Future"] != "keep" {
		t.Fatalf("independent command slots did not round-trip: %+v", commands)
	}
	if _, leaked := parent.Records[viewID]; leaked {
		t.Fatal("editor-only records escaped into the parent draft")
	}
}

func TestFileGroupAssociationButtonsFollowSelection(t *testing.T) {
	parent, overview := setupFileGroupColorChild(t)
	parent.Records["filegroups"][0].Values["group.Associations.Enabled"] = "true"
	child := overview.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(0, 0, 118, 32)

	bar := func(slot string) *settingsButtonRow {
		t.Helper()
		for _, row := range child.page.rows {
			if bar, ok := row.control.(*settingsButtonRow); ok && bar.GetId() == "collection-actions:"+fileGroupActionPrefix+slot {
				return bar
			}
		}
		t.Fatalf("button bar missing for %s", slot)
		return nil
	}
	check := func(slot string, disabled [4]bool) {
		t.Helper()
		for index, button := range bar(slot).buttons {
			if button.IsDisabled() != disabled[index] {
				t.Fatalf("%s button %d disabled = %t, want %t", slot, index, button.IsDisabled(), disabled[index])
			}
		}
	}
	check("Execute", [4]bool{false, true, true, true})
	bar("Execute").buttons[0].OnClick()
	check("Execute", [4]bool{false, false, true, true})
	bar("Execute").buttons[0].OnClick()
	check("Execute", [4]bool{false, false, false, true})
	bar("Execute").buttons[2].OnClick()
	check("Execute", [4]bool{false, false, true, false})
	check("AltExec", [4]bool{false, true, true, true})

	node := child.page.SemanticNode(&vtui.SemanticContext{})
	roles := map[string]int{}
	toolbars := 0
	for _, group := range semantic.AppMapSlice(node["children"]) {
		role := semantic.String(group["layoutRole"])
		roles[role]++
		for _, child := range semantic.AppMapSlice(group["children"]) {
			if child["layoutRole"] != "association-buttons" {
				continue
			}
			toolbars++
			buttons := semantic.AppMapSlice(child["children"])
			icons := [...]string{"plus", "trash-2", "arrow-up", "arrow-down"}
			if len(buttons) != len(icons) {
				t.Fatalf("association toolbar has %d buttons", len(buttons))
			}
			for index, button := range buttons {
				if button["iconName"] != icons[index] {
					t.Fatalf("wrong icon on button %d: %v", index, button)
				}
			}
		}
	}
	if roles["association-regular"] != 3 || roles["association-alternative"] != 3 || toolbars != 6 {
		t.Fatalf("association column roles = %v, toolbars = %d", roles, toolbars)
	}
}
