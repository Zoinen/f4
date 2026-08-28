package main

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

func incrementalTestContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func incrementalTestPanel(side int, entries []map[string]any) map[string]any {
	return map[string]any{
		"id": "panel-" + string(rune('0'+side)), "kind": "filePanel",
		"side": side, "active": side == 0, "path": "C:/large",
		"catalogRevision": int64(11), "selectionRevision": int64(5),
		"metadataDeferred": true, "metadataRevision": int64(7),
		"cursor": 0, "cursorEntryId": "entry-0", "selectedCount": 0,
		"totalCount": len(entries), "entries": entries,
	}
}

func semanticValueContainsKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for candidate, nested := range typed {
			if candidate == key || semanticValueContainsKey(nested, key) {
				return true
			}
		}
	case []map[string]any:
		for _, nested := range typed {
			if semanticValueContainsKey(nested, key) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if semanticValueContainsKey(nested, key) {
				return true
			}
		}
	}
	return false
}

func TestAppScenePatchMenuDoesNotCarryLargePanelCatalogs(t *testing.T) {
	entries := make([]map[string]any, 20_000)
	for index := range entries {
		entries[index] = map[string]any{
			"index": index, "entryId": "stable-entry", "name": "file",
			"selected": false,
		}
	}
	full := map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"width": 120, "height": 40, "activeScreen": 0,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "mode": "panels",
			"activePanel": 0,
			"panels": []map[string]any{
				incrementalTestPanel(0, entries),
				incrementalTestPanel(1, entries),
			},
		},
	}
	previous := compactAppSemanticScene(full)
	currentScene := semanticShallowMapCopy(previous)
	currentScene["shell"] = semanticShallowMapCopy(
		previous["shell"].(map[string]any))
	currentScene["menus"] = []map[string]any{{
		"id": "drive-menu", "role": "popup", "title": "Drives",
		"items": []map[string]any{
			{"index": 0, "text": "Other panel", "icon": "panels-top-left"},
			{"index": 1, "text": "C: Local", "icon": "hard-drive"},
		},
	}}

	patch, _, ok := buildAppScenePatch(previous, &appIncrementalScene{
		Scene: currentScene, Panels: map[int]*FileSystemPanel{},
	})
	if !ok {
		t.Fatal("row-free menu transition was rejected")
	}
	if patch.Root == nil || patch.Shell != nil {
		t.Fatalf("menu transition patch = %#v, want root-only", patch)
	}
	wire := patch.ToMap()
	if semanticValueContainsKey(wire, "entries") {
		t.Fatal("menu patch leaked a panel catalog")
	}
	if !semanticValueContainsKey(wire, "icon") {
		t.Fatal("menu patch lost semantic item icons")
	}
	payload, err := msgpack.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 1024 {
		t.Fatalf("menu patch is %d bytes; expected a bounded packet", len(payload))
	}
}

func TestAppWorkspaceActivationUsesOnlyDeliveredRowFreeShell(t *testing.T) {
	entries := make([]map[string]any, 12_000)
	for index := range entries {
		entries[index] = map[string]any{
			"index": index, "entryId": "entry", "name": "large.dll",
		}
	}
	workspaceScene := func(screen int, shellID, panelPrefix string) map[string]any {
		left := incrementalTestPanel(0, entries)
		right := incrementalTestPanel(1, entries)
		left["id"] = panelPrefix + "-left"
		right["id"] = panelPrefix + "-right"
		left["path"] = "C:/" + panelPrefix + "/left"
		right["path"] = "C:/" + panelPrefix + "/right"
		return map[string]any{
			"type": "scene", "schema": "app", "version": 4,
			"width": 120, "height": 40, "activeScreen": screen,
			"workspaceCount": 2,
			"workspaceTabs": map[string]any{"tabs": []map[string]any{
				{"index": 0, "active": screen == 0, "text": "A"},
				{"index": 1, "active": screen == 1, "text": "B"},
			}},
			"shell": map[string]any{
				"id": shellID, "kind": "shell", "mode": "panels",
				"activePanel": 0,
				"panels":      []map[string]any{left, right},
			},
		}
	}
	previous := compactAppSemanticScene(workspaceScene(0, "shell-a", "a"))
	currentScene := compactAppSemanticScene(workspaceScene(1, "shell-b", "b"))
	current := &appIncrementalScene{Scene: currentScene}
	if patch, _, ok := buildAppScenePatch(previous, current); ok {
		t.Fatalf("generic panel patch unexpectedly accepted identity swap: %#v", patch)
	}

	delivered := make(map[string]semanticDeliveredPanelCatalog)
	for _, panel := range semanticPanelsBySide(currentScene) {
		panelID, catalog, ok := semanticDeliveredPanelFromMap(panel)
		if !ok {
			t.Fatalf("invalid test panel: %#v", panel)
		}
		delivered[panelID] = catalog
	}
	patch, ok := buildAppWorkspaceActivationPatch(previous, current, delivered)
	if !ok || patch.Root == nil || patch.Root.Set["shell"] == nil {
		t.Fatalf("delivered workspace activation rejected: ok=%v patch=%#v", ok, patch)
	}
	if semanticInt(patch.Root.Set["activeScreen"]) != 1 {
		t.Fatalf("activation lost active screen: %#v", patch.Root.Set)
	}
	if semanticValueContainsKey(patch.ToMap(), "entries") {
		t.Fatal("workspace activation leaked cached catalog entries")
	}
	payload, err := msgpack.Marshal(patch.ToMap())
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 4096 {
		t.Fatalf("workspace activation is %d bytes; expected a bounded packet", len(payload))
	}

	delete(delivered, "b-right")
	if cacheMiss, ok := buildAppWorkspaceActivationPatch(previous, current, delivered); ok {
		t.Fatalf("cache-miss activation was accepted: %#v", cacheMiss)
	}
}

func TestAppScenePatchSwitchesBetweenPanelsAndEditorWithoutCatalogs(t *testing.T) {
	entries := make([]map[string]any, 10_000)
	for index := range entries {
		entries[index] = map[string]any{
			"index": index, "entryId": "entry", "name": "large.svg",
		}
	}
	panelsScene := map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"width": 120, "height": 40, "activeScreen": 0,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "activePanel": 1,
			"panels": []map[string]any{
				incrementalTestPanel(0, entries),
				incrementalTestPanel(1, entries),
			},
		},
	}
	previousPanels := compactAppSemanticScene(panelsScene)
	editorSurface := map[string]any{
		"id": "editor:music.svg", "kind": "editor", "cursorLine": 0,
		"cursorPos": 0, "rows": []map[string]any{{"text": "<svg/>"}},
	}
	editorScene := semanticShallowMapCopy(previousPanels)
	delete(editorScene, "shell")
	editorScene["surface"] = editorSurface

	enterPatch, acknowledgements, ok := buildAppScenePatch(previousPanels,
		&appIncrementalScene{Scene: editorScene})
	if !ok || len(acknowledgements) != 0 || enterPatch.Root == nil {
		t.Fatalf("panels -> editor transition rejected: ok=%v patch=%#v", ok, enterPatch)
	}
	if enterPatch.Root.Set["surface"] == nil ||
		!incrementalTestContainsString(enterPatch.Root.Clear, "shell") {
		t.Fatalf("panels -> editor patch has wrong root shape: %#v", enterPatch.Root)
	}
	if semanticValueContainsKey(enterPatch.ToMap(), "entries") {
		t.Fatal("panels -> editor patch leaked hidden panel entries")
	}

	returnScene := compactAppSemanticScene(panelsScene)
	returnPatch, acknowledgements, ok := buildAppScenePatch(editorScene,
		&appIncrementalScene{Scene: returnScene})
	if !ok || len(acknowledgements) != 0 || returnPatch.Root == nil {
		t.Fatalf("editor -> panels transition rejected: ok=%v patch=%#v", ok, returnPatch)
	}
	if returnPatch.Root.Set["shell"] == nil ||
		!incrementalTestContainsString(returnPatch.Root.Clear, "surface") {
		t.Fatalf("editor -> panels patch has wrong root shape: %#v", returnPatch.Root)
	}
	if semanticValueContainsKey(returnPatch.ToMap(), "entries") {
		t.Fatal("editor -> panels patch carried a full panel catalog")
	}
}

func TestApplyAppSceneSurfacePatchDoesNotMutatePreviousSnapshot(t *testing.T) {
	previousSurface := map[string]any{
		"id": "editor:music.svg", "kind": "editor",
		"cursorLine": 2, "cursorPos": 3,
		"rows": []map[string]any{{"text": "immutable"}},
	}
	previous := map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"surface": previousSurface,
	}
	next := semanticSceneStructuralMapCopy(previous)
	applyAppScenePatchToSnapshot(next, extui.ScenePatch{
		Surface: &extui.SurfacePatch{
			SurfaceID: "editor:music.svg",
			MapPatch: extui.MapPatch{Set: extui.M{
				"cursorLine": 7, "cursorPos": 11,
			}},
		},
	})
	if semanticInt(previousSurface["cursorLine"]) != 2 ||
		semanticInt(previousSurface["cursorPos"]) != 3 {
		t.Fatalf("surface patch mutated its source snapshot: %#v", previousSurface)
	}
	nextSurface := next["surface"].(map[string]any)
	if semanticInt(nextSurface["cursorLine"]) != 7 ||
		semanticInt(nextSurface["cursorPos"]) != 11 {
		t.Fatalf("surface patch was not applied to the successor: %#v", nextSurface)
	}
	if !reflect.DeepEqual(nextSurface["rows"], previousSurface["rows"]) {
		t.Fatal("surface cursor patch rewrote immutable document rows")
	}
}

func TestAppScenePatchFileInfoSettingUsesCatalogFreePanelState(t *testing.T) {
	basePanel := incrementalTestPanel(0, []map[string]any{{
		"index": 0, "entryId": "entry-0", "name": "one.txt",
	}})
	basePanel["showFileInfo"] = false
	previous := compactAppSemanticScene(map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "activePanel": 0,
			"panels": []map[string]any{basePanel},
		},
	})
	currentPanel := semanticShallowMapCopy(
		semanticPanelsBySide(previous)[0])
	currentPanel["showFileInfo"] = true
	currentScene := semanticShallowMapCopy(previous)
	currentShell := semanticShallowMapCopy(previous["shell"].(map[string]any))
	currentShell["panels"] = []map[string]any{currentPanel}
	currentScene["shell"] = currentShell

	patch, acknowledgements, ok := buildAppScenePatch(previous,
		&appIncrementalScene{Scene: currentScene})
	if !ok || patch.Shell == nil {
		t.Fatalf("file-information transition rejected: ok=%v patch=%#v", ok, patch)
	}
	if len(acknowledgements) != 0 || len(patch.Shell.Panels) != 1 {
		t.Fatalf("unexpected file-information operations: patch=%#v acknowledgements=%#v",
			patch, acknowledgements)
	}
	operation := patch.Shell.Panels[0]
	if operation.Op != "state_update" || operation.State["showFileInfo"] != true {
		t.Fatalf("file-information setting did not use state_update: %#v", operation)
	}
	wire := patch.ToMap()
	if semanticValueContainsKey(wire, "entries") ||
		semanticValueContainsKey(wire, "highlightStyles") {
		t.Fatalf("file-information state leaked catalog data: %#v", wire)
	}
	payload, err := msgpack.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 1024 {
		t.Fatalf("file-information patch is %d bytes; expected a bounded packet", len(payload))
	}
}

func TestAppScenePatchFastFindCloseClearsTransientMatches(t *testing.T) {
	basePanel := incrementalTestPanel(0, []map[string]any{{
		"index": 0, "entryId": "entry-0", "name": "needle.txt",
	}})
	basePanel["fastFind"] = true
	basePanel["fastFindText"] = "needle"
	basePanel["fastFindMatchColor"] = "#c678dd"
	basePanel["fastFindMatches"] = map[string]any{
		"entry-0": map[string]any{"start": 0, "length": 6},
	}
	previous := compactAppSemanticScene(map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "activePanel": 0,
			"panels": []map[string]any{basePanel},
		},
	})
	currentPanel := semanticShallowMapCopy(semanticPanelsBySide(previous)[0])
	currentPanel["fastFind"] = false
	currentPanel["fastFindText"] = ""
	delete(currentPanel, "fastFindMatchColor")
	delete(currentPanel, "fastFindMatches")
	currentScene := semanticShallowMapCopy(previous)
	currentShell := semanticShallowMapCopy(previous["shell"].(map[string]any))
	currentShell["panels"] = []map[string]any{currentPanel}
	currentScene["shell"] = currentShell

	patch, acknowledgements, ok := buildAppScenePatch(previous,
		&appIncrementalScene{Scene: currentScene})
	if !ok || patch.Shell == nil || len(acknowledgements) != 0 {
		t.Fatalf("fast-find close transition rejected: ok=%v patch=%#v acknowledgements=%#v",
			ok, patch, acknowledgements)
	}
	if len(patch.Shell.Panels) != 1 || patch.Shell.Panels[0].Op != "state_update" {
		t.Fatalf("fast-find close operations = %#v", patch.Shell.Panels)
	}
	state := patch.Shell.Panels[0].State
	matches, ok := state["fastFindMatches"].(map[string]any)
	if !ok || len(matches) != 0 {
		t.Fatalf("fast-find close did not carry an empty match map: %#v", state)
	}
	if color, ok := state["fastFindMatchColor"].(string); !ok || color != "" {
		t.Fatalf("fast-find close did not clear match color: %#v", state)
	}
	if _, leaked := state["entries"]; leaked {
		t.Fatalf("fast-find close state carried catalog rows: %#v", state)
	}
}

func TestBuildAppIncrementalSceneSkipsCoveredDesktop(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })
	vtui.FrameManager.Push(vtui.NewDesktop())
	vtui.FrameManager.Push(vtui.NewCenteredDialog(32, 7, " Bounded "))

	projected, ok := BuildAppIncrementalScene(&vtui.SemanticContext{
		Width: 100, Height: 30, ActiveScreen: 0,
	})
	if !ok || projected == nil || projected.Scene == nil {
		t.Fatal("covered structural Desktop disabled the incremental projection")
	}
	dialogs := appMapSlice(projected.Scene["dialogs"])
	if len(dialogs) != 1 || semanticString(dialogs[0]["title"]) != "Bounded" {
		t.Fatalf("projected dialogs = %#v", dialogs)
	}
	if semanticString(dialogs[0]["kind"]) == "fallback" {
		t.Fatalf("covered Desktop leaked into the app scene: %#v", dialogs)
	}
}

func TestExtUiRendererIncrementalReconciliationClearsUnsafeBoundary(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })
	vtui.FrameManager.Push(vtui.NewDesktop())
	vtui.FrameManager.Push(vtui.NewCenteredDialog(32, 7, " Bounded "))

	projected, ok := BuildAppIncrementalScene(&vtui.SemanticContext{
		Width: 100, Height: 30, ActiveScreen: 0,
	})
	if !ok {
		t.Fatal("initial bounded projection was unavailable")
	}
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}}
	renderer.SetSemanticScene(projected.Scene)
	renderer.Flush()
	if _, err := extUiReadMessage(&wire); err != nil {
		t.Fatalf("initial scene was not delivered: %v", err)
	}

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

	root, ok := BuildAppMenuState(nil)
	if !ok {
		t.Fatal("bounded initial root was unavailable")
	}
	entries := make([]map[string]any, 20_000)
	for index := range entries {
		entries[index] = map[string]any{
			"index": index, "entryId": "entry", "name": "large-file-name",
		}
	}
	initial := semanticShallowMapCopy(root)
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
		nativeSemanticSurfaceEnabled: true,
	}
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	if _, err := extUiReadMessage(&wire); err != nil {
		t.Fatalf("initial scene was not delivered: %v", err)
	}

	// A compact panel activation uses its own scalar wire message but still
	// advances the renderer's logical app snapshot. The following menu patch
	// must therefore remain bounded and use the exact scene state Qt owns.
	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(1)
	renderer.EndSemanticSceneUpdate()
	activation, err := extUiReadMessage(&wire)
	if err != nil || activation["type"] != "panel_activation" {
		t.Fatalf("direct activation before menu = %#v, %v", activation, err)
	}
	compactShell, _ := renderer.lastCompactScene["shell"].(map[string]any)
	if got := semanticInt(compactShell["activePanel"]); got != 1 {
		t.Fatalf("compact snapshot active panel = %d, want 1", got)
	}
	compactPanels := appMapSlice(compactShell["panels"])
	if len(compactPanels) != 2 || appBool(compactPanels[0]["active"]) ||
		!appBool(compactPanels[1]["active"]) {
		t.Fatalf("compact snapshot panel activation = %#v", compactPanels)
	}
	if !renderer.ConsumeSemanticSceneExportSuppression() {
		t.Fatal("direct activation did not suppress its full export")
	}

	menu := vtui.NewVMenu(" Drives ")
	menu.AddItem(vtui.MenuItem{
		Text: "Other panel", Icon: "panels-top-left",
	})
	menu.AddItem(vtui.MenuItem{Text: "C: Local", Icon: "hard-drive"})
	menu.SetPosition(10, 5, 35, 9)

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

	opened := assertDirectMenuPatch("open", func() {
		vtui.FrameManager.Push(menu)
	})
	if !semanticValueContainsKey(opened, "icon") {
		t.Fatal("open menu patch lost drive icons")
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
}

func TestAppScenePatchSeparatesSparseSelectionFromPanelState(t *testing.T) {
	basePanel := incrementalTestPanel(0, nil)
	basePanel["selectionRevision"] = int64(5)
	currentPanel := semanticShallowMapCopy(basePanel)
	currentPanel["selectionRevision"] = int64(6)
	currentPanel["selectedCount"] = 1
	previous := map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "activePanel": 0,
			"panels": []map[string]any{basePanel},
		},
	}
	currentScene := map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "activePanel": 0,
			"panels": []map[string]any{currentPanel},
		},
	}
	panel := &FileSystemPanel{
		catalogRevision: 11, selectionRevision: 6,
		semanticSelectionBaseRevision: 5,
		semanticSelectionChanges: map[string]semanticSelectionChange{
			"entry-7": {Index: 7, EntryID: "entry-7", Selected: true},
		},
	}
	patch, acknowledgements, ok := buildAppScenePatch(previous,
		&appIncrementalScene{Scene: currentScene,
			Panels: map[int]*FileSystemPanel{0: panel}})
	if !ok || patch.Shell == nil {
		t.Fatalf("selection transition rejected: ok=%v patch=%#v", ok, patch)
	}
	if len(patch.Shell.Panels) != 2 {
		t.Fatalf("panel operations = %d, want state + selection delta",
			len(patch.Shell.Panels))
	}
	state, selection := patch.Shell.Panels[0], patch.Shell.Panels[1]
	if state.Op != "state_update" || selection.Op != "selection_delta" {
		t.Fatalf("panel operation order = %q, %q", state.Op, selection.Op)
	}
	if _, leaked := state.State["selectionRevision"]; leaked {
		t.Fatal("state_update advanced selection revision before its delta")
	}
	if len(selection.SelectionChanges) != 1 ||
		selection.SelectionChanges[0]["entryId"] != "entry-7" {
		t.Fatalf("unexpected sparse selection payload: %#v",
			selection.SelectionChanges)
	}
	if len(acknowledgements) != 1 || acknowledgements[0].revision != 6 {
		t.Fatalf("selection acknowledgement = %#v", acknowledgements)
	}
}

func TestApplyAppSceneSelectionPatchDoesNotRewriteCatalog(t *testing.T) {
	entry := map[string]any{"entryId": "entry-7", "selected": false}
	original := map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"shell": map[string]any{
			"id": "shell", "kind": "shell",
			"panels": []map[string]any{{
				"id": "panel-0", "kind": "filePanel", "side": 0,
				"catalogRevision": int64(11), "selectionRevision": int64(5),
				"entries": []map[string]any{entry},
			}},
		},
	}
	snapshot := semanticSceneStructuralMapCopy(original)
	applyAppScenePatchToSnapshot(snapshot, extui.ScenePatch{
		Shell: &extui.ShellPatch{Panels: []extui.PanelPatch{{
			Op: "selection_delta", Side: 0, PanelID: "panel-0",
			CatalogRevision: 11, BaseSelection: 5, SelectionRevision: 6,
			SelectionChanges: []extui.M{{
				"index": 0, "entryId": "entry-7", "selected": true,
			}},
		}}},
	})

	panel := appMapSlice(snapshot["shell"].(map[string]any)["panels"])[0]
	if got := semanticInt64(panel["selectionRevision"]); got != 6 {
		t.Fatalf("selection revision = %d, want 6", got)
	}
	if selected := appBool(entry["selected"]); selected {
		t.Fatal("sparse selection mutated the immutable source catalog")
	}
	retained := appMapSlice(panel["entries"])[0]
	if selected := appBool(retained["selected"]); selected {
		t.Fatal("sparse selection rewrote the retained catalog instead of its overlay")
	}
}

func TestFilePanelSelectionJournalKeepsOneSparseChangeForLargeCatalog(t *testing.T) {
	const entryCount = 20_000
	entries := make([]*fileEntry, entryCount)
	staticEntries := make([]extui.FileEntryModel, entryCount)
	for index := range entries {
		entries[index] = &fileEntry{}
	}
	staticEntries[entryCount-1].EntryID = "entry-last"
	panel := &FileSystemPanel{
		entries: entries, catalogRevision: 11, selectionRevision: 5,
		semanticStaticCache: &semanticPanelStaticCache{
			catalogRevision: 11, entries: staticEntries,
		},
	}

	panel.SetItemSelected(entryCount-1, true)
	patch, ok := panel.semanticSelectionPatch(5)
	if !ok || patch.Op != "selection_delta" ||
		patch.BaseSelection != 5 || patch.SelectionRevision != 6 {
		t.Fatalf("selection journal patch = %#v, ok=%v", patch, ok)
	}
	if len(patch.SelectionChanges) != 1 ||
		semanticInt(patch.SelectionChanges[0]["index"]) != entryCount-1 {
		t.Fatalf("selection delta carried %d changes: %#v",
			len(patch.SelectionChanges), patch.SelectionChanges)
	}
}
