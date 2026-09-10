package nativeui

import (
	semantic "github.com/unxed/f4/internal/semantic"
	extui "github.com/unxed/f4/sdk/extui"
	vtui "github.com/unxed/vtui"
	msgpack "github.com/vmihailenco/msgpack/v5"
	reflect "reflect"
	testing "testing"
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

func TestSemanticNormalizeSparsePanelCatalogsMarksEveryAlias(t *testing.T) {
	entries := []map[string]any{{
		"index": 0, "entryId": "entry-0", "name": "..", "isDir": true,
	}}
	panel := incrementalTestPanel(0, entries)
	panel["totalCount"] = 30_001
	alias := incrementalTestPanel(0, entries)
	alias["totalCount"] = 30_001
	scene := map[string]any{
		"type": "scene",
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "activePanel": 0,
			"panels": []map[string]any{panel},
		},
		"frames": []map[string]any{{
			"id": "shell", "kind": "shell", "panels": []map[string]any{alias},
		}},
		"legacy": map[string]any{
			"frames": []map[string]any{{
				"id": "shell", "kind": "shell", "panels": []map[string]any{alias},
			}},
		},
	}

	normalized := semantic.SemanticNormalizeSparsePanelCatalogs(scene)
	if normalized == nil {
		t.Fatal("sparse normalization returned no scene")
	}
	normalized["normalizationProbe"] = true
	if _, present := scene["normalizationProbe"]; present {
		t.Fatal("sparse normalization reused the caller scene map")
	}
	delete(normalized, "normalizationProbe")
	if _, present := panel["catalogRowsDeferred"]; present {
		t.Fatal("sparse normalization mutated the source panel")
	}
	var checked int
	var inspect func(any)
	inspect = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if semantic.String(typed["kind"]) == "filePanel" &&
				semantic.String(typed["id"]) == "panel-0" {
				checked++
				if typed["catalogRowsDeferred"] != true {
					t.Fatalf("panel alias was not normalized: %#v", typed)
				}
				if got := len(semantic.AppMapSlice(typed["entries"])); got != 1 {
					t.Fatalf("normalization expanded the bounded page to %d rows", got)
				}
			}
			for _, key := range []string{"shell", "panels", "frames", "screens", "legacy"} {
				inspect(typed[key])
			}
		case []map[string]any:
			for _, nested := range typed {
				inspect(nested)
			}
		case []any:
			for _, nested := range typed {
				inspect(nested)
			}
		}
	}
	inspect(normalized)
	if checked != 3 {
		t.Fatalf("checked %d normalized aliases, want 3", checked)
	}
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
	previous := semantic.CompactAppSemanticScene(full)
	currentScene := semantic.SemanticShallowMapCopy(previous)
	currentScene["shell"] = semantic.SemanticShallowMapCopy(
		previous["shell"].(map[string]any))
	currentScene["menus"] = []map[string]any{{
		"id": "drive-menu", "role": "popup", "title": "Drives",
		"items": []map[string]any{
			{"index": 0, "text": "Other panel", "icon": "panels-top-left"},
			{"index": 1, "text": "C: Local", "icon": "hard-drive"},
		},
	}}

	patch, _, ok := BuildAppScenePatch(previous, &appIncrementalScene{
		Scene: currentScene, Panels: map[int]panelSelectionJournal{},
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
	previousPanels := semantic.CompactAppSemanticScene(panelsScene)
	editorSurface := map[string]any{
		"id": "editor:music.svg", "kind": "editor", "cursorLine": 0,
		"cursorPos": 0, "rows": []map[string]any{{"text": "<svg/>"}},
	}
	editorScene := semantic.SemanticShallowMapCopy(previousPanels)
	delete(editorScene, "shell")
	editorScene["surface"] = editorSurface

	enterPatch, acknowledgements, ok := BuildAppScenePatch(previousPanels,
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

	returnScene := semantic.CompactAppSemanticScene(panelsScene)
	returnPatch, acknowledgements, ok := BuildAppScenePatch(editorScene,
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
	next := semantic.SemanticSceneStructuralMapCopy(previous)
	semantic.ApplyAppScenePatchToSnapshot(next, extui.ScenePatch{
		Surface: &extui.SurfacePatch{
			SurfaceID: "editor:music.svg",
			MapPatch: extui.MapPatch{Set: extui.M{
				"cursorLine": 7, "cursorPos": 11,
			}},
		},
	})
	if semantic.Int(previousSurface["cursorLine"]) != 2 ||
		semantic.Int(previousSurface["cursorPos"]) != 3 {
		t.Fatalf("surface patch mutated its source snapshot: %#v", previousSurface)
	}
	nextSurface := next["surface"].(map[string]any)
	if semantic.Int(nextSurface["cursorLine"]) != 7 ||
		semantic.Int(nextSurface["cursorPos"]) != 11 {
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
	previous := semantic.CompactAppSemanticScene(map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "activePanel": 0,
			"panels": []map[string]any{basePanel},
		},
	})
	currentPanel := semantic.SemanticShallowMapCopy(
		semantic.SemanticPanelsBySide(previous)[0])
	currentPanel["showFileInfo"] = true
	currentPanel["catalogDelta"] = map[string]any{
		"baseCatalogRevision": 1, "oldTotalCount": 1,
		"ranges": []map[string]any{{"oldIndex": 0, "index": 0, "count": 1}},
	}
	currentScene := semantic.SemanticShallowMapCopy(previous)
	currentShell := semantic.SemanticShallowMapCopy(previous["shell"].(map[string]any))
	currentShell["panels"] = []map[string]any{currentPanel}
	currentScene["shell"] = currentShell

	patch, acknowledgements, ok := BuildAppScenePatch(previous,
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
	for _, unrelated := range []string{
		"path", "title", "cursor", "cursorEntryId", "galleryLayoutMode",
		"galleryColumnCount", "galleryDensity", "galleryDensities", "galleryColumns",
		"selectedCount", "totalCount",
	} {
		if _, present := operation.State[unrelated]; present {
			t.Fatalf("file-information delta carried unchanged %q: %#v",
				unrelated, operation.State)
		}
	}
	for _, identity := range semanticPanelStateIdentityKeys {
		if _, present := operation.State[identity]; !present {
			t.Fatalf("file-information delta omitted identity %q: %#v",
				identity, operation.State)
		}
	}
	wire := patch.ToMap()
	if semanticValueContainsKey(wire, "entries") || semanticValueContainsKey(wire, "catalogDelta") ||
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

func TestAppScenePatchGalleryLayoutUsesSmallStateDelta(t *testing.T) {
	basePanel := incrementalTestPanel(0, nil)
	basePanel["path"] = "C:/WINDOWS/system32"
	basePanel["title"] = "system32"
	basePanel["galleryLayoutMode"] = "details"
	basePanel["galleryColumnCount"] = 2
	basePanel["galleryDensity"] = 22
	basePanel["galleryDensities"] = map[string]any{
		"masonry": 150, "grid": 160, "icons": 64,
	}
	basePanel["galleryLayoutRevision"] = int64(17)
	basePanel["galleryColumns"] = []map[string]any{
		{"id": "name", "role": "name", "title": "Name", "width": 50},
		{"id": "size", "role": "size", "title": "Size", "width": 14},
	}
	basePanel["fastFind"] = false
	basePanel["fastFindText"] = ""
	basePanel["fastFindMatches"] = map[string]any{}
	basePanel["fastFindMatchColor"] = ""
	previous := semantic.CompactAppSemanticScene(map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "activePanel": 0,
			"panels": []map[string]any{basePanel},
		},
	})
	currentPanel := semantic.SemanticShallowMapCopy(semantic.SemanticPanelsBySide(previous)[0])
	currentPanel["galleryLayoutMode"] = "icons"
	currentPanel["galleryDensity"] = 64
	currentPanel["galleryLayoutRevision"] = int64(18)
	currentScene := semantic.SemanticShallowMapCopy(previous)
	currentShell := semantic.SemanticShallowMapCopy(previous["shell"].(map[string]any))
	currentShell["panels"] = []map[string]any{currentPanel}
	currentScene["shell"] = currentShell

	patch, acknowledgements, ok := BuildAppScenePatch(previous,
		&appIncrementalScene{Scene: currentScene})
	if !ok || patch.Shell == nil || len(acknowledgements) != 0 ||
		len(patch.Shell.Panels) != 1 {
		t.Fatalf("gallery-layout transition rejected: ok=%v patch=%#v acknowledgements=%#v",
			ok, patch, acknowledgements)
	}
	state := patch.Shell.Panels[0].State
	for _, changed := range []string{
		"galleryLayoutMode", "galleryDensity", "galleryLayoutRevision",
	} {
		if _, present := state[changed]; !present {
			t.Fatalf("gallery-layout delta omitted %q: %#v", changed, state)
		}
	}
	for _, unchanged := range []string{
		"path", "title", "cursor", "cursorEntryId", "galleryColumnCount",
		"galleryDensities", "galleryColumns", "fastFind", "fastFindText", "fastFindMatches",
		"fastFindMatchColor", "selectedCount", "totalCount",
	} {
		if _, present := state[unchanged]; present {
			t.Fatalf("gallery-layout delta carried unchanged %q: %#v",
				unchanged, state)
		}
	}
	payload, err := msgpack.Marshal(patch.ToMap())
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 512 {
		t.Fatalf("gallery-layout patch is %d bytes; expected a small delta",
			len(payload))
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
	previous := semantic.CompactAppSemanticScene(map[string]any{
		"type": "scene", "schema": "app", "version": 4,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "activePanel": 0,
			"panels": []map[string]any{basePanel},
		},
	})
	currentPanel := semantic.SemanticShallowMapCopy(semantic.SemanticPanelsBySide(previous)[0])
	currentPanel["fastFind"] = false
	currentPanel["fastFindText"] = ""
	delete(currentPanel, "fastFindMatchColor")
	delete(currentPanel, "fastFindMatches")
	currentScene := semantic.SemanticShallowMapCopy(previous)
	currentShell := semantic.SemanticShallowMapCopy(previous["shell"].(map[string]any))
	currentShell["panels"] = []map[string]any{currentPanel}
	currentScene["shell"] = currentShell

	patch, acknowledgements, ok := BuildAppScenePatch(previous,
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
	dialog := vtui.NewCenteredDialog(32, 7, " Bounded ")
	dialog.AddItem(vtui.NewText(dialog.X1+2, dialog.Y1+2, "Name:", 0))
	dialog.AddItem(vtui.NewEdit(dialog.X1+10, dialog.Y1+2, 16, "Google Drive"))
	dialog.AddItem(vtui.NewButton(dialog.X1+10, dialog.Y2-2, "Authorize"))
	vtui.FrameManager.Push(dialog)

	projected, ok := BuildAppIncrementalScene(&vtui.SemanticContext{
		Width: 100, Height: 30, ActiveScreen: 0,
	})
	if !ok || projected == nil || projected.Scene == nil {
		t.Fatal("covered structural Desktop disabled the incremental projection")
	}
	dialogs := semantic.AppMapSlice(projected.Scene["dialogs"])
	if len(dialogs) != 1 || semantic.String(dialogs[0]["title"]) != "Bounded" {
		t.Fatalf("projected dialogs = %#v", dialogs)
	}
	if semantic.String(dialogs[0]["kind"]) == "fallback" {
		t.Fatalf("covered Desktop leaked into the app scene: %#v", dialogs)
	}
	children := semantic.AppMapSlice(dialogs[0]["children"])
	if len(children) != 3 {
		t.Fatalf("projected dialog controls = %#v", children)
	}
	for _, child := range children {
		if !semantic.AppBool(child["visible"]) {
			t.Fatalf("new dialog control was hidden before first render: %#v", child)
		}
	}
}

func TestAppScenePatchSeparatesSparseSelectionFromPanelState(t *testing.T) {
	basePanel := incrementalTestPanel(0, nil)
	basePanel["selectionRevision"] = int64(5)
	currentPanel := semantic.SemanticShallowMapCopy(basePanel)
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
	panel := &selectionJournalFixture{patch: extui.PanelPatch{Op: "selection_delta", PanelID: "panel-0", CatalogRevision: 11, BaseSelection: 5, SelectionRevision: 6, SelectionChanges: []map[string]any{{"index": 7, "entryId": "entry-7", "selected": true}}}}

	patch, acknowledgements, ok := BuildAppScenePatch(previous,
		&appIncrementalScene{Scene: currentScene,
			Panels: map[int]panelSelectionJournal{0: panel}})
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
	if len(acknowledgements) != 1 || acknowledgements[0].Revision != 6 {
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
	snapshot := semantic.SemanticSceneStructuralMapCopy(original)
	semantic.ApplyAppScenePatchToSnapshot(snapshot, extui.ScenePatch{
		Shell: &extui.ShellPatch{Panels: []extui.PanelPatch{{
			Op: "selection_delta", Side: 0, PanelID: "panel-0",
			CatalogRevision: 11, BaseSelection: 5, SelectionRevision: 6,
			SelectionChanges: []extui.M{{
				"index": 0, "entryId": "entry-7", "selected": true,
			}},
		}}},
	})

	panel := semantic.AppMapSlice(snapshot["shell"].(map[string]any)["panels"])[0]
	if got := semantic.Int64(panel["selectionRevision"]); got != 6 {
		t.Fatalf("selection revision = %d, want 6", got)
	}
	if selected := semantic.AppBool(entry["selected"]); selected {
		t.Fatal("sparse selection mutated the immutable source catalog")
	}
	retained := semantic.AppMapSlice(panel["entries"])[0]
	if selected := semantic.AppBool(retained["selected"]); selected {
		t.Fatal("sparse selection rewrote the retained catalog instead of its overlay")
	}
}

type selectionJournalFixture struct {
	patch extui.PanelPatch
	ack   int64
}

func (j *selectionJournalFixture) SemanticSelectionPatch(base int64) (extui.PanelPatch, bool) {
	return j.patch, base == j.patch.BaseSelection
}
func (j *selectionJournalFixture) AcknowledgeSemanticSelection(revision int64) { j.ack = revision }
