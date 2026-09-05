package main

import (
	"bytes"
	"reflect"
	"testing"
)

func productionPanelCatalogLegacy(path string, catalogRevision, metadataRevision int64,
	provisional bool, entryName string,
) map[string]any {
	panel := func(id string, side int, panelPath string, active bool,
		catalog, metadata int64, panelProvisional bool, name string,
	) map[string]any {
		return map[string]any{
			"id": id, "kind": "filePanel", "side": side, "active": active,
			"path": panelPath, "title": panelPath,
			"sourceKind": "local", "previewCapable": true,
			"catalogRevision": catalog, "selectionRevision": int64(1),
			"metadataDeferred": true, "metadataRevision": metadata,
			"highlightRevision": int64(1),
			"highlightStyles": map[string]any{
				"style-" + name: map[string]any{"marker": name},
			},
			"cursor": 0, "cursorEntryId": id + ":" + name,
			"loading": true, "catalogProvisional": panelProvisional,
			"sortModeName": "name", "sortReverse": false,
			"selectedCount": 0, "totalCount": 1,
			"entries": []map[string]any{{
				"index": 0, "entryId": id + ":" + name, "name": name,
				"displayBaseName": name, "displayExtension": "",
				"path":  panelPath + `\` + name,
				"isDir": true, "isUp": name == "..", "isImage": false,
				"selected": false,
			}},
		}
	}
	left := panel("left", 0, path, true, catalogRevision, metadataRevision, provisional, entryName)
	right := panel("right", 1, `D:\right`, false, 3, 4, false, "right")
	shell := map[string]any{
		"id": "shell", "kind": "panels", "mode": "panels",
		"title": "Panels: " + path, "activePanel": 0,
		"showPanels": true, "showLeftPanel": true, "showRightPanel": true,
		"terminalActive": false, "terminalBusy": false,
		"panels": []map[string]any{left, right},
		"commandLine": map[string]any{
			"id": "command-line", "kind": "commandLine", "visible": true,
			"promptRuns": []map[string]any{{"text": path + ">"}},
			"runs":       []map[string]any{{"text": path + ">"}},
			"inputX":     len(path) + 1, "cursorX": len(path) + 1,
			"cursorPrefixRuns": []map[string]any{{"text": path + ">"}},
			"cursorVisible":    true, "cursorShape": "block",
		},
		"terminal": map[string]any{
			"id": "terminal", "kind": "terminal", "title": "Shell",
			"visible": false, "focused": false, "altScreen": false, "busy": false,
			"cursorX": 4, "cursorY": 2,
			"rows": []map[string]any{{"index": 0, "runs": []map[string]any{{"text": "prompt"}}}},
		},
	}
	workspaceTabs := map[string]any{
		"id": "workspace-tabs", "kind": "tablist", "visible": true,
		"tabs": []map[string]any{{
			"id": "workspace-tab-1", "index": 0, "active": true,
			"text": path + " - right", "tooltipPrimary": path,
			"tooltipSecondary": `D:\right`, "x": 0, "y": 0, "w": 20, "h": 1,
		}},
		"newTab": map[string]any{"id": "workspace-new", "x": 20, "y": 0, "w": 1, "h": 1},
	}
	screen := map[string]any{
		"index": 0, "active": true, "title": "Panels: " + path,
		"frames": []map[string]any{shell},
	}
	return map[string]any{
		"type": "scene", "version": 2, "width": 120, "height": 40,
		"activeScreen": 0, "workspaceCount": 1,
		"workspaceTabs": workspaceTabs,
		"frames":        []map[string]any{shell}, "screens": []map[string]any{screen},
	}
}

func TestSemanticPanelCatalogPatchAcceptsProductionAliasesAndMetadataOnlyPhase(t *testing.T) {
	previous := BuildAppSceneFromLegacy(nil,
		productionPanelCatalogLegacy(`D:\Code\f4`, 10, 20, false, "old"))
	placeholder := BuildAppSceneFromLegacy(nil,
		productionPanelCatalogLegacy(`D:\Code\f4\plugins`, 11, 21, true, ".."))
	previousPanels, previousPanelsOK := semanticScenePanelMaps(previous)
	placeholderPanels, placeholderPanelsOK := semanticScenePanelMaps(placeholder)
	if !previousPanelsOK || !placeholderPanelsOK || len(previousPanels) != 2 || len(placeholderPanels) != 2 {
		t.Fatalf("production panels missing: previous=%#v placeholder=%#v", previous["shell"], placeholder["shell"])
	}
	if !semanticPanelCatalogTransitionSafe(previousPanels[0], placeholderPanels[0], 0) {
		t.Fatalf("typed panel transition rejected\nprevious=%#v\ncurrent=%#v", previousPanels[0], placeholderPanels[0])
	}
	previousShell := previous["shell"].(map[string]any)
	placeholderShell := placeholder["shell"].(map[string]any)
	if !semanticMapEqualExceptKeys(previousShell["commandLine"].(map[string]any),
		placeholderShell["commandLine"].(map[string]any), semanticCommandLineNavigationKeys) {
		t.Fatalf("command-line transition rejected\nprevious=%#v\ncurrent=%#v",
			previousShell["commandLine"], placeholderShell["commandLine"])
	}
	if !semanticWorkspaceTabsNavigationEquivalent(previous["workspaceTabs"],
		placeholder["workspaceTabs"], 0, 0) {
		t.Fatalf("workspace-tab transition rejected\nprevious=%#v\ncurrent=%#v",
			previous["workspaceTabs"], placeholder["workspaceTabs"])
	}
	if !semanticMapEqualExceptKeys(previousShell, placeholderShell, map[string]struct{}{
		"panels": {}, "commandLine": {}, "title": {},
	}) {
		t.Fatalf("shell transition rejected\nprevious=%#v\ncurrent=%#v", previousShell, placeholderShell)
	}
	if !semanticMapEqualExceptKeys(previous, placeholder, map[string]struct{}{
		"shell": {}, "workspaceTabs": {}, "menus": {}, "legacy": {},
		"frames": {}, "screens": {}, "benchmarkTraceId": {}, "benchmark": {},
	}) {
		t.Fatalf("root transition rejected\nprevious=%#v\ncurrent=%#v", previous, placeholder)
	}
	diff := semanticPanelCatalogDiff{
		activePanel: 0, activeScreen: 0, side: 0, panelID: "left", shellID: "shell",
		commandLineID: "command-line", commandLineDiff: true, shellTitleDiff: true,
		workspaceTabsDiff: true,
	}
	for _, key := range []string{"legacy", "frames", "screens"} {
		left := semanticScrubPanelCatalogAlias(previous[key], diff)
		right := semanticScrubPanelCatalogAlias(placeholder[key], diff)
		if !reflect.DeepEqual(left, right) {
			t.Fatalf("production alias %q rejected\nprevious=%#v\ncurrent=%#v", key, left, right)
		}
	}
	patch, ok := semanticPanelCatalogPatch(previous, placeholder)
	if !ok {
		t.Fatal("production placeholder scene rejected by panel_catalog fast path")
	}
	if patch["type"] != "panel_catalog" || patch["shellTitle"] != `Panels: D:\Code\f4\plugins` {
		t.Fatalf("unexpected production placeholder patch: %#v", patch)
	}

	metadata := BuildAppSceneFromLegacy(nil,
		productionPanelCatalogLegacy(`D:\Code\f4\plugins`, 11, 22, true, ".."))
	patch, ok = semanticPanelCatalogPatch(placeholder, metadata)
	if !ok {
		t.Fatal("production metadata-only scene rejected by panel_catalog fast path")
	}
	if panel := patch["panel"].(map[string]any); panel["metadataRevision"] != int64(22) {
		t.Fatalf("metadata revision missing from patch: %#v", panel)
	}
}

func productionPanelCatalogTerminal(scene map[string]any) map[string]any {
	frames, _ := scene["frames"].([]map[string]any)
	if len(frames) == 0 {
		return nil
	}
	terminal, _ := frames[0]["terminal"].(map[string]any)
	return terminal
}

func TestSemanticPanelCatalogPatchDefersOnlyCoveredTerminalPresentation(t *testing.T) {
	previousLegacy := productionPanelCatalogLegacy(`D:\Code\f4`, 10, 20, false, "old")
	currentLegacy := productionPanelCatalogLegacy(`D:\Code\f4\plugins`, 11, 21, false, "new")
	terminal := productionPanelCatalogTerminal(currentLegacy)
	terminal["cursorX"] = 19
	terminal["cursorY"] = 3
	terminal["rows"] = []map[string]any{{
		"index": 0,
		"runs":  []map[string]any{{"text": `D:\Code\f4\plugins>`}},
	}}

	previous := BuildAppSceneFromLegacy(nil, previousLegacy)
	current := BuildAppSceneFromLegacy(nil, currentLegacy)
	patch, ok := semanticPanelCatalogPatch(previous, current)
	if !ok {
		t.Fatal("covered terminal prompt update rejected a plain panel catalog transition")
	}
	if patch["type"] != "panel_catalog" {
		t.Fatalf("unexpected compact message: %#v", patch)
	}
}

func TestExtUiRenderer_HiddenTerminalLogicalChangesForceFullScene(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "mode",
			mutate: func(scene map[string]any) {
				frames := scene["frames"].([]map[string]any)
				frames[0]["mode"] = "terminal"
			},
		},
		{
			name: "alt screen",
			mutate: func(scene map[string]any) {
				productionPanelCatalogTerminal(scene)["altScreen"] = true
			},
		},
		{
			name: "busy",
			mutate: func(scene map[string]any) {
				productionPanelCatalogTerminal(scene)["busy"] = true
			},
		},
		{
			name: "terminal backdrop visible",
			mutate: func(scene map[string]any) {
				frames := scene["frames"].([]map[string]any)
				frames[0]["showRightPanel"] = false
				productionPanelCatalogTerminal(scene)["cursorX"] = 19
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var wire bytes.Buffer
			renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}}
			previous := BuildAppSceneFromLegacy(nil,
				productionPanelCatalogLegacy(`D:\Code\f4`, 10, 20, false, "old"))
			renderer.SetSemanticScene(previous)
			renderer.Flush()
			extUiDrainBufferedMessages(t, &wire)

			currentLegacy := productionPanelCatalogLegacy(
				`D:\Code\f4\plugins`, 11, 21, false, "new")
			tc.mutate(currentLegacy)
			current := BuildAppSceneFromLegacy(nil, currentLegacy)
			renderer.SetSemanticScene(current)
			renderer.Flush()
			messages := extUiDrainBufferedMessages(t, &wire)
			foundShell := false
			for _, message := range messages {
				if message["type"] == "scene" {
					t.Fatalf("logical terminal change sent a whole scene: %#v", message)
				}
				foundShell = foundShell || message["type"] == "shell_snapshot"
			}
			if !foundShell {
				t.Fatalf("logical terminal correction omitted the shell stream: %#v", messages)
			}
		})
	}
}

func TestExtUiRenderer_CoveredTerminalPromptUpdateIsDeferredUntilReveal(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}}
	initial := BuildAppSceneFromLegacy(nil,
		productionPanelCatalogLegacy(`D:\Code\f4`, 10, 20, false, "old"))
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	extUiDrainBufferedMessages(t, &wire)

	covered := BuildAppSceneFromLegacy(nil,
		productionPanelCatalogLegacy(`D:\Code\f4`, 10, 20, false, "old"))
	terminal := covered["shell"].(map[string]any)["terminal"].(map[string]any)
	terminal["cursorX"] = 27
	terminal["rows"] = []map[string]any{{
		"index": 0,
		"runs":  []map[string]any{{"text": `D:\Code\f4>`}},
	}}
	renderer.SetSemanticScene(covered)
	renderer.Flush()
	if wire.Len() != 0 {
		t.Fatalf("covered terminal presentation crossed the wire: %d bytes", wire.Len())
	}

	revealedLegacy := productionPanelCatalogLegacy(`D:\Code\f4`, 10, 20, false, "old")
	frames := revealedLegacy["frames"].([]map[string]any)
	frames[0]["showRightPanel"] = false
	revealedTerminal := productionPanelCatalogTerminal(revealedLegacy)
	revealedTerminal["cursorX"] = 27
	revealedTerminal["rows"] = terminal["rows"]
	renderer.SetSemanticScene(BuildAppSceneFromLegacy(nil, revealedLegacy))
	renderer.Flush()
	messages := extUiDrainBufferedMessages(t, &wire)
	foundShell := false
	for _, message := range messages {
		if message["type"] == "scene" {
			t.Fatalf("terminal reveal sent a whole scene: %#v", message)
		}
		foundShell = foundShell || message["type"] == "shell_snapshot"
	}
	if !foundShell {
		t.Fatalf("terminal reveal omitted the shell stream: %#v", messages)
	}
}

func TestExtUiRenderer_DirectPanelActivationAdoptsTracedRenderedCommandLine(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}}
	initial := panelActivationFastPathScene(0, `Panels: C:\left`)
	initial["benchmarkTraceId"] = "trace:before"
	initial["benchmark"] = map[string]any{
		"benchmarkTraceId": "trace:before", "phase": "before",
		"phaseSequence": int64(1), "sceneSequence": int64(7),
	}
	initial["shell"].(map[string]any)["commandLine"] = map[string]any{
		"id": "command-line", "kind": "commandLine",
		"promptRuns": []map[string]any{{"text": `C:\left>`}},
		"text":       "", "inputX": 8, "cursorPosition": 0,
		"runs":             []map[string]any{{"text": `C:\left>`}},
		"cursorPrefixRuns": []map[string]any{{"text": `C:\left>`}},
		"cursorX":          8, "cursorVisible": true, "cursorShape": "block",
	}
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	extUiDrainBufferedMessages(t, &wire)

	projectedCommandLine := map[string]any{
		"id": "command-line", "kind": "commandLine",
		"promptRuns": []map[string]any{{"text": `D:\right>`}},
		"text":       "", "inputX": 9, "cursorPosition": 0,
		// QueuePanelActivationState runs before Show. These values therefore
		// still describe the previous cell render.
		"runs":             []map[string]any{{"text": `C:\left>`}},
		"cursorPrefixRuns": []map[string]any{{"text": `C:\left>`}},
		"cursorX":          8, "cursorVisible": true, "cursorShape": "block",
	}
	renderer.QueuePanelActivationState(1, `Panels: D:\right`, projectedCommandLine)
	activation, err := extUiReadMessage(&wire)
	if err != nil || activation["type"] != "panel_activation" {
		t.Fatalf("direct activation was not sent: %#v, %v", activation, err)
	}

	authoritative := panelActivationFastPathScene(1, `Panels: D:\right`)
	authoritative["benchmarkTraceId"] = "trace:tab"
	authoritative["benchmark"] = map[string]any{
		"benchmarkTraceId": "trace:tab", "phase": "tab-dispatch",
		"phaseSequence": int64(1),
	}
	authoritativeCommandLine := map[string]any{
		"id": "command-line", "kind": "commandLine",
		"promptRuns": []map[string]any{{"text": `D:\right>`}},
		"text":       "", "inputX": 9, "cursorPosition": 0,
		"runs":             []map[string]any{{"text": `D:\right>`}},
		"cursorPrefixRuns": []map[string]any{{"text": `D:\right>`}},
		"cursorX":          9, "cursorVisible": true, "cursorShape": "block",
	}
	authoritative["shell"].(map[string]any)["commandLine"] = authoritativeCommandLine
	var moveHiddenTerminalCursor func(any)
	moveHiddenTerminalCursor = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if typed["kind"] == "terminal" && typed["id"] == "shell-0:terminal" {
				typed["cursorX"] = 12
				typed["cursorY"] = 1
				return
			}
			for _, nested := range typed {
				moveHiddenTerminalCursor(nested)
			}
		case []map[string]any:
			for _, nested := range typed {
				moveHiddenTerminalCursor(nested)
			}
		case []any:
			for _, nested := range typed {
				moveHiddenTerminalCursor(nested)
			}
		}
	}
	moveHiddenTerminalCursor(authoritative)
	renderer.SetSemanticScene(authoritative)
	renderer.Flush()
	if wire.Len() != 0 {
		t.Fatalf("authoritative post-Tab scene was redundantly sent (%d bytes)", wire.Len())
	}
	if !reflect.DeepEqual(renderer.lastScene, authoritative) {
		t.Fatal("renderer did not adopt the authoritative traced post-Tab scene")
	}
	if renderer.panelActivationProjected {
		t.Fatal("direct-activation projection remained pending after authoritative export")
	}
}

func TestExtUiRenderer_TraceAnnotationsDoNotChangeLogicalScene(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}}
	first := panelActivationFastPathScene(0, "Panels: left")
	first["benchmarkTraceId"] = "trace:first"
	first["benchmark"] = map[string]any{
		"benchmarkTraceId": "trace:first", "phase": "cached",
		"phaseSequence": int64(1), "sceneSequence": int64(10),
	}
	renderer.SetSemanticScene(first)
	renderer.Flush()
	extUiDrainBufferedMessages(t, &wire)

	second := panelActivationFastPathScene(0, "Panels: left")
	second["benchmarkTraceId"] = "trace:second"
	second["benchmark"] = map[string]any{
		"benchmarkTraceId": "trace:second", "phase": "fresh",
		"phaseSequence": int64(2),
	}
	renderer.SetSemanticScene(second)
	renderer.Flush()
	if wire.Len() != 0 {
		t.Fatalf("trace-only redraw emitted %d bytes", wire.Len())
	}
	if !reflect.DeepEqual(renderer.lastScene, second) {
		t.Fatal("trace-only redraw did not refresh the renderer snapshot")
	}
}
