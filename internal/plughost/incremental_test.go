package plughost

import (
	bytes "bytes"
	semantic "github.com/unxed/f4/internal/semantic"
	extui "github.com/unxed/f4/sdk/extui"
	vtui "github.com/unxed/vtui"
	testing "testing"
)

func TestExtUiRendererIncrementalReconciliationClearsUnsafeBoundary(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })
	vtui.FrameManager.Push(vtui.NewDesktop())
	vtui.FrameManager.Push(vtui.NewCenteredDialog(32, 7, " Bounded "))

	projected, ok := Presentation.ProjectScene(&vtui.SemanticContext{
		Width: 100, Height: 30, ActiveScreen: 0,
	})
	if !ok {
		t.Fatal("initial bounded projection was unavailable")
	}
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}}
	renderer.SetSemanticScene(projected.Snapshot())
	renderer.Flush()
	extUiDrainBufferedMessages(t, &wire)

	renderer.BeginSemanticSceneUpdate()
	renderer.EndSemanticSceneUpdate()
	if !renderer.semanticFastPathUnsafe {
		t.Fatal("unhandled mutation boundary was not marked unsafe")
	}
	if !renderer.SetSemanticSceneIncremental(&vtui.SemanticContext{
		Width: 100, Height: 30, ActiveScreen: 0,
	}) {
		t.Fatal("unchanged bounded reconciliation was rejected")
	}
	if renderer.semanticFastPathUnsafe {
		t.Fatal("successful bounded reconciliation left fast paths disabled")
	}
	if renderer.panelActivationQueued || renderer.panelActivationProjected {
		t.Fatal("successful reconciliation retained stale activation bookkeeping")
	}
}

func TestExtUiRendererDirectMenuLifecycleNeverExportsLargeCatalogs(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 40)
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })
	vtui.FrameManager.Push(vtui.NewDesktop())

	root, ok := Presentation.MenuState(nil)
	if !ok {
		t.Fatal("bounded initial root was unavailable")
	}
	entries := make([]map[string]any, 20_000)
	for index := range entries {
		entries[index] = map[string]any{
			"index": index, "entryId": "entry", "name": "large-file-name",
		}
	}
	initial := semantic.SemanticShallowMapCopy(root)
	initial["type"] = "scene"
	initial["schema"] = extui.Schema
	initial["version"] = extui.SceneVersion
	initial["shell"] = map[string]any{
		"id": "shell", "kind": "shell", "mode": "panels",
		"activePanel": 0, "showPanels": true,
		"showLeftPanel": true, "showRightPanel": true,
		"panels": []map[string]any{
			incrementalTestPanel(0, entries),
			incrementalTestPanel(1, entries),
		},
	}
	// App v4 treats these as compatibility aliases. A future/foreign alias
	// shape must not disable a typed shell transition.
	initial["frames"] = map[string]any{"futureAliasShape": true}
	initial["screens"] = "future-screen-alias"
	initial["legacy"] = []any{"future-legacy-alias"}

	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send:                         &extUiMessageSender{w: &wire},
		NativeSemanticSurfaceEnabled: true,
	}
	screen.Renderer = renderer
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	extUiDrainBufferedMessages(t, &wire)

	menu := vtui.NewVMenu(" Drives ")
	menu.AddItem(vtui.MenuItem{
		Text: "Other panel", Icon: "panels-top-left",
	})
	menu.AddItem(vtui.MenuItem{Text: "C: Local", Icon: "hard-drive"})
	menu.SetPosition(10, 5, 35, 9)

	// Clicking the drive icon on the inactive panel performs both mutations in
	// one posted task. The activation and root-only menu patch use independent
	// wire streams, but together they are the atomic result of that one click.
	// If the renderer treats panel_activation as the entire handled boundary,
	// this first menu is stranded in Go. A second click then opens a duplicate,
	// which is why an outside click appears to need two presses as well.
	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(1)
	renderer.AllowSemanticMenuAfterPanelActivation()
	vtui.FrameManager.Push(menu)
	if !renderer.SetSemanticMenuState(nil) {
		t.Fatal("inactive-panel activation discarded the drive menu update")
	}
	renderer.EndSemanticSceneUpdate()
	openWireBytes := wire.Len()
	openedMessages := extUiDrainBufferedMessages(t, &wire)
	if len(openedMessages) != 2 ||
		semantic.String(openedMessages[0]["type"]) != "panel_activation" ||
		semantic.String(openedMessages[1]["type"]) != "scene_patch" {
		t.Fatalf("combined click wire order = %#v, want activation then menu patch",
			openedMessages)
	}
	activation := extUiBufferedMessageOfType(openedMessages, "panel_activation")
	opened := extUiBufferedMessageOfType(openedMessages, "scene_patch")
	if activation == nil || ExtUiInt(activation, "activePanel") != 1 {
		t.Fatalf("combined click omitted panel activation: %#v", openedMessages)
	}
	if opened == nil || !semanticValueContainsKey(opened, "icon") {
		t.Fatalf("combined click omitted the drive menu: %#v", openedMessages)
	}
	if semanticValueContainsKey(opened, "entries") {
		t.Fatalf("combined click leaked a file catalog: %#v", opened)
	}
	if menus := semantic.AppMapSlice(renderer.lastCompactScene["menus"]); len(menus) != 1 {
		t.Fatalf("combined click installed %d drive menus, want exactly one", len(menus))
	}
	if renderer.semanticUpdatePanelActivation || renderer.semanticUpdateActivationMenu {
		t.Fatal("successful composed menu retained transaction permissions")
	}
	if openWireBytes > 4352 {
		t.Fatalf("combined activation/menu update is %d bytes, want bounded packets",
			openWireBytes)
	}
	compactShell, _ := renderer.lastCompactScene["shell"].(map[string]any)
	if got := semantic.Int(compactShell["activePanel"]); got != 1 {
		t.Fatalf("compact snapshot active panel = %d, want 1", got)
	}
	compactPanels := semantic.AppMapSlice(compactShell["panels"])
	if len(compactPanels) != 2 || semantic.AppBool(compactPanels[0]["active"]) ||
		!semantic.AppBool(compactPanels[1]["active"]) {
		t.Fatalf("compact snapshot panel activation = %#v", compactPanels)
	}
	if !renderer.ConsumeSemanticSceneExportSuppression() {
		t.Fatal("combined activation/menu update did not suppress its full export")
	}

	assertDirectMenuPatch := func(name string, mutate func()) map[string]any {
		t.Helper()
		renderer.BeginSemanticSceneUpdate()
		mutate()
		if !renderer.SetSemanticMenuState(nil) {
			t.Fatalf("%s menu state was not accepted", name)
		}
		renderer.EndSemanticSceneUpdate()
		payloadBytes := wire.Len()
		message, err := extUiReadMessage(&wire)
		if err != nil {
			t.Fatalf("%s menu patch was not sent: %v", name, err)
		}
		if message["type"] != "scene_patch" {
			t.Fatalf("%s sent %q, want scene_patch", name, message["type"])
		}
		if semanticValueContainsKey(message, "entries") {
			t.Fatalf("%s menu patch leaked a file catalog", name)
		}
		if payloadBytes > 4096 {
			t.Fatalf("%s menu patch is %d bytes, want a bounded packet", name, payloadBytes)
		}
		if !renderer.ConsumeSemanticSceneExportSuppression() {
			t.Fatalf("%s did not suppress the following full export", name)
		}
		return message
	}

	assertDirectMenuPatch("selection", func() { menu.SetSelectPos(1) })
	closed := assertDirectMenuPatch("close", func() {
		menu.Close()
		vtui.FrameManager.RemoveFrame(menu)
	})
	rootPatch, _ := closed["root"].(map[string]any)
	cleared, _ := rootPatch["clear"].([]any)
	foundMenus := false
	for _, key := range cleared {
		foundMenus = foundMenus || key == "menus"
	}
	if !foundMenus {
		t.Fatalf("close patch did not clear menus: %#v", closed)
	}
	if menus := semantic.AppMapSlice(renderer.lastCompactScene["menus"]); len(menus) != 0 {
		t.Fatalf("one close left %d drive menus behind", len(menus))
	}
}

func TestExtUiRendererMenuCompositionRequiresExplicitActivationContract(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })
	vtui.FrameManager.Push(vtui.NewDesktop())

	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send:                         &extUiMessageSender{w: &wire},
		NativeSemanticSurfaceEnabled: true,
	}
	renderer.SetSemanticScene(panelActivationFastPathScene(
		0, `Panels: C:\left`))
	renderer.Flush()
	extUiDrainBufferedMessages(t, &wire)

	menu := vtui.NewVMenu(" Drives ")
	menu.AddItem(vtui.MenuItem{Text: "C: Local", Icon: "hard-drive"})
	menu.SetPosition(10, 5, 35, 8)
	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(1, `Panels: D:\right`)
	vtui.FrameManager.Push(menu)
	if renderer.SetSemanticMenuState(nil) {
		t.Fatal("undeclared activation inherited permission for a second compact update")
	}
	renderer.EndSemanticSceneUpdate()

	messages := extUiDrainBufferedMessages(t, &wire)
	if len(messages) != 1 || semantic.String(messages[0]["type"]) != "panel_activation" {
		t.Fatalf("rejected composition sent unexpected messages: %#v", messages)
	}
	if renderer.suppressSemanticExport || renderer.deferSemanticRender ||
		!renderer.semanticFastPathUnsafe {
		t.Fatal("rejected composition did not restore the authoritative export path")
	}
	menu.Close()
	vtui.FrameManager.RemoveFrame(menu)
}
