package settings

import (
	"testing"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func TestFileGroupIconChoicesPreserveAutomaticAndExplicitOverrides(t *testing.T) {
	parent, overview := setupFileGroupColorChild(t)
	parent.Records["filegroups"][0].Values["group.Icons.Enabled"] = "true"
	previous := fileGroupContribution
	t.Cleanup(func() { fileGroupContribution = previous })
	for _, catalog := range []string{"lucide", "system", "zoin"} {
		fileGroupContribution.Themes = append(fileGroupContribution.Themes, filegroups.IconTheme{
			ID: catalog, Assets: []string{"text-files", "images"},
		})
		parent.Records["filegroups.themes"] = append(parent.Records["filegroups.themes"], f4settings.Record{
			ID: catalog, Values: map[string]string{
				"IconCatalog": catalog, "GroupIcons": "true", "CatalogAssets": "text-files,images",
			},
		})
	}
	child := overview.childEditor(0)
	for _, catalog := range []string{"lucide", "system", "zoin"} {
		key := "group.Icon." + catalog
		var row *settingsRow
		for _, candidate := range child.page.rows {
			if candidate.field.ID == key {
				row = candidate
				break
			}
		}
		if row == nil {
			t.Fatalf("%s field missing", key)
		}
		combo := row.control.(*vtui.ComboBox)
		if got := combo.Edit.GetText(); got != "Automatic" {
			t.Fatalf("%s label = %q", key, got)
		}
		for index, reference := range []string{"asset:text-files", "none", "asset:text-files", "asset:images"} {
			details := combo.Menu.Items[index].Details
			if details["iconTheme"] != catalog || details["iconReference"] != reference {
				t.Fatalf("%s choice %d preview = %v", key, index, details)
			}
		}
		for _, selection := range []struct {
			index int
			value string
		}{{3, "asset:images"}, {1, "none"}, {0, ""}} {
			if !combo.HandleSemanticAction(map[string]any{"action": "control.select", "index": selection.index}) {
				t.Fatalf("%s rejected choice %d", key, selection.index)
			}
			if got := row.read(); got != selection.value {
				t.Fatalf("%s read = %q, want %q", key, got, selection.value)
			}
			_, explicit := row.session.draft.Values[key]
			if explicit != (selection.index != 0) {
				t.Fatalf("%s explicit override = %t after choice %d", key, explicit, selection.index)
			}
		}
	}
}
