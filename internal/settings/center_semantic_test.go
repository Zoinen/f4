package settings

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"

	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtinput"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

func settingsSemanticNodes(node map[string]any) []map[string]any {
	nodes := []map[string]any{node}
	children, _ := node["children"].([]map[string]any)
	for _, child := range children {
		nodes = append(nodes, settingsSemanticNodes(child)...)
	}
	return nodes
}

func TestSettingsCategoryIconsAndSelection(t *testing.T) {
	provider := coreSettingsProvider{}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(draft.Close)
	catalog := provider.Catalog()
	// Reordering and a contributed category must preserve row/icon association.
	catalog.Categories = append([]f4settings.Category{{
		ID: "contributed", Label: f4settings.Text{English: "Extra & settings"},
	}}, catalog.Categories...)
	center := newSettingsCenter([]*settingsSession{{catalog: catalog, draft: draft}})
	center.SetPosition(0, 0, 99, 59)
	ctx := &vtui.SemanticContext{Width: 100, Height: 60}
	seen := map[string]bool{}
	for _, query := range []string{"", "font"} {
		center.query = query
		for _, node := range settingsSemanticNodes(center.SemanticNode(ctx)) {
			if node["id"] != vtui.SemanticID(center.sidebar) {
				continue
			}
			icons, _ := node["itemIcons"].([]string)
			rows, _ := node["rows"].([]map[string]any)
			if len(icons) != len(catalog.Categories) || len(rows) != len(icons) {
				t.Fatalf("category rows/icons mismatch: %d rows, %d icons", len(rows), len(icons))
			}
			for i, category := range catalog.Categories {
				if icons[i] == "" || (category.ID != "contributed" && icons[i] == "file-cog") {
					t.Errorf("category %s has no dedicated icon", category.ID)
				}
				if icons[i] != settingsCategoryIcon(category.ID) {
					t.Errorf("category %s has the wrong row icon: %s", category.ID, icons[i])
				}
				want := (settingsCategoryRow{center: center, category: category}).GetCellText(0)
				if cells := rows[i]["cells"].([]string); len(cells) != 1 || cells[0] != want {
					t.Errorf("category caption changed: %v, want %s", cells, want)
				}
				seen[category.ID] = true
			}
			if icons[0] != "file-cog" {
				t.Errorf("contributed category fallback = %s", icons[0])
			}
		}
	}
	if len(seen) != len(catalog.Categories) {
		t.Fatal("missing category sidebar")
	}
	for _, index := range []int{3, 2, 0} {
		action := map[string]any{"target": vtui.SemanticID(center.sidebar), "action": "control.select", "index": index}
		if !center.HandleSemanticAction(action) || center.category != catalog.Categories[index].ID {
			t.Fatalf("selection %d did not switch the category", index)
		}
		if center.GetFocusedItem() != center.sidebar {
			t.Fatal("category selection did not keep keyboard focus in the sidebar")
		}
	}
}

func TestSettingsSemanticCaptionsAndGroups(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old; InitLang() })
	config.App.Language = "en"
	config.App.UseLocalLanguageFiles = false
	InitLang()
	provider := coreSettingsProvider{}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(draft.Close)
	center := newSettingsCenter([]*settingsSession{{catalog: provider.Catalog(), draft: draft}})
	center.SetPosition(0, 0, 99, 59)
	ctx := &vtui.SemanticContext{Width: 100, Height: 60}
	nodes := settingsSemanticNodes(center.SemanticNode(ctx))
	if path := os.Getenv("F4_SETTINGS_SCENE"); path != "" {
		center.SetPosition(0, 0, 99, 25)
		data, err := json.MarshalIndent(center.SemanticNode(ctx), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, caption := range []string{"Search:", "Appearance & language", "Interface language", "Help language", "Color theme", "Graphical font", "Font size", "Window title template", "Select a setting to read what it does."} {
		t.Run(caption, func(t *testing.T) {
			for _, node := range nodes {
				if node["kind"] == "text" && node["text"] == caption {
					return
				}
			}
			t.Errorf("missing semantic caption %q", caption)
		})
	}
	for _, title := range []string{"Language", "Colors", "Font", "Titles and menus"} {
		t.Run("group/"+title, func(t *testing.T) {
			for _, node := range nodes {
				if node["kind"] == "group" && node["title"] == title && node["bordered"] == true {
					if len(settingsSemanticNodes(node)) < 2 {
						t.Error("group has no semantic content")
					}
					return
				}
			}
			t.Errorf("missing semantic group %q", title)
		})
	}
}

func TestSettingsSemanticPageScrollAndFocus(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old; InitLang() })
	config.App.Language = "en"
	config.App.UseLocalLanguageFiles = false
	InitLang()
	provider := coreSettingsProvider{}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(draft.Close)
	center := newSettingsCenter([]*settingsSession{{catalog: provider.Catalog(), draft: draft}})
	center.SetPosition(0, 0, 99, 25)
	ctx := &vtui.SemanticContext{Width: 100, Height: 30}
	page := center.page.SemanticNode(ctx)
	if page["id"] != "id:settings-page" || page["scrollable"] != true {
		t.Fatalf("missing stable viewport: id=%v scrollable=%v", page["id"], page["scrollable"])
	}
	before := settingsSemanticNodes(page)
	action := map[string]any{"target": "id:settings-page", "action": "control.scroll", "value": 8}
	if !center.HandleSemanticAction(action) || center.page.scroll != 8 {
		t.Fatal("semantic scroll did not reach the page")
	}
	after := settingsSemanticNodes(center.page.SemanticNode(ctx))
	if len(before) != len(after) {
		t.Fatal("scroll changed the exported content")
	}
	for i := range before {
		if before[i]["id"] != after[i]["id"] || before[i]["y"] != after[i]["y"] {
			t.Fatalf("scroll changed unscrolled coordinates/identity for %v", before[i]["id"])
		}
	}
	for _, row := range center.page.rows {
		button, ok := row.control.(*settingsCheckbox)
		if !ok {
			continue
		}
		oldState := button.State
		action = map[string]any{"target": vtui.SemanticID(button), "action": "control.toggle"}
		if !center.HandleSemanticAction(action) || button.State == oldState {
			t.Fatal("decorative nesting broke checkbox actions")
		}
		if center.GetFocusedItem() != center.page || center.page.GetFocusedItem() != button {
			t.Fatal("semantic click lost Go focus")
		}
		x1, y1, x2, y2 := button.GetPosition()
		center.page.SemanticNode(ctx)
		ax1, ay1, ax2, ay2 := button.GetPosition()
		// notifyFocus may scroll. Exporting must not then alter live positions.
		if [4]int{ax1, ay1, ax2, ay2} != [4]int{x1, y1, x2, y2} {
			t.Fatal("export changed live control geometry")
		}
		if center.help.text == settingsText("Select", "Select a setting to read what it does.") {
			t.Fatal("click did not update help")
		}
		break
	}
	center.query = "no such setting"
	center.updateMatches()
	for _, node := range settingsSemanticNodes(center.page.SemanticNode(ctx)) {
		if node["bordered"] == true && node["dimmed"] != true {
			t.Fatal("unmatched group is not dimmed")
		}
	}
	center.selectCategory(center.categories[1].ID)
	page = center.page.SemanticNode(ctx)
	if page["id"] != "id:settings-page" {
		t.Fatal("category switch lost viewport identity")
	}
	if semantic.Int(page["contentHeight"]) != center.page.total {
		t.Fatal("stale content extent")
	}
}

func TestSettingsSemanticRadiosAndExplain(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old; InitLang() })
	config.App.Language = "en"
	config.App.UseLocalLanguageFiles = false
	InitLang()
	mode := f4settings.Scalar("mode", "startup", "Test", "Operation mode", "Choose a mode.", f4settings.ChoiceKind)
	mode.Choices = []f4settings.Choice{
		{Value: "a", Label: f4settings.Text{English: "First full option caption"}, Description: f4settings.Text{English: "First explanation"}},
		{Value: "b", Label: f4settings.Text{English: "Second full option caption"}, Description: f4settings.Text{English: "Second explanation"}},
	}
	input := f4settings.Scalar("input", "startup", "Test", "Input caption", "Input explanation", f4settings.String)
	input.Unavailable = "Not supported here"
	draft := f4settings.NewDraft(map[string]string{"mode": "a", "input": "saved"}, nil)
	t.Cleanup(draft.Close)
	c := newSettingsCenter([]*settingsSession{{catalog: f4settings.Catalog{
		ID: "semantic-test", Categories: Categories, Fields: []f4settings.Field{mode, input},
	}, draft: draft}})
	c.selectCategory("startup")
	c.SetPosition(0, 0, 79, 24)
	var radios *settingsRadios
	var inputRow *settingsRow
	for _, row := range c.page.rows {
		if row.field.ID == "mode" {
			radios = row.control.(*settingsRadios)
		}
		if row.field.ID == "input" {
			inputRow = row
		}
	}
	if radios == nil || inputRow == nil {
		t.Fatal("missing fixture controls")
	}
	ctx := &vtui.SemanticContext{Width: 80, Height: 25}
	for _, node := range settingsSemanticNodes(c.SemanticNode(ctx)) {
		if node["id"] == vtui.SemanticID(radios) {
			if node["title"] != mode.Label.English || node["fillWidth"] != true {
				t.Fatalf("adaptive radio metadata lost: %v", node)
			}
		}
		if node["id"] == vtui.SemanticID(radios)+"-label" {
			t.Fatal("duplicate adaptive radio caption")
		}
		if node["id"] == vtui.SemanticID(inputRow.control) && node["fillWidth"] != true {
			t.Fatal("full-width edit missing size policy")
		}
	}
	t.Run("radio source and focus", func(t *testing.T) {
		node := radios.SemanticNode(ctx)
		if node["kind"] != "radioGroup" {
			t.Fatalf("radio kind = %v, want radioGroup", node["kind"])
		}
		if !reflect.DeepEqual(node["items"], []string{mode.Choices[0].Label.English, mode.Choices[1].Label.English}) {
			t.Fatalf("radio captions lost or terminal-wrapped: %v", node["items"])
		}
		c.SetFocusedItem(c.page)
		c.page.SetFocusedItem(radios)
		radios.SetFocusedItem(radios.buttons[0])
		c.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
		node = radios.SemanticNode(ctx)
		if node["focusIndex"] != 1 || node["selected"] != 0 || draft.Values["mode"] != "a" {
			t.Fatalf("focus and selection not independent: %v", node)
		}
		if !c.HandleSemanticAction(map[string]any{"target": vtui.SemanticID(radios), "action": "control.select", "index": 1}) {
			t.Fatal("radio selection was not handled")
		}
		if draft.Values["mode"] != "b" || !radios.buttons[1].Selected || radios.buttons[0].Selected {
			t.Fatal("radio click did not select exactly one option")
		}
		if c.GetFocusedItem() != c.page || c.page.GetFocusedItem() != radios || radios.GetFocusedItem() != radios.buttons[1] {
			t.Fatal("radio click lost focus chain")
		}
		radios.buttons[0].SetDisabled(true)
		if !reflect.DeepEqual(radios.SemanticNode(ctx)["disabledItems"], []int{0}) {
			t.Fatal("disabled choice not exported")
		}
		for _, index := range []any{nil, -1, 0, 5} {
			action := map[string]any{"target": vtui.SemanticID(radios), "action": "control.select"}
			if index != nil {
				action["index"] = index
			}
			if radios.HandleSemanticAction(action) || draft.Values["mode"] != "b" {
				t.Fatalf("accepted unavailable/invalid choice %v", index)
			}
		}
		radios.SetDisabled(true)
		if radios.SemanticNode(ctx)["disabled"] != true || radios.HandleSemanticAction(map[string]any{"target": vtui.SemanticID(radios), "action": "control.select", "index": 1}) {
			t.Fatal("disabled radio group accepted selection")
		}
		radios.SetDisabled(false)
	})
	t.Run("hover changes only help", func(t *testing.T) {
		c.SetFocusedItem(c.sidebar)
		beforeValue := draft.Values["mode"]
		pageFocus, radioFocus, scroll := c.page.GetFocusedItem(), radios.GetFocusedItem(), c.page.scroll
		for _, tc := range []struct {
			target, want string
			index        any
		}{
			{vtui.SemanticID(inputRow.control), "Input explanation", nil},
			{vtui.SemanticID(radios), "First explanation", 0},
			{vtui.SemanticID(radios), "Second explanation", 1},
		} {
			action := map[string]any{"target": tc.target, "action": "control.explain"}
			if tc.index != nil {
				action["index"] = tc.index
			}
			if !c.HandleSemanticAction(action) || !strings.Contains(c.help.text, tc.want) {
				t.Fatalf("hover did not explain %s: %s", tc.target, c.help.text)
			}
			if c.GetFocusedItem() != c.sidebar || c.page.GetFocusedItem() != pageFocus || radios.GetFocusedItem() != radioFocus || c.page.scroll != scroll || draft.Values["mode"] != beforeValue {
				t.Fatal("hover changed focus, scrolling or draft")
			}
			if tc.target == vtui.SemanticID(inputRow.control) && !strings.Contains(c.help.text, input.Unavailable) {
				t.Fatal("disabled explanation lost reason")
			}
		}
		for _, node := range settingsSemanticNodes(c.SemanticNode(ctx)) {
			id, _ := node["id"].(string)
			if id == vtui.SemanticID(inputRow.control) || id == vtui.SemanticID(inputRow.control)+"-label" {
				if node["explainTarget"] != vtui.SemanticID(inputRow.control) {
					t.Fatalf("caption/control lacks help target: %v", node)
				}
			}
		}
	})
}

func TestSettingsExportsDockingRoles(t *testing.T) {
	provider := coreSettingsProvider{}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(draft.Close)
	center := newSettingsCenter([]*settingsSession{{catalog: provider.Catalog(), draft: draft}})
	center.SetPosition(0, 0, 99, 59)
	for _, query := range []string{"", "font"} {
		center.query = query
		node := center.SemanticNode(&vtui.SemanticContext{Width: 100, Height: 60})
		if node["layout"] != "settings" {
			t.Fatal("missing native layout contract")
		}
		roles := map[string]bool{}
		for _, child := range node["children"].([]map[string]any) {
			role := semantic.String(child["layoutRole"])
			if role == "" || roles[role] {
				t.Fatalf("missing or duplicate role %q", role)
			}
			roles[role] = true
		}
		for _, role := range []string{"search", "search-label", "navigation", "content-title", "content", "description", "apply", "accept", "cancel"} {
			if !roles[role] {
				t.Fatalf("missing %s", role)
			}
		}
		if roles["search-matches"] != (query != "") {
			t.Fatal("search matches role differs from visibility")
		}
	}
}
