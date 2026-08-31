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

func semanticIncrementalStageStart() int64 {
	if !navigationBenchmarkIsEnabled() {
		return 0
	}
	return navigationBenchmarkMonotonicNs()
}

func semanticIncrementalStageDone(stage string, started int64, fields ...any) {
	if started == 0 {
		return
	}
	fields = append(fields, "stage", stage,
		"durationNs", navigationBenchmarkMonotonicNs()-started)
	navigationBenchmarkUIEvent("scene.incremental.stage", fields...)
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
	totalStarted := semanticIncrementalStageStart()
	started := semanticIncrementalStageStart()
	title := strings.TrimSpace(pf.GetTitle())
	semanticIncrementalStageDone("shell.title", started)
	started = semanticIncrementalStageStart()
	terminalBusy := pf.isPtyBusy()
	semanticIncrementalStageDone("shell.pty_busy", started)
	started = semanticIncrementalStageStart()
	panelLayout := pf.semanticPanelLayoutModel(ctx)
	semanticIncrementalStageDone("shell.panel_layout", started)
	shell := extui.ShellModel{
		ID:             vtui.SemanticID(pf),
		Title:          title,
		Mode:           "panels",
		ActivePanel:    pf.activeIdx,
		ShowPanels:     pf.showPanels,
		ShowLeftPanel:  pf.showLeftPanel,
		ShowRightPanel: pf.showRightPanel,
		Wide:           pf.wide,
		WidePanel:      pf.widePanel,
		PanelLayout:    panelLayout,
		ShowKeyBar:     pf.showKeyBar,
		TerminalBusy:   terminalBusy,
		TerminalActive: !pf.showPanels,
	}
	if !pf.showPanels {
		shell.Mode = "terminal"
	}
	panels := make(map[int]*FileSystemPanel)
	for side, panel := range pf.panels {
		if fsp, ok := panel.(*FileSystemPanel); ok {
			started = semanticIncrementalStageStart()
			header, valid := fsp.semanticPanelHeaderModel(ctx, side, side == pf.activeIdx)
			semanticIncrementalStageDone("shell.panel_header", started, "side", side)
			if !valid {
				semanticIncrementalStageDone("shell.total", totalStarted, "valid", false)
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
		started = semanticIncrementalStageStart()
		shell.CommandLine = pf.cmdLine.semanticModel(ctx)
		semanticIncrementalStageDone("shell.command_line", started)
	}
	if pf.termView != nil {
		started = semanticIncrementalStageStart()
		shell.Terminal = pf.termView.semanticModelWithBottomOverlay(
			ctx, terminalCommandLineOverlayRows(shell.CommandLine))
		semanticIncrementalStageDone("shell.terminal", started)
	}
	if MacroMgr != nil && MacroMgr.Recording {
		shell.MacroRecording = true
	}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		shell.Fallback = true
		shell.FallbackReason = reason
	}
	semanticIncrementalStageDone("shell.total", totalStarted, "valid", true)
	return shell, panels, true
}

// BuildAppIncrementalScene builds every native app section without visiting a
// file catalog. Document/terminal rows are viewport-bounded; popup and dialog
// models are bounded by their own controls.
func BuildAppIncrementalScene(ctx *vtui.SemanticContext) (*appIncrementalScene, bool) {
	totalStarted := semanticIncrementalStageStart()
	if vtui.FrameManager == nil {
		navigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
			"reason", "no_frame_manager")
		return nil, false
	}
	started := semanticIncrementalStageStart()
	header := vtui.FrameManager.ExportSemanticSceneHeader()
	semanticIncrementalStageDone("projection.scene_header", started)
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
	started = semanticIncrementalStageStart()
	autocompletes := appActiveAutocompleteMenus()
	vmenus := appActiveVMenus()
	semanticIncrementalStageDone("projection.active_menus", started)
	var elements map[string]vtui.UIElement
	started = semanticIncrementalStageStart()
	scene := appSemanticRootFromHeader(header)
	semanticIncrementalStageDone("projection.root_header", started)

	result := &appIncrementalScene{Panels: make(map[int]*FileSystemPanel)}
	started = semanticIncrementalStageStart()
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	semanticIncrementalStageDone("projection.active_frames", started, "frameCount", len(frames))
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
			started = semanticIncrementalStageStart()
			shell, livePanels, valid := panels.semanticIncrementalShell(ctx)
			semanticIncrementalStageDone("projection.panels_shell", started)
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
		started = semanticIncrementalStageStart()
		node := provider.SemanticNode(ctx)
		semanticIncrementalStageDone("projection.surface_node", started,
			"frameType", fmt.Sprintf("%T", frame))
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
	started = semanticIncrementalStageStart()
	for _, menu := range vmenus {
		scene.Menus = append(scene.Menus, menu.model())
	}
	appAppendAutocompleteMenus(&scene, autocompletes)
	semanticIncrementalStageDone("projection.menu_models", started)
	started = semanticIncrementalStageStart()
	result.Scene = compactAppSemanticScene(scene.ToMap())
	semanticIncrementalStageDone("projection.compact", started)
	semanticIncrementalStageDone("projection.total", totalStarted)
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
	// state_update is merged into the frontend's cached row-free panel. The
	// quick-search fields are transient, so an omitted empty map/color would
	// leave the previous query's highlights in that cache. Keep both fields
	// explicit whenever this is a semantic panel state, including the
	// no-match and closed-search states.
	if _, hasFastFind := panel["fastFind"]; hasFastFind {
		if _, present := state["fastFindMatches"]; !present {
			state["fastFindMatches"] = map[string]any{}
		}
		if _, present := state["fastFindMatchColor"]; !present {
			state["fastFindMatchColor"] = ""
		}
	}
	return state
}

var semanticPanelStateIdentityKeys = [...]string{
	"id", "kind", "side", "catalogRevision",
	"metadataDeferred", "metadataRevision",
}

// semanticPanelStateDeltaForPatch keeps state_update proportional to the
// actual state change. The Qt receiver merges this map into its authoritative
// row-free panel cache; the identity tuple remains present on every update so
// the receiver can reject stale or cross-panel deltas before committing them.
func semanticPanelStateDeltaForPatch(previous, current map[string]any) map[string]any {
	if reflect.DeepEqual(previous, current) {
		return nil
	}
	delta := make(map[string]any)
	for key, currentValue := range current {
		if previousValue, present := previous[key]; present && reflect.DeepEqual(previousValue, currentValue) {
			continue
		}
		delta[key] = currentValue
	}
	for _, key := range semanticPanelStateIdentityKeys {
		delta[key] = current[key]
	}
	return delta
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
		// Switching between commander panels and a standalone document changes
		// the root shape, but not any catalog. Carry the row-free shell as one
		// bounded root value (or clear it on entry) instead of falling back to
		// ExportSemanticScene, which would walk every hidden file entry.
		if patch.Root == nil {
			patch.Root = &extui.MapPatch{}
		}
		if currentHasShell {
			if patch.Root.Set == nil {
				patch.Root.Set = make(map[string]any)
			}
			patch.Root.Set["shell"] = currentShell
		} else {
			patch.Root.Clear = append(patch.Root.Clear, "shell")
			sort.Strings(patch.Root.Clear)
		}
		rootSetKeys := make([]string, 0, len(patch.Root.Set))
		for key := range patch.Root.Set {
			rootSetKeys = append(rootSetKeys, key)
		}
		sort.Strings(rootSetKeys)
		navigationBenchmarkIncrementalEvent("scene.incremental.structural_patch",
			"rootSetKeys", strings.Join(rootSetKeys, ","),
			"rootClearKeys", strings.Join(patch.Root.Clear, ","))
		return patch, nil, true
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
			if panelStateDelta := semanticPanelStateDeltaForPatch(
				beforeState, panelState); panelStateDelta != nil {
				deltaKeys := make([]string, 0, len(panelStateDelta))
				for key := range panelStateDelta {
					deltaKeys = append(deltaKeys, key)
				}
				sort.Strings(deltaKeys)
				rootSetKeys := make([]string, 0, len(rootSet))
				for key := range rootSet {
					rootSetKeys = append(rootSetKeys, key)
				}
				sort.Strings(rootSetKeys)
				shellSetKeys := make([]string, 0, len(shellSet))
				for key := range shellSet {
					shellSetKeys = append(shellSetKeys, key)
				}
				sort.Strings(shellSetKeys)
				navigationBenchmarkIncrementalEvent(
					"scene.incremental.panel_state_delta",
					"side", side,
					"keys", strings.Join(deltaKeys, ","),
					"fieldCount", len(deltaKeys),
					"rootSetKeys", strings.Join(rootSetKeys, ","),
					"shellSetKeys", strings.Join(shellSetKeys, ","),
					"galleryLayoutMode",
					semanticString(panelStateDelta["galleryLayoutMode"]),
					"galleryDensity",
					semanticInt(panelStateDelta["galleryDensity"]))
				shellPatch.Panels = append(shellPatch.Panels, extui.PanelPatch{
					Op: "state_update", Side: side,
					PanelID:         semanticString(panel["id"]),
					CatalogRevision: semanticInt64(panel["catalogRevision"]),
					State:           panelStateDelta,
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
	return patch.Root == nil && patch.Shell == nil && patch.Surface == nil
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
			case "shell", "surface", "panels", "frames", "screens", "legacy":
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

// semanticNormalizeSparsePanelCatalogs repairs the one invariant that the
// paged protocol cannot infer safely on the client: an authoritative catalog
// whose entries are only a prefix of totalCount must explicitly advertise
// that its remaining rows are deferred. Some semantic snapshots can be built
// before the rows-capability handshake is observed, so normalize every scene
// at the renderer boundary. The copy is structural only; entry payloads stay
// shared and no complete catalog is retained or duplicated.
func semanticNormalizeSparsePanelCatalogs(scene map[string]any) map[string]any {
	panels, ok := semanticScenePanelMaps(scene)
	if !ok {
		return scene
	}
	normalized := scene
	copied := false
	for side, panel := range panels {
		if panel == nil || !extUiBool(panel, "metadataDeferred") ||
			extUiBool(panel, "catalogProvisional") ||
			extUiBool(panel, "catalogRowsDeferred") {
			continue
		}
		entries, entriesOK := semanticMapSlice(panel["entries"])
		if !entriesOK {
			continue
		}
		totalCount := extUiAnyInt(panel["totalCount"])
		if totalCount <= len(entries) {
			continue
		}
		if !copied {
			normalized = semanticSceneStructuralMapCopy(scene)
			copied = true
		}
		replacement := make(map[string]any, len(panel)+1)
		for key, value := range panel {
			replacement[key] = value
		}
		replacement["catalogRowsDeferred"] = true
		semanticReplacePanelCatalogAliases(
			normalized, semanticString(panel["id"]), side, replacement)
	}
	return normalized
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
	if patch.Surface != nil {
		surface, _ := scene["surface"].(map[string]any)
		if surface != nil && semanticString(surface["id"]) == patch.Surface.SurfaceID {
			applyMapPatch(surface, &patch.Surface.MapPatch)
			scene["surface"] = surface
		}
	}
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
		case "catalog_append":
			if panel == nil || operation.CatalogOffset != len(appMapSlice(panel["entries"])) {
				continue
			}
			entries := appMapSlice(panel["entries"])
			entries = append(entries, operation.Entries...)
			panel["entries"] = entries
			panel["catalogProvisional"] = !operation.CatalogFinal
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
