package nativeui

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/sdk/extui"
)

func TestDialogKeyHintsSurviveTypedProjection(t *testing.T) {
	want := []extui.DialogKeyHint{
		{Key: "F2", Text: "Настройки", Action: "envman.settings", Icon: "file-cog"},
		{Key: "Esc", Text: "Cancel", Action: "envman.cancel", Disabled: true},
	}
	dialog := appDialogFromLegacy(map[string]any{
		"kind": "dialog", "keyHints": []map[string]any{
			{"key": "F2", "text": "Настройки", "action": "envman.settings", "icon": "file-cog"},
			{"key": "Esc", "text": "Cancel", "action": "envman.cancel", "disabled": true},
		},
		"paneSplit": map[string]any{"x": 35, "leftTitle": "Profiles", "rightTitle": "Details", "active": "left"},
	})
	dialog.Legacy = nil
	if got := appDialogFromLegacy(dialog.ToMap()).KeyHints; !reflect.DeepEqual(got, want) {
		t.Fatalf("typed dialog lost hints: %#v", got)
	}
	if _, exists := (extui.DialogModel{}).ToMap()["keyHints"]; exists {
		t.Fatal("ordinary dialogs should omit absent hints")
	}
	if got := appDialogFromLegacy(dialog.ToMap()).PaneSplit; got == nil || *got != *dialog.PaneSplit {
		t.Fatalf("typed dialog lost pane split: %#v", got)
	}
}
