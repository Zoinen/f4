package nativeui

import (
	"testing"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

func TestPluginHotkeyDialogPreservesMixedOverlayStack(t *testing.T) {
	oldManager := *vtui.FrameManager
	t.Cleanup(func() { *vtui.FrameManager = oldManager })
	screen := vtui.NewScreenBuf()
	screen.AllocBuf(100, 40)
	vtui.FrameManager.Init(screen)
	menu := vtui.NewVMenu("Plugins")
	menu.AddItem(vtui.MenuItem{Text: "Environment Manager"})
	vtui.FrameManager.PushMenu(menu)
	dialog := panel.NewPluginHotkeyAssignFrame(nil, "plugin.test", "Environment Manager", nil)
	vtui.FrameManager.Push(dialog)

	scene := vtui.FrameManager.ExportSemanticScene()
	menus := semantic.AppMapSlice(scene["menus"])
	dialogs := semantic.AppMapSlice(scene["dialogs"])
	if len(menus) != 1 || len(dialogs) != 1 {
		t.Fatalf("expected both overlays, got menus=%#v dialogs=%#v", menus, dialogs)
	}
	if got := semantic.Int(menus[0]["stackOrder"]); got != 1 {
		t.Fatalf("plugin menu stackOrder = %d, want 1", got)
	}
	if got := semantic.Int(dialogs[0]["stackOrder"]); got != 2 {
		t.Fatalf("F4 dialog stackOrder = %d, want 2 above plugin menu", got)
	}
	incremental, supported := BuildAppIncrementalScene(nil)
	if !supported {
		t.Fatal("mixed menu/dialog scene should support incremental projection")
	}
	incrementalDialogs := semantic.AppMapSlice(incremental.Scene["dialogs"])
	if len(incrementalDialogs) != 1 || semantic.Int(incrementalDialogs[0]["stackOrder"]) != 2 {
		t.Fatalf("incremental F4 dialog lost stack order: %#v", incrementalDialogs)
	}

	// A menu opened by a dialog must still appear above that dialog.
	child := vtui.NewVMenu("Child")
	child.AddItem(vtui.MenuItem{Text: "Choice"})
	vtui.FrameManager.PushMenu(child)
	active := appActiveVMenus()
	if got := semantic.Int(active[len(active)-1].model().ToMap()["stackOrder"]); got != 3 {
		t.Fatalf("child menu stackOrder = %d, want 3 above dialog", got)
	}
	menuState, supported := BuildAppMenuState(nil)
	if !supported {
		t.Fatal("mixed stack should support menu-only updates")
	}
	updatedMenus := semantic.AppMapSlice(menuState["menus"])
	if len(updatedMenus) != 2 || semantic.Int(updatedMenus[1]["stackOrder"]) != 3 {
		t.Fatalf("menu-only update lost shared stack order: %#v", updatedMenus)
	}
	edit := vtui.NewEdit(0, 0, 40, "git st")
	edit.History = []string{"git status", "git stash"}
	autocomplete := vtui.NewAutoCompleteMenu(edit)
	vtui.FrameManager.Push(autocomplete)
	for _, projection := range []string{"complete", "incremental", "menus"} {
		var state map[string]any
		switch projection {
		case "complete":
			state = vtui.FrameManager.ExportSemanticScene()
		case "incremental":
			result, ok := BuildAppIncrementalScene(nil)
			if !ok {
				t.Fatal("autocomplete incremental projection rejected")
			}
			state = result.Scene
		case "menus":
			state, _ = BuildAppMenuState(nil)
		}
		menus := semantic.AppMapSlice(state["menus"])
		if len(menus) != 3 || semantic.Int(menus[2]["stackOrder"]) != 4 || menus[2]["role"] != "autocomplete" {
			t.Fatalf("%s autocomplete lost shared stack order: %#v", projection, menus)
		}
	}
}

func TestOverlayStackOrderSurvivesTypedProjection(t *testing.T) {
	menu := appMenuFromLegacy(map[string]any{"stackOrder": 3}, "popup")
	menu.Legacy = nil
	if got := semantic.Int(menu.ToMap()["stackOrder"]); got != 3 {
		t.Fatalf("typed menu stackOrder = %d", got)
	}
	dialog := appDialogFromLegacy(map[string]any{"stackOrder": 2})
	dialog.Legacy = nil
	if got := semantic.Int(dialog.ToMap()["stackOrder"]); got != 2 {
		t.Fatalf("typed dialog stackOrder = %d", got)
	}
	for _, node := range []map[string]any{appMenuFromLegacy(nil, "popup").ToMap(), appDialogFromLegacy(nil).ToMap()} {
		if _, exists := node["stackOrder"]; exists {
			t.Fatal("absent stack order should remain absent for compatibility")
		}
	}
}
