package nativeui

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/sdk/extui"
)

func TestListItemStatesSurviveTypedProjection(t *testing.T) {
	want := []extui.ListItemState{
		{Checkable: true, Checked: true},
		{Checkable: true, Dimmed: true},
		{Dimmed: true},
	}
	control := appControlFromLegacy(map[string]any{
		"kind": "listBox", "items": []string{"on", "off", "separator"},
		"itemStates": []map[string]any{
			{"checkable": true, "checked": true},
			{"checkable": true, "dimmed": true},
			{"dimmed": true},
		},
	})
	control.Legacy = nil
	if got := appControlFromLegacy(control.ToMap()).ItemStates; !reflect.DeepEqual(got, want) {
		t.Fatalf("typed row states = %#v", got)
	}
	if _, exists := (extui.ControlModel{}).ToMap()["itemStates"]; exists {
		t.Fatal("ordinary lists should omit row states")
	}
}
