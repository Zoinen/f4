package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

// appIncrementalScene is a row-free authoritative projection plus the live
// panels which own its revisioned catalogs. It is intentionally application
// specific: generic vtui renderers retain ExportSemanticScene unchanged.
type appIncrementalScene struct {
	Scene  map[string]any
	Panels map[int]*FileSystemPanel
}

func appSemanticRootFromHeader(header map[string]any) extui.Scene {
	scene := extui.Scene{
		Width:          semanticInt(header["width"]),
		Height:         semanticInt(header["height"]),
		ActiveScreen:   semanticInt(header["activeScreen"]),
		WorkspaceCount: semanticInt(header["workspaceCount"]),
		WorkspaceTabs:  appQueueAwareWorkspaceTabs(header),
		Presentation:   string(parseGuiPresentationMode(string(AppConfig.GuiPresentation))),
		QmlIconSet:     string(parseQmlIconSetMode(string(AppConfig.QmlIconSet))),
	}
	if menu := appMap(header["menuBar"]); menu != nil {
		model := appMenuFromLegacy(menu, "menuBar")
		scene.MenuBar = &model
	}
	if keyBar := appMap(header["keyBar"]); keyBar != nil {
		model := appKeyBarFromLegacy(keyBar)
		scene.KeyBar = &model
	}
	if toast := appMap(header["toast"]); toast != nil {
		scene.Toast = &extui.ToastModel{Message: semanticString(toast["message"])}
	}
	return scene
}

func (pf *PanelsFrame) semanticIncrementalShell(ctx *vtui.SemanticContext) (extui.ShellModel, map[int]*FileSystemPanel, bool) {
	shell := extui.ShellModel{
		ID:             vtui.SemanticID(pf),
		Title:          strings.TrimSpace(pf.GetTitle()),
		Mode:           "panels",
		ActivePanel:    pf.activeIdx,
		ShowPanels:     pf.showPanels,
		ShowLeftPanel:  pf.showLeftPanel,
		ShowRightPanel: pf.showRightPanel,
		Wide:           pf.wide,
		WidePanel:      pf.widePanel,
		PanelLayout:    pf.semanticPanelLayoutModel(ctx),
		ShowKeyBar:     pf.showKeyBar,
		TerminalBusy:   pf.isPtyBusy(),
		TerminalActive: !pf.showPanels,
	}
	if !pf.showPanels {
		shell.Mode = "terminal"
	}
	panels := make(map[int]*FileSystemPanel)
	for side, panel := range pf.panels {
		if fsp, ok := panel.(*FileSystemPanel); ok {
			header, valid := fsp.semanticPanelHeaderModel(ctx, side, side == pf.activeIdx)
			if !valid {
				return extui.ShellModel{}, nil, false
			}
			shell.Panels = append(shell.Panels, header)
			panels[side] = fsp
		}
		if info, ok := pf.altPanels[side].(*InfoPanel); ok {
			shell.InfoPanels = append(shell.InfoPanels, info.semanticModel(side, side == pf.activeIdx))
		}
		if quick, ok := pf.altPanels[side].(*QuickViewPanel); ok {
			sourceSide := -1
			for candidate, source := range pf.panels {
				if source == quick.Source() {
					sourceSide = candidate
					break
				}
			}
			shell.QuickViews = append(shell.QuickViews,
				quick.semanticModel(side, sourceSide, side == pf.activeIdx))
		}
	}
	if pf.cmdLine != nil {
		shell.CommandLine = pf.cmdLine.semanticModel(ctx)
	}
	if pf.termView != nil {
		shell.Terminal = pf.termView.semanticModel(ctx)
	}
	if MacroMgr != nil && MacroMgr.Recording {
		shell.MacroRecording = true
	}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		shell.Fallback = true
		shell.FallbackReason = reason
	}
	return shell, panels, true
}

// BuildAppIncrementalScene builds every native app section without visiting a
// file catalog. Document/terminal rows are viewport-bounded; popup and dialog
// models are bounded by their own controls.
func BuildAppIncrementalScene(ctx *vtui.SemanticContext) (*appIncrementalScene, bool) {
	if vtui.FrameManager == nil {
		navigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
			"reason", "no_frame_manager")
		return nil, false
	}
	header := vtui.FrameManager.ExportSemanticSceneHeader()
	if header == nil {
		navigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
			"reason", "no_scene_header")
		return nil, false
	}
	if ctx == nil {
		ctx = &vtui.SemanticContext{
			Width: semanticInt(header["width"]), Height: semanticInt(header["height"]),
			ActiveScreen: semanticInt(header["activeScreen"]),
		}
	}
	autocompletes := appActiveAutocompleteMenus()
	vmenus := appActiveVMenus()
	var elements map[string]vtui.UIElement
	scene := appSemanticRootFromHeader(header)

	result := &appIncrementalScene{Panels: make(map[int]*FileSystemPanel)}
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	for _, frame := range frames {
		// Every vtui screen owns a structural Desktop at the bottom of its
		// stack. It is not an application surface when another frame covers it,
		// and the complete exporter/app adapter likewise does not expose that
		// fallback beside commander panels, documents, queues or dialogs. Keep a
		// lone Desktop unsupported so an actually empty screen still takes the
		// conservative full-scene/cell-grid path.
		if frame.GetType() == vtui.TypeDesktop && len(frames) > 1 {
			continue
		}
		if menu, _ := appFrameVMenu(frame); menu != nil {
			continue
		}
		if _, ok := frame.(*vtui.AutoCompleteMenu); ok {
			continue
		}
		if panels, ok := frame.(*PanelsFrame); ok {
			shell, livePanels, valid := panels.semanticIncrementalShell(ctx)
			if !valid {
				navigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
					"reason", "invalid_panel_header",
					"frameType", fmt.Sprintf("%T", frame))
				return nil, false
			}
			scene.Shell = &shell
			for side, panel := range livePanels {
				result.Panels[side] = panel
			}
			continue
		}
		provider, ok := frame.(vtui.SemanticProvider)
		if !ok {
			navigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
				"reason", "unsupported_frame",
				"frameType", fmt.Sprintf("%T", frame))
			return nil, false
		}
		node := provider.SemanticNode(ctx)
		switch semanticString(node["kind"]) {
		case "dialog", "window":
			if elements == nil {
				elements = appActiveSemanticElements()
			}
			appEnrichLegacyTextWidgets(node, elements)
			scene.Dialogs = append(scene.Dialogs, appDialogFromLegacy(node))
		case "viewer", "editor", "terminal":
			model := appSurfaceFromLegacy(node)
			scene.Surface = &model
		case "operationsQueue":
			model := appOperationsQueueFromLegacy(node)
			model.WorkspaceIndex = scene.ActiveScreen
			appBindOperationsQueueWorkspace(&model, scene.WorkspaceTabs)
			scene.OperationsQueue = &model
		default:
			navigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
				"reason", "unsupported_node_kind", "kind", semanticString(node["kind"]),
				"frameType", fmt.Sprintf("%T", frame))
			return nil, false
		}
	}
	for _, menu := range vmenus {
		scene.Menus = append(scene.Menus, menu.model())
	}
	appAppendAutocompleteMenus(&scene, autocompletes)
	result.Scene = compactAppSemanticScene(scene.ToMap())
	return result, true
}

var compactAppRootKeys = []string{
	"type", "schema", "version", "width", "height", "activeScreen",
	"workspaceCount", "workspaceTabs", "presentation", "qmlIconSet",
	"menuBar", "keyBar", "toast", "dialogs", "menus", "surface",
	"operationsQueue",
}

var incrementalRootPatchKeys = []string{
	"width", "height", "activeScreen", "workspaceCount", "workspaceTabs",
	"presentation", "qmlIconSet", "menuBar", "keyBar", "toast", "dialogs",
	"menus", "surface", "operationsQueue",
}

// semanticMenuStateRootPatchKeys is the complete root state which can change
// while a popup owns input. It is deliberately bounded: no frame provider,
// shell, panel header, file entry, editor document, or terminal row is visited.
var semanticMenuStateRootPatchKeys = []string{
	"width", "height", "activeScreen", "workspaceCount", "workspaceTabs",
	"presentation", "qmlIconSet", "menuBar", "keyBar", "toast", "menus",
}

// BuildAppMenuState builds the authoritative popup/global-chrome projection
// used by a transaction which has already proved that an input changed only
// menu presentation. Unlike BuildAppIncrementalScene, this path cannot fail
// because a large panel catalog/header cache is between revisions.
func BuildAppMenuState(_ *vtui.SemanticContext) (map[string]any, bool) {
	if vtui.FrameManager == nil {
		return nil, false
	}
	header := vtui.FrameManager.ExportSemanticSceneHeader()
	if header == nil {
		return nil, false
	}
	scene := appSemanticRootFromHeader(header)
	for _, menu := range appActiveVMenus() {
		scene.Menus = append(scene.Menus, menu.model())
	}
	appAppendAutocompleteMenus(&scene, appActiveAutocompleteMenus())

	complete := scene.ToMap()
	projection := make(map[string]any, len(semanticMenuStateRootPatchKeys))
	for _, key := range semanticMenuStateRootPatchKeys {
		if value, present := complete[key]; present {
			projection[key] = value
		}
	}
	return projection, true
}

var incrementalShellPatchKeys = []string{
	"id", "kind", "title", "mode", "activePanel", "showPanels",
	"showLeftPanel", "showRightPanel", "wide", "widePanel", "showKeyBar",
	"terminalBusy", "terminalActive", "macroRecording", "fallback", "reason",
	"infoPanels", "quickViews", "commandLine", "terminal",
}

func compactAppSemanticScene(scene map[string]any) map[string]any {
	if scene == nil {
		return nil
	}
	out := make(map[string]any, len(compactAppRootKeys)+1)
	for _, key := range compactAppRootKeys {
		if value, present := scene[key]; present {
			out[key] = value
		}
	}
	if source, ok := scene["shell"].(map[string]any); ok && source != nil {
		shell := make(map[string]any, len(source))
		for key, value := range source {
			if key == "panels" {
				continue
			}
			shell[key] = value
		}
		panels := appMapSlice(source["panels"])
		rowFree := make([]map[string]any, 0, len(panels))
		for _, panel := range panels {
			copyPanel := make(map[string]any, len(panel))
			for key, value := range panel {
				switch key {
				case "entries", "highlightStyles":
					continue
				default:
					copyPanel[key] = value
				}
			}
			rowFree = append(rowFree, copyPanel)
		}
		shell["panels"] = rowFree
		out["shell"] = shell
	}
	return out
}

func semanticPatchChangedKeys(previous, current map[string]any, keys []string) (set map[string]any, clear []string) {
	for _, key := range keys {
		previousValue, previousPresent := previous[key]
		currentValue, currentPresent := current[key]
		if previousPresent == currentPresent && reflect.DeepEqual(previousValue, currentValue) {
			continue
		}
		if currentPresent {
			if set == nil {
				set = make(map[string]any)
			}
			set[key] = currentValue
		} else {
			clear = append(clear, key)
		}
	}
	sort.Strings(clear)
	return set, clear
}

func semanticPanelsBySide(scene map[string]any) map[int]map[string]any {
	result := make(map[int]map[string]any)
	shell, _ := scene["shell"].(map[string]any)
	for index, panel := range appMapSlice(shell["panels"]) {
		side := semanticInt(panel["side"])
		if _, present := panel["side"]; !present {
			side = index
		}
		result[side] = panel
	}
	return result
}

type semanticSelectionAcknowledgement struct {
	panel    *FileSystemPanel
	revision int64
}

func semanticPanelStateForPatch(panel map[string]any) map[string]any {
	if panel == nil {
		return nil
	}
	state := make(map[string]any, len(panel))
	for key, value := range panel {
		// Selection revisions are advanced by selection_delta/replace. Keeping
		// them out of state_update makes the two operations independently
		// ordered and lets the frontend reject a stale delta atomically.
		if key == "selectionRevision" {
			continue
		}
		state[key] = value
	}
	return state
}

// buildAppScenePatch compares only bounded projections. Catalog changes are
// intentionally declined here until their complete minimal panel payload has
// been supplied by QueuePanelCatalogState; the ordinary full exporter remains
// the conservative fallback for unsupported transitions during migration.
func buildAppScenePatch(previous map[string]any, current *appIncrementalScene) (extui.ScenePatch, []semanticSelectionAcknowledgement, bool) {
	if previous == nil || current == nil || current.Scene == nil {
		navigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
			"reason", "missing_projection")
		return extui.ScenePatch{}, nil, false
	}
	rootSet, rootClear := semanticPatchChangedKeys(previous, current.Scene, incrementalRootPatchKeys)
	patch := extui.ScenePatch{}
	if len(rootSet) > 0 || len(rootClear) > 0 {
		patch.Root = &extui.MapPatch{Set: rootSet, Clear: rootClear}
	}

	previousShell, previousHasShell := previous["shell"].(map[string]any)
	currentShell, currentHasShell := current.Scene["shell"].(map[string]any)
	if previousHasShell != currentHasShell {
		navigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
			"reason", "shell_presence_changed",
			"previousHasShell", previousHasShell,
			"currentHasShell", currentHasShell)
		return extui.ScenePatch{}, nil, false
	}
	var acknowledgements []semanticSelectionAcknowledgement
	if currentHasShell {
		shellSet, shellClear := semanticPatchChangedKeys(previousShell, currentShell, incrementalShellPatchKeys)
		shellPatch := &extui.ShellPatch{MapPatch: extui.MapPatch{Set: shellSet, Clear: shellClear}}
		previousPanels := semanticPanelsBySide(previous)
		currentPanels := semanticPanelsBySide(current.Scene)
		if len(previousPanels) != len(currentPanels) {
			navigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
				"reason", "panel_count_changed",
				"previousPanels", len(previousPanels),
				"currentPanels", len(currentPanels))
			return extui.ScenePatch{}, nil, false
		}
		for side, panel := range currentPanels {
			before, present := previousPanels[side]
			if !present {
				navigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
					"reason", "panel_side_missing", "side", side)
				return extui.ScenePatch{}, nil, false
			}
			previousPanelID := semanticString(before["id"])
			currentPanelID := semanticString(panel["id"])
			if previousPanelID != currentPanelID {
				navigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
					"reason", "panel_id_changed", "side", side,
					"previousPanelId", previousPanelID,
					"currentPanelId", currentPanelID)
				return extui.ScenePatch{}, nil, false
			}
			previousCatalogRevision := semanticInt64(before["catalogRevision"])
			currentCatalogRevision := semanticInt64(panel["catalogRevision"])
			if previousCatalogRevision != currentCatalogRevision {
				navigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
					"reason", "catalog_revision_changed", "side", side,
					"previousCatalogRevision", previousCatalogRevision,
					"currentCatalogRevision", currentCatalogRevision)
				return extui.ScenePatch{}, nil, false
			}
			beforeState := semanticPanelStateForPatch(before)
			panelState := semanticPanelStateForPatch(panel)
			if !reflect.DeepEqual(beforeState, panelState) {
				shellPatch.Panels = append(shellPatch.Panels, extui.PanelPatch{
					Op: "state_update", Side: side,
					PanelID:         semanticString(panel["id"]),
					CatalogRevision: semanticInt64(panel["catalogRevision"]),
					State:           panelState,
				})
			}
			beforeSelection := semanticInt64(before["selectionRevision"])
			currentSelection := semanticInt64(panel["selectionRevision"])
			if beforeSelection != currentSelection {
				live := current.Panels[side]
				selection, ok := live.semanticSelectionPatch(beforeSelection)
				if !ok || selection.SelectionRevision != currentSelection {
					navigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
						"reason", "selection_journal_mismatch", "side", side,
						"beforeSelection", beforeSelection,
						"currentSelection", currentSelection,
						"patchSelection", selection.SelectionRevision,
						"journalAvailable", ok)
					return extui.ScenePatch{}, nil, false
				}
				selection.Side = side
				shellPatch.Panels = append(shellPatch.Panels, selection)
				acknowledgements = append(acknowledgements,
					semanticSelectionAcknowledgement{panel: live, revision: currentSelection})
			}
		}
		if len(shellPatch.Set) > 0 || len(shellPatch.Clear) > 0 || len(shellPatch.Panels) > 0 {
			patch.Shell = shellPatch
		}
	}
	return patch, acknowledgements, true
}

func scenePatchEmpty(patch extui.ScenePatch) bool {
	return patch.Root == nil && patch.Shell == nil
}

func applyMapPatch(target map[string]any, patch *extui.MapPatch) {
	if target == nil || patch == nil {
		return
	}
	for key, value := range patch.Set {
		target[key] = value
	}
	for _, key := range patch.Clear {
		delete(target, key)
	}
}

// semanticSceneStructuralCopy creates copy-on-write containers for every
// scene alias while sharing immutable heavy values such as catalog entries,
// highlight styles and terminal rows. Incremental application must never
// mutate the exporter-owned scene passed to SetSemanticScene.
func semanticSceneStructuralCopy(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copyMap := make(map[string]any, len(typed))
		for key, nested := range typed {
			switch key {
			case "shell", "panels", "frames", "screens", "legacy":
				copyMap[key] = semanticSceneStructuralCopy(nested)
			default:
				copyMap[key] = nested
			}
		}
		return copyMap
	case []map[string]any:
		copySlice := make([]map[string]any, len(typed))
		for index, nested := range typed {
			copySlice[index], _ = semanticSceneStructuralCopy(nested).(map[string]any)
		}
		return copySlice
	case []any:
		copySlice := make([]any, len(typed))
		for index, nested := range typed {
			copySlice[index] = semanticSceneStructuralCopy(nested)
		}
		return copySlice
	default:
		return value
	}
}

func semanticSceneStructuralMapCopy(scene map[string]any) map[string]any {
	copyScene, _ := semanticSceneStructuralCopy(scene).(map[string]any)
	return copyScene
}

// semanticReplacePanelCatalogAliases updates only structural app/legacy
// containers. It deliberately never descends into entry/style/row payloads;
// every alias shares the one replacement catalog map.
func semanticReplacePanelCatalogAliases(value any, panelID string, side int, replacement map[string]any) any {
	switch typed := value.(type) {
	case map[string]any:
		if semanticString(typed["kind"]) == "filePanel" &&
			semanticString(typed["id"]) == panelID {
			return replacement
		}
		for _, key := range []string{"shell", "panels", "frames", "screens", "legacy"} {
			if nested, present := typed[key]; present {
				typed[key] = semanticReplacePanelCatalogAliases(nested, panelID, side, replacement)
			}
		}
		return typed
	case []map[string]any:
		for index, nested := range typed {
			typed[index], _ = semanticReplacePanelCatalogAliases(
				nested, panelID, side, replacement).(map[string]any)
		}
		return typed
	case []any:
		for index, nested := range typed {
			typed[index] = semanticReplacePanelCatalogAliases(nested, panelID, side, replacement)
		}
		return typed
	default:
		return value
	}
}

// applyAppScenePatchToSnapshot keeps the renderer's complete Go-side snapshot
// coherent while sharing immutable catalog slices. Selection lives in a
// revisioned overlay, so neither sparse deltas nor replacement snapshots
// rewrite entries in the retained catalog.
func applyAppScenePatchToSnapshot(scene map[string]any, patch extui.ScenePatch) {
	if scene == nil {
		return
	}
	applyMapPatch(scene, patch.Root)
	if patch.Shell == nil {
		return
	}
	shell, _ := scene["shell"].(map[string]any)
	if shell == nil {
		return
	}
	applyMapPatch(shell, &patch.Shell.MapPatch)
	panels := appMapSlice(shell["panels"])
	for _, operation := range patch.Shell.Panels {
		if operation.Side < 0 || operation.Side >= len(panels) {
			continue
		}
		panel := panels[operation.Side]
		switch operation.Op {
		case "state_update":
			for key, value := range operation.State {
				if key != "entries" && key != "highlightStyles" {
					panel[key] = value
				}
			}
		case "catalog_replace":
			if operation.Panel != nil {
				panels[operation.Side] = operation.Panel
			}
		case "selection_delta", "selection_replace":
			panel["selectionRevision"] = operation.SelectionRevision
		}
	}
	shell["panels"] = panels
	scene["shell"] = shell
	for _, operation := range patch.Shell.Panels {
		if operation.Op == "catalog_replace" && operation.Panel != nil {
			semanticReplacePanelCatalogAliases(
				scene, operation.PanelID, operation.Side, operation.Panel)
		}
	}
}
