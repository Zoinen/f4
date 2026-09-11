package nativeui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestControlProjectionPreservesItemIcons(t *testing.T) {
	want := []string{"palette", "", "keyboard"}
	for _, kind := range []string{"listBox", "table"} {
		t.Run(kind, func(t *testing.T) {
			control := appControlFromLegacy(map[string]any{"id": "categories", "kind": kind, "itemIcons": want})
			control.Legacy = nil
			projected := appControlFromLegacy(control.ToMap())
			if !reflect.DeepEqual(projected.ItemIcons, want) {
				t.Fatalf("item icons lost on typed wire roundtrip: %v", projected.ItemIcons)
			}
		})
	}
	control := appControlFromLegacy(map[string]any{"kind": "listBox"})
	if _, exists := control.ToMap()["itemIcons"]; exists {
		t.Fatal("unadorned lists should omit icon metadata")
	}
}

func TestControlProjectionPreservesScrollableDecorations(t *testing.T) {
	node := map[string]any{"id": "page", "kind": "group", "scrollable": true, "scrollTop": 7, "contentHeight": 90,
		"children": []map[string]any{{"id": "language", "kind": "group", "bordered": true, "dimmed": true, "title": "Language",
			"children": []map[string]any{{"id": "label", "kind": "text", "text": "Interface language"}}}}}
	control := appControlFromLegacy(node)
	if !control.Scrollable || control.ScrollTop != 7 || control.ContentHeight != 90 {
		t.Fatalf("viewport lost: %+v", control)
	}
	if !control.Children[0].Bordered || !control.Children[0].Dimmed || control.Children[0].Children[0].Text != "Interface language" {
		t.Fatal("decorations lost")
	}
	projected := appControlFromLegacy(control.ToMap())
	if !projected.Scrollable || !projected.Children[0].Bordered || projected.Children[0].Children[0].Text != "Interface language" {
		t.Fatal("wire roundtrip lost decorations")
	}
}

func TestSettingsInteractionProjection(t *testing.T) {
	node := map[string]any{"id": "mode", "kind": "radioGroup", "explainTarget": "mode",
		"disabledItems": []int{2}, "items": []string{"One", "Two", "Three"}, "selected": 0, "focusIndex": 1}
	control := appControlFromLegacy(node)
	// These are typed protocol fields, not accidental Legacy passthrough.
	control.Legacy = nil
	if control.ExplainTarget != "mode" || len(control.DisabledItems) != 1 || control.DisabledItems[0] != 2 {
		t.Fatalf("interaction metadata lost: %+v", control)
	}
	projected := appControlFromLegacy(control.ToMap())
	if projected.ExplainTarget != "mode" || len(projected.DisabledItems) != 1 || projected.DisabledItems[0] != 2 {
		t.Fatal("interaction metadata lost on wire roundtrip")
	}
}

func TestSettingsLayoutProjection(t *testing.T) {
	dialog := appDialogFromLegacy(map[string]any{"kind": "dialog", "layout": "settings",
		"children": []map[string]any{{"kind": "group", "layoutRole": "content"}}})
	dialog.Legacy = nil
	dialog.Controls[0].Legacy = nil
	projected := appDialogFromLegacy(dialog.ToMap())
	if projected.Layout != "settings" || projected.Controls[0].LayoutRole != "content" {
		t.Fatal("typed layout roles lost on wire roundtrip")
	}
}

func TestExternalDropdownOmitsRedundantChoices(t *testing.T) {
	for _, external := range []bool{false, true} {
		source := map[string]any{"kind": "comboBox", "dropdownOnly": external,
			"items": []string{"One", "Two"}, "text": "Two", "selected": 1}
		wire := appControlFromLegacy(source).ToMap()
		_, hasItems := wire["items"]
		if hasItems == external || wire["text"] != "Two" {
			t.Fatalf("incorrect dropdown projection: %v", wire)
		}
		if len(source["items"].([]string)) != 2 {
			t.Fatal("native projection mutated the owner model")
		}
	}
}

// Project the owner replay through the actual native transport before profiling Qt.
func TestSettingsNativeProfileFixtures(t *testing.T) {
	directory := os.Getenv("F4_SETTINGS_NATIVE_PROFILE_DIR")
	if directory == "" {
		t.Skip("set F4_SETTINGS_NATIVE_PROFILE_DIR to project replay fixtures")
	}
	files, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if filepath.Base(file) == "manifest.json" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var node map[string]any
		if err := json.Unmarshal(data, &node); err != nil {
			t.Fatal(err)
		}
		data, err = json.MarshalIndent(appDialogFromLegacy(node).ToMap(), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFillWidthTypedProjection(t *testing.T) {
	control := appControlFromLegacy(map[string]any{"kind": "edit", "fillWidth": true})
	control.Legacy = nil
	if !appControlFromLegacy(control.ToMap()).FillWidth {
		t.Fatal("fillWidth lost on typed roundtrip")
	}
}

func TestMultiLineControlProjection(t *testing.T) {
	source := map[string]any{"kind": "multiLineEdit", "text": "echo 😀\necho next", "cursor": 8, "selectionStart": 5, "selectionEnd": 8, "selectionActive": true, "fillWidth": true}
	result := appControlFromLegacy(source).ToMap()
	for key, want := range source {
		if !reflect.DeepEqual(result[key], want) {
			t.Fatalf("%s = %#v, want %#v", key, result[key], want)
		}
	}
}
