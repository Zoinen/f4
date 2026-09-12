package nativeui

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestHelpDialogSurvivesNativeSceneProjection(t *testing.T) {
	engine := vtui.NewHelpEngine(nil)
	engine.AddTopic(&vtui.HelpTopic{Name: "Contents", Lines: []string{"~Next~Next@"}})
	view := vtui.NewHelpView(engine, "Contents")
	ctx := &vtui.SemanticContext{Width: 80, Height: 25}
	node := view.SemanticNode(ctx)
	scene := BuildAppSceneFromLegacy(ctx, map[string]any{
		"width": 80, "height": 25, "frames": []map[string]any{node},
	})
	dialogs := semantic.AppMapSlice(scene["dialogs"])
	if len(dialogs) != 1 || dialogs[0]["layout"] != "help" {
		t.Fatalf("help lost from native dialog stream: %v", dialogs)
	}
	rows := semantic.AppMapSlice(dialogs[0]["helpLines"])
	if len(rows) != 1 || dialogs[0]["topic"] != "Contents" {
		t.Fatalf("help content lost in dialog conversion: %v", dialogs[0])
	}
}

func TestHelpSearchSurvivesNativeSceneProjection(t *testing.T) {
	defer testutil.SwapFrameManager(t)()
	oldEngine := vtui.GlobalHelpEngine
	t.Cleanup(func() { vtui.GlobalHelpEngine = oldEngine; dialog.ResetHelpState() })
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	engine := vtui.NewHelpEngine(nil)
	engine.AddTopic(&vtui.HelpTopic{Name: "Search", Lines: []string{"😀 #Nee#~dle~Next@ and NEEDLE"}})
	vtui.GlobalHelpEngine = engine
	view := vtui.NewHelpView(engine, "Search")
	vtui.FrameManager.Push(view)
	for _, r := range "needle" {
		dialog.HandleHelpSearchHotkey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}
	node := view.SemanticNode(&vtui.SemanticContext{Width: 80, Height: 25})
	projected := appDialogFromLegacy(node).ToMap()
	if !strings.Contains(semantic.String(projected["title"]), "[needle]") || projected["searchHint"] == nil {
		t.Fatalf("search query/hint missing from Help: %v", projected)
	}
	rows := semantic.AppMapSlice(projected["helpLines"])
	spans := semantic.AppMapSlice(rows[0]["spans"])
	selected := ""
	for _, span := range spans {
		if span["searchSelected"] == true {
			selected += semantic.String(span["text"])
		}
	}
	if selected != "Needle" {
		t.Fatalf("selected match = %q", selected)
	}
	if !dialog.MoveHelpSearch(view, false) {
		t.Fatal("next match was not selected")
	}
	projected = appDialogFromLegacy(view.SemanticNode(nil)).ToMap()
	spans = semantic.AppMapSlice(semantic.AppMapSlice(projected["helpLines"])[0]["spans"])
	selected = ""
	for _, span := range spans {
		if span["searchSelected"] == true {
			selected += semantic.String(span["text"])
		}
	}
	if selected != "NEEDLE" {
		t.Fatalf("next selected match = %q", selected)
	}
	engine.AddTopic(&vtui.HelpTopic{Name: "Next", Lines: []string{"needle"}})
	view.SwitchTopic("Next")
	projected = appDialogFromLegacy(view.SemanticNode(nil)).ToMap()
	if strings.Contains(semantic.String(projected["title"]), "[needle]") {
		t.Fatal("previous topic search leaked into next topic")
	}
	if strings.Contains(semantic.String(node["title"]), "[needle]") {
		t.Fatal("projection mutated the owner node")
	}
}
