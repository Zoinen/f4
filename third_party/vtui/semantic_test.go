package vtui

import (
	"bytes"
	"testing"
)

func TestSemanticSceneExportsWindowWidgets(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	FrameManager.Init(scr)

	dlg := NewDialog(10, 5, 40, 12, " Test ")
	edit := NewEdit(12, 7, 12, "value")
	button := NewButton(12, 9, "&Ok")
	dlg.AddItem(NewLabel(12, 6, "&Name", edit))
	dlg.AddItem(edit)
	dlg.AddItem(button)
	FrameManager.Push(dlg)

	scene := FrameManager.ExportSemanticScene()
	if scene["type"] != "scene" {
		t.Fatalf("scene type = %v, want scene", scene["type"])
	}
	frames := scene["frames"].([]map[string]any)
	if len(frames) == 0 || frames[len(frames)-1]["kind"] != "dialog" {
		t.Fatalf("expected top dialog frame, got %#v", frames)
	}
	children := frames[len(frames)-1]["children"].([]map[string]any)
	if len(children) != 3 {
		t.Fatalf("children len = %d, want 3", len(children))
	}
	if children[1]["kind"] != "edit" || children[1]["text"] != "value" {
		t.Fatalf("unexpected edit node: %#v", children[1])
	}
	if children[2]["kind"] != "button" {
		t.Fatalf("unexpected button node: %#v", children[2])
	}
}

func TestSemanticActionActivatesButton(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	FrameManager.Init(scr)

	clicked := false
	dlg := NewDialog(10, 5, 40, 12, " Test ")
	button := NewButton(12, 9, "&Ok")
	button.OnClick = func() { clicked = true }
	dlg.AddItem(button)
	FrameManager.Push(dlg)

	if !FrameManager.HandleSemanticAction(map[string]any{
		"target": SemanticID(button),
		"action": "activate",
	}) {
		t.Fatal("semantic action was not handled")
	}
	if !clicked {
		t.Fatal("button action did not run")
	}
}

func TestSemanticMenuBarCoordinatesAndActivation(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	FrameManager.Init(scr)

	mb := NewMenuBar(nil)
	mb.SetPosition(0, 0, 79, 0)
	mb.Items = []MenuBarItem{
		{Label: "&Left", SubItems: []MenuItem{{Text: "&One", Command: 101}}},
		{Label: "&Files", SubItems: []MenuItem{{Text: "&Two", Command: 102}}},
	}
	FrameManager.MenuBar = mb
	FrameManager.Push(NewDesktop())

	scene := FrameManager.ExportSemanticScene()
	menu := scene["menuBar"].(map[string]any)
	items := menu["items"].([]map[string]any)
	if items[0]["x"] != 2 {
		t.Fatalf("first menu x = %v, want 2", items[0]["x"])
	}
	if items[0]["w"].(int) <= 0 || items[1]["x"].(int) <= items[0]["x"].(int) {
		t.Fatalf("unexpected menu item metrics: %#v", items)
	}

	if !FrameManager.HandleSemanticAction(map[string]any{
		"action": "menuBar.activate",
		"index":  float64(1),
	}) {
		t.Fatal("menu bar action was not handled")
	}
	if !mb.Active || mb.SelectPos != 1 {
		t.Fatalf("menu bar state: active=%v selected=%d", mb.Active, mb.SelectPos)
	}
	if FrameManager.GetTopFrameType() != TypeMenu {
		t.Fatalf("top frame type = %v, want TypeMenu", FrameManager.GetTopFrameType())
	}
}

func TestSemanticSceneAdapterCanPromoteLegacyScene(t *testing.T) {
	oldAdapter := AppSceneAdapter
	defer func() { AppSceneAdapter = oldAdapter }()

	AppSceneAdapter = func(ctx *SemanticContext, legacy map[string]any) map[string]any {
		if legacy["type"] != "scene" {
			t.Fatalf("legacy scene type = %v, want scene", legacy["type"])
		}
		return map[string]any{
			"type":    "scene",
			"schema":  "app",
			"version": 2,
			"width":   ctx.Width,
			"height":  ctx.Height,
			"legacy":  legacy,
		}
	}

	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	FrameManager.Init(scr)
	FrameManager.Push(NewDesktop())

	scene := FrameManager.ExportSemanticScene()
	if scene["schema"] != "app" || scene["version"] != 2 {
		t.Fatalf("adapter scene = %#v, want app schema v2", scene)
	}
	if scene["legacy"] == nil {
		t.Fatalf("adapter scene did not keep legacy fallback: %#v", scene)
	}
}

func TestQtRendererFlushesSemanticScene(t *testing.T) {
	var wire bytes.Buffer
	renderer := NewQtRendererWithSender(nil, &qtMessageSender{w: &wire})
	renderer.cursorDirty = false
	renderer.SetSemanticScene(map[string]any{
		"type":    "scene",
		"version": SemanticSceneVersion,
		"width":   80,
		"height":  25,
	})
	renderer.Flush()

	msg, err := qtReadMessage(&wire)
	if err != nil {
		t.Fatalf("read scene failed: %v", err)
	}
	if qtString(msg, "type") != "scene" {
		t.Fatalf("message type = %q, want scene", qtString(msg, "type"))
	}
	if qtInt(msg, "version") != SemanticSceneVersion {
		t.Fatalf("scene version = %d, want %d", qtInt(msg, "version"), SemanticSceneVersion)
	}
}
