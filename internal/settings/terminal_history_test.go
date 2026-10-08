package settings

import (
	"context"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func TestTerminalHistoryInheritanceSetting(t *testing.T) {
	oldConfig, oldWriter := config.App, writeSettingsCandidate
	t.Cleanup(func() { config.App = oldConfig; writeSettingsCandidate = oldWriter })
	config.App.InheritTerminalHistory = false
	provider := coreSettingsProvider{}
	var field *f4settings.Field
	catalog := provider.Catalog()
	for i := range catalog.Fields {
		if catalog.Fields[i].ID == "InheritTerminalHistory" {
			field = &catalog.Fields[i]
			break
		}
	}
	if field == nil || field.Kind != f4settings.Boolean || field.Category != "terminal" || field.Timing != "new workspaces" {
		t.Fatalf("missing terminal history checkbox: %+v", field)
	}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(draft.Close)
	if draft.Values[field.ID] != "false" {
		t.Fatal("history checkbox does not reflect the disabled default")
	}
	writes := 0
	writeSettingsCandidate = func(before, candidate config.F4Config) error {
		writes++
		if before.InheritTerminalHistory || !candidate.InheritTerminalHistory {
			t.Fatal("Apply wrote the wrong terminal history preference")
		}
		return nil
	}
	draft.Values[field.ID] = "true"
	if config.App.InheritTerminalHistory {
		t.Fatal("draft changed terminal inheritance before Apply")
	}
	if result := draft.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	if writes != 1 || !config.App.InheritTerminalHistory {
		t.Fatal("Apply did not persist and publish terminal inheritance")
	}
	draft.Values[field.ID] = "false"
	draft.Close()
	if !config.App.InheritTerminalHistory {
		t.Fatal("Cancel lost the applied terminal history preference")
	}
}

func TestTerminalHistoryInheritanceCheckbox(t *testing.T) {
	oldConfig := config.App
	t.Cleanup(func() { config.App = oldConfig; InitLang() })
	config.App.Language = "en"
	config.App.UseLocalLanguageFiles = false
	config.App.InheritTerminalHistory = false
	InitLang()
	provider := coreSettingsProvider{}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(draft.Close)
	center := newSettingsCenter([]*settingsSession{{catalog: provider.Catalog(), draft: draft}})
	center.selectCategory("terminal")
	center.SetPosition(0, 0, 119, 39)
	var checkbox *settingsCheckbox
	for _, row := range center.page.rows {
		if row.field.ID == "InheritTerminalHistory" {
			checkbox, _ = row.control.(*settingsCheckbox)
		}
	}
	if checkbox == nil || checkbox.State != 0 {
		t.Fatal("Terminal settings do not contain an unchecked history option")
	}
	if !center.HandleSemanticAction(map[string]any{"target": vtui.SemanticID(checkbox), "action": "control.toggle"}) {
		t.Fatal("Qt checkbox action did not reach the Go settings owner")
	}
	if draft.Values["InheritTerminalHistory"] != "true" || config.App.InheritTerminalHistory {
		t.Fatal("checkbox did not update only the settings draft")
	}
	for _, node := range settingsSemanticNodes(center.SemanticNode(&vtui.SemanticContext{Width: 120, Height: 40})) {
		if node["id"] == vtui.SemanticID(checkbox) {
			if node["kind"] != "checkbox" || node["text"] != "Inherit terminal history in new tabs" || node["state"] != 1 {
				t.Fatalf("Qt history checkbox metadata is incorrect: %v", node)
			}
			return
		}
	}
	t.Fatal("history checkbox was not exported to Qt")
}
