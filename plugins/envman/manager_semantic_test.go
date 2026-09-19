package envman

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestManagerRepeatedNavigationKeepsListPresentation(t *testing.T) {
	entries := make([]Entry, 40)
	for index := range entries {
		entries[index] = Entry{
			Kind: KindProfile, Name: fmt.Sprintf("Profile %02d", index), Enabled: index%2 == 0,
			Variables: []string{fmt.Sprintf("VALUE=%d", index)},
		}
	}
	controller, _, _, _ := newManagerControllerForTest(t, Config{Version: CurrentConfigVersion, Entries: entries})
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 60)
	want := managerSemanticList(t, controller)
	durations := make([]time.Duration, 0, 200)
	bytes := 0
	for step := 0; step < 200; step++ {
		started := time.Now()
		controller.dialog.ProcessKey(managerTestKey(vtinput.VK_DOWN, 0, 0))
		controller.dialog.Show(screen)
		node := controller.dialog.SemanticNode(nil)
		payload, err := json.Marshal(node)
		if err != nil {
			t.Fatal(err)
		}
		durations = append(durations, time.Since(started))
		bytes += len(payload)
		if controller.list.SelectPos != (step+1)%len(entries) {
			t.Fatalf("navigation step %d: cursor=%d", step, controller.list.SelectPos)
		}
		got := managerSemanticList(t, controller)
		for _, key := range []string{"items", "itemStates"} {
			if !reflect.DeepEqual(got[key], want[key]) {
				t.Fatalf("navigation changed %s", key)
			}
		}
	}
	slices.Sort(durations)
	t.Logf("200 navigation + console draw + semantic JSON snapshots: median=%s p95=%s mean payload=%d bytes",
		durations[len(durations)/2], durations[len(durations)*95/100], bytes/len(durations))
}

func managerSemanticList(t *testing.T, controller *managerController) map[string]any {
	t.Helper()
	for _, child := range controller.dialog.SemanticNode(nil)["children"].([]map[string]any) {
		if child["id"] == vtui.SemanticID(controller.list) {
			return child
		}
	}
	t.Fatal("semantic profile list missing")
	return nil
}

func TestManagerSemanticResponsiveLayout(t *testing.T) {
	controller, _, _, _ := newManagerControllerForTest(t, Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{{Kind: KindProfile, Name: "one", Enabled: true}},
	})
	for _, editing := range []bool{false, true} {
		if editing {
			controller.handleKey(managerTestKey(vtinput.VK_F4, 0, 0))
		}
		node := controller.dialog.SemanticNode(nil)
		if node["layout"] != "environmentProfiles" {
			t.Fatalf("native layout = %v, want environmentProfiles", node["layout"])
		}
		roles := map[string]string{}
		for _, child := range node["children"].([]map[string]any) {
			role, _ := child["layoutRole"].(string)
			if role == "" {
				t.Fatalf("control %v has no native layout role", child["id"])
			}
			if _, duplicate := roles[role]; duplicate {
				t.Fatalf("duplicate layout role %q", role)
			}
			roles[role] = child["id"].(string)
		}
		for _, role := range []string{"profiles", "name-label", "name", "enabled", "variables-label", "variables"} {
			if roles[role] == "" {
				t.Fatalf("missing layout role %q", role)
			}
		}
		if roles["profiles"] != vtui.SemanticID(controller.list) || roles["variables"] != vtui.SemanticID(controller.variablesEdit) {
			t.Fatal("native layout replaced control action identities")
		}
	}
}

func TestManagerSemanticProfileCheckboxes(t *testing.T) {
	controller, _, _, store := newManagerControllerForTest(t, Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindProfile, Name: "&Enabled", Enabled: true},
			{Kind: KindProfile, Name: "[x] literal name"},
			{Kind: KindSeparator},
		},
	})
	list := managerSemanticList(t, controller)
	items := list["items"].([]string)
	if items[0] != "Enabled" || items[1] != "[x] literal name" {
		t.Fatalf("GUI labels retain console decorations: %#v", items)
	}
	if controller.list.Items[0] != "[x] Enabled" || controller.list.Items[1] != "[ ] [x] literal name" {
		t.Fatal("GUI projection changed console row markers")
	}
	want := []map[string]any{
		{"checkable": true, "checked": true, "dimmed": false},
		{"checkable": true, "checked": false, "dimmed": true},
		{"checkable": false, "checked": false, "dimmed": true},
	}
	if !reflect.DeepEqual(list["itemStates"], want) {
		t.Fatalf("checkbox states = %#v", list["itemStates"])
	}
	action := map[string]any{"action": "control.toggle", "target": vtui.SemanticID(controller.list), "index": 1}
	controller.dialog.SetFocusedItem(controller.addButton)
	if !controller.dialog.HandleSemanticAction(action) {
		t.Fatal("native checkbox click was not handled")
	}
	if !store.Snapshot().Entries[1].Enabled || controller.list.SelectPos != 1 {
		t.Fatal("checkbox did not persist through the existing profile toggle")
	}
	if controller.dialog.GetFocusedItem() != controller.list {
		t.Fatal("checkbox click did not transfer keyboard focus to the profile list")
	}
	want[1]["checked"], want[1]["dimmed"] = true, false
	if !reflect.DeepEqual(managerSemanticList(t, controller)["itemStates"], want) {
		t.Fatal("checkbox presentation did not follow persisted state")
	}
	// Choosing a row must not toggle it.
	controller.dialog.HandleSemanticAction(map[string]any{
		"action": "control.select", "target": vtui.SemanticID(controller.list), "index": 1,
	})
	if !store.Snapshot().Entries[1].Enabled {
		t.Fatal("row selection toggled the profile")
	}
	for _, index := range []int{-1, 2, 3} {
		action["index"] = index
		if controller.dialog.HandleSemanticAction(action) {
			t.Fatalf("invalid/non-profile toggle accepted: %d", index)
		}
	}
	action["index"] = 1
	controller.handleKey(managerTestKey(vtinput.VK_F4, 0, 0))
	if controller.dialog.HandleSemanticAction(action) || !store.Snapshot().Entries[1].Enabled {
		t.Fatal("disabled profile list accepted toggle during inline editing")
	}
}

func TestManagerSemanticEmptyListHasNoCheckbox(t *testing.T) {
	controller, _, _, _ := newManagerControllerForTest(t, Config{Version: CurrentConfigVersion})
	states := managerSemanticList(t, controller)["itemStates"]
	want := []map[string]any{{"checkable": false, "checked": false, "dimmed": false}}
	if !reflect.DeepEqual(states, want) {
		t.Fatalf("empty list decoration = %#v", states)
	}
}

func TestManagerSemanticKeyHintsFollowMode(t *testing.T) {
	controller, _, _, _ := newManagerControllerForTest(t, Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{{Kind: KindProfile, Name: "one", Variables: []string{"A=one"}}},
	})
	check := func(expected []string) {
		t.Helper()
		hints, ok := controller.dialog.SemanticNode(nil)["keyHints"].([]map[string]any)
		if !ok || len(hints) != len(expected) {
			t.Fatalf("GUI key hints = %#v, want keys %v", hints, expected)
		}
		for index, key := range expected {
			if hints[index]["key"] != key || hints[index]["text"] == "" {
				t.Fatalf("hint %d = %#v, want %s and its localized label", index, hints[index], key)
			}
			icons := map[string]string{
				"Ins": "plus", "Del": "trash-2", "F4": "pencil", "F5": "copy",
				"Space": "circle-check", "Ctrl+↑": "arrow-up", "Ctrl+↓": "arrow-down",
				"F2": "file-cog", "Ctrl+Enter": "save", "Esc": "x",
			}
			if hints[index]["icon"] != icons[key] {
				t.Fatalf("hint %s icon = %v, want %s", key, hints[index]["icon"], icons[key])
			}
		}
		split, ok := controller.dialog.SemanticNode(nil)["paneSplit"].(map[string]any)
		if !ok || split["leftTitle"] != controller.dialog.profilesTitle ||
			split["x"] != controller.dialog.X1+controller.dialog.splitOffset {
			t.Fatalf("GUI pane split = %#v", split)
		}
		wantTitle, wantActive := controller.dialog.detailsTitle, "left"
		if controller.editing {
			wantTitle, wantActive = controller.dialog.editingTitle, "right"
		}
		if split["rightTitle"] != wantTitle || split["active"] != wantActive {
			t.Fatalf("GUI pane split did not follow editing mode: %#v", split)
		}
	}
	listKeys := []string{"Ins", "Del", "F4", "F5", "Space", "Ctrl+↑", "Ctrl+↓", "F2"}
	check(listKeys)
	controller.handleKey(managerTestKey(vtinput.VK_F4, 0, 0))
	check([]string{"Ctrl+Enter", "Esc"})
	controller.cancelInlineEdit()
	check(listKeys)
}

func TestManagerSemanticFooterActions(t *testing.T) {
	controller, _, _, store := newManagerControllerForTest(t, Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindProfile, Name: "one", Enabled: true},
			{Kind: KindProfile, Name: "two"},
		},
	})
	invoke := func(name string) bool {
		return controller.dialog.HandleSemanticAction(map[string]any{
			"target": vtui.SemanticID(controller.dialog.Window), "action": name,
		})
	}
	hints := controller.dialog.SemanticNode(nil)["keyHints"].([]map[string]any)
	wantActions := []string{
		"envman.add", "envman.delete", "envman.edit", "envman.copy", "envman.toggle",
		"envman.moveUp", "envman.moveDown", "envman.settings",
	}
	if len(hints) != len(wantActions) {
		t.Fatalf("footer action count = %d", len(hints))
	}
	for index, action := range wantActions {
		if hints[index]["action"] != action {
			t.Fatalf("footer action %d = %#v", index, hints[index])
		}
	}
	if !invoke("envman.toggle") || store.Snapshot().Entries[0].Enabled {
		t.Fatal("footer toggle did not use profile persistence")
	}
	controller.list.SetSelectPos(0)
	controller.selectionChanged()
	if !invoke("envman.moveDown") || store.Snapshot().Entries[1].Name != "one" {
		t.Fatal("footer move down did not reorder the selected profile")
	}
	if !invoke("envman.moveUp") || store.Snapshot().Entries[0].Name != "one" {
		t.Fatal("footer move up did not restore the order")
	}
	if !invoke("envman.edit") || !controller.editing {
		t.Fatal("footer edit did not open inline editor")
	}
	hints = controller.dialog.SemanticNode(nil)["keyHints"].([]map[string]any)
	if len(hints) != 2 || hints[0]["action"] != "envman.save" || hints[1]["action"] != "envman.cancel" {
		t.Fatalf("inline actions = %#v", hints)
	}
	controller.nameEdit.SetText("discarded")
	if invoke("envman.delete") || invoke("envman.settings") {
		t.Fatal("stale browsing action accepted during editing")
	}
	if !invoke("envman.cancel") || controller.editing || store.Snapshot().Entries[0].Name != "one" {
		t.Fatal("footer cancel did not discard inline edits")
	}
	if !invoke("envman.edit") {
		t.Fatal("edit action not handled")
	}
	controller.nameEdit.SetText("saved")
	if !invoke("envman.save") || controller.editing || store.Snapshot().Entries[0].Name != "saved" {
		t.Fatal("footer save did not persist inline edits")
	}
	if invoke("envman.save") || invoke("envman.cancel") {
		t.Fatal("stale editing action accepted in browse mode")
	}
}

func TestManagerSemanticFooterAvailability(t *testing.T) {
	controller, _, _, _ := newManagerControllerForTest(t, Config{Version: CurrentConfigVersion})
	hints := controller.dialog.SemanticNode(nil)["keyHints"].([]map[string]any)
	for _, hint := range hints {
		action := hint["action"].(string)
		enabled := action == "envman.add" || action == "envman.settings"
		if hint["disabled"] != !enabled {
			t.Fatalf("empty-list availability = %#v", hint)
		}
		if !enabled && controller.dialog.HandleSemanticAction(map[string]any{
			"target": vtui.SemanticID(controller.dialog.Window), "action": action,
		}) {
			t.Fatalf("disabled action was accepted: %s", action)
		}
	}
	if !controller.dialog.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(controller.dialog.Window), "action": "envman.add",
	}) || vtui.FrameManager.GetTopFrame() == controller.dialog {
		t.Fatal("Add button did not open the new-profile dialog in an empty list")
	}
}
