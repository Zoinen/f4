package settings

import (
	"context"
	"strings"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestFileGroupAttributeDropdownEditsDraft(t *testing.T) {
	setupFileGroupSettings(t)
	draft, err := (fileGroupsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	record := &draft.Records["filegroups"][0]
	record.Values["group.IncludeAttributes"] = "h"
	record.Values["group.ExcludeAttributes"] = "dir"
	session := &settingsSession{catalog: (fileGroupsProvider{}).Catalog(), draft: draft}
	center := newSettingsCenter([]*settingsSession{session})
	center.selectCategory("filegroups")
	child := center.fileGroupOverview.childEditor(0)
	childDraft := child.sessions[0].draft
	defer childDraft.Close()

	for _, tc := range []struct {
		key   string
		saved string
		add   string
		want  string
	}{
		{key: "group.IncludeAttributes", saved: "Hidden", add: "System", want: "Hidden, System"},
		{key: "group.ExcludeAttributes", saved: "Directory", add: "Archive", want: "Directory, Archive"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			var combo *vtui.ComboBox
			for _, row := range child.page.rows {
				if row.field.ID == tc.key {
					combo, _ = row.control.(*vtui.ComboBox)
					break
				}
			}
			if combo == nil || !combo.DropdownOnly {
				t.Fatal("expected a dropdown-only control")
			}
			savedIndex, addIndex := -1, -1
			for i, item := range combo.Menu.Items {
				switch item.Text {
				case "☑ " + tc.saved:
					savedIndex = i
				case "☐ " + tc.add:
					addIndex = i
				}
			}
			if savedIndex < 0 || addIndex < 0 {
				t.Fatalf("missing selected alias or unchecked choice: %v", combo.Menu.Items)
			}
			if !combo.Menu.HandleSemanticAction(map[string]any{
				"target": vtui.SemanticID(combo.Menu), "action": "menu.activate", "index": addIndex,
			}) || combo.Menu.IsDone() {
				t.Fatal("Qt menu activation must toggle without closing")
			}
			if got := childDraft.Values[tc.key]; got != tc.want || combo.Edit.GetText() != got {
				t.Fatalf("draft = %q, summary = %q, want %q", got, combo.Edit.GetText(), tc.want)
			}
			combo.Menu.SetSelectPos(savedIndex)
			combo.Menu.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE, Char: ' ',
			})
			if got := childDraft.Values[tc.key]; got != tc.add || combo.Menu.IsDone() {
				t.Fatalf("Space should uncheck the saved choice and keep the menu open, draft = %q", got)
			}
			if !strings.HasPrefix(combo.Menu.Items[savedIndex].Text, "☐ ") {
				t.Fatal("unchecked item still looks checked")
			}
		})
	}
	if record.Values["group.IncludeAttributes"] != "h" || record.Values["group.ExcludeAttributes"] != "dir" {
		t.Fatal("edits escaped the child draft")
	}
}
