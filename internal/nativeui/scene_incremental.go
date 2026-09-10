package nativeui

import (
	"fmt"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
	"reflect"
	"sort"
	"strings"
)

type panelSelectionJournal interface {
	SemanticSelectionPatch(int64) (extui.PanelPatch, bool)
	AcknowledgeSemanticSelection(int64)
}

type appIncrementalScene struct {
	Scene  map[string]any
	Panels map[int]panelSelectionJournal
}

func appSemanticRootFromHeader(header map[string]any) extui.Scene {
	scene := extui.Scene{
		Width:          semantic.Int(header["width"]),
		Height:         semantic.Int(header["height"]),
		ActiveScreen:   semantic.Int(header["activeScreen"]),
		WorkspaceCount: semantic.Int(header["workspaceCount"]),
		WorkspaceTabs:  appQueueAwareWorkspaceTabs(header),
		Presentation:   string(config.ParseGuiPresentationMode(string(config.App.GuiPresentation))),
		QmlIconSet:     string(config.ParseQmlIconSetMode(string(config.App.QmlIconSet))),
	}
	if menu := semantic.AppMap(header["menuBar"]); menu != nil {
		model := appMenuFromLegacy(menu, "menuBar")
		scene.MenuBar = &model
	}
	if keyBar := semantic.AppMap(header["keyBar"]); keyBar != nil {
		model := appKeyBarFromLegacy(keyBar)
		scene.KeyBar = &model
	}
	if toast := semantic.AppMap(header["toast"]); toast != nil {
		scene.Toast = &extui.ToastModel{Message: semantic.String(toast["message"])}
	}
	return scene
}

func BuildAppIncrementalScene(ctx *vtui.SemanticContext) (*appIncrementalScene, bool) {
	totalStarted := navtrace.SemanticIncrementalStageStart()
	if vtui.FrameManager == nil {
		navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
			"reason", "no_frame_manager")
		return nil, false
	}
	started := navtrace.SemanticIncrementalStageStart()
	header := vtui.FrameManager.ExportSemanticSceneHeader()
	navtrace.SemanticIncrementalStageDone("projection.scene_header", started)
	if header == nil {
		navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
			"reason", "no_scene_header")
		return nil, false
	}
	if ctx == nil {
		ctx = &vtui.SemanticContext{
			Width: semantic.Int(header["width"]), Height: semantic.Int(header["height"]),
			ActiveScreen: semantic.Int(header["activeScreen"]),
		}
	}
	started = navtrace.SemanticIncrementalStageStart()
	autocompletes := appActiveAutocompleteMenus()
	vmenus := appActiveVMenus()
	navtrace.SemanticIncrementalStageDone("projection.active_menus", started)
	var elements map[string]vtui.UIElement
	started = navtrace.SemanticIncrementalStageStart()
	scene := appSemanticRootFromHeader(header)
	navtrace.SemanticIncrementalStageDone("projection.root_header", started)

	result := &appIncrementalScene{Panels: make(map[int]panelSelectionJournal)}
	started = navtrace.SemanticIncrementalStageStart()
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	navtrace.SemanticIncrementalStageDone("projection.active_frames", started, "frameCount", len(frames))
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
		if menu, _ := FrameVMenu(frame); menu != nil {
			continue
		}
		if _, ok := frame.(*vtui.AutoCompleteMenu); ok {
			continue
		}
		if panels, ok := frame.(*panel.PanelsFrame); ok {
			started = navtrace.SemanticIncrementalStageStart()
			shell, livePanels, valid := panels.SemanticIncrementalShell(ctx)
			navtrace.SemanticIncrementalStageDone("projection.panels_shell", started)
			if !valid {
				navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
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
			navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
				"reason", "unsupported_frame",
				"frameType", fmt.Sprintf("%T", frame))
			return nil, false
		}
		started = navtrace.SemanticIncrementalStageStart()
		node := provider.SemanticNode(ctx)
		navtrace.SemanticIncrementalStageDone("projection.surface_node", started,
			"frameType", fmt.Sprintf("%T", frame))
		switch semantic.String(node["kind"]) {
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
			navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.projection_rejected",
				"reason", "unsupported_node_kind", "kind", semantic.String(node["kind"]),
				"frameType", fmt.Sprintf("%T", frame))
			return nil, false
		}
	}
	started = navtrace.SemanticIncrementalStageStart()
	for _, menu := range vmenus {
		scene.Menus = append(scene.Menus, menu.model())
	}
	appAppendAutocompleteMenus(&scene, autocompletes)
	navtrace.SemanticIncrementalStageDone("projection.menu_models", started)
	started = navtrace.SemanticIncrementalStageStart()
	if scene.OperationsQueue == nil {
		scene.OperationsQueue = fileops.BackgroundOperationsQueue()
	}
	result.Scene = semantic.CompactAppSemanticScene(scene.ToMap())
	navtrace.SemanticIncrementalStageDone("projection.compact", started)
	navtrace.SemanticIncrementalStageDone("projection.total", totalStarted)
	return result, true
}

var incrementalRootPatchKeys = []string{
	"width", "height", "activeScreen", "workspaceCount", "workspaceTabs",
	"presentation", "qmlIconSet", "menuBar", "keyBar", "toast", "dialogs",
	"menus", "surface", "operationsQueue",
}

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
	projection := make(map[string]any, len(semantic.SemanticMenuStateRootPatchKeys))
	for _, key := range semantic.SemanticMenuStateRootPatchKeys {
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
	"infoPanels", "quickViews", "commandLine", "terminal", "panelLayout",
}

type semanticSelectionAcknowledgement struct {
	Panel    panelSelectionJournal
	Revision int64
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
		// Retained catalog ranges likewise belong only to panel_catalog.
		if key == "selectionRevision" || key == "catalogDelta" {
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

func BuildAppScenePatch(previous map[string]any, current *appIncrementalScene) (extui.ScenePatch, []semanticSelectionAcknowledgement, bool) {
	if previous == nil || current == nil || current.Scene == nil {
		navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
			"reason", "missing_projection")
		return extui.ScenePatch{}, nil, false
	}
	rootSet, rootClear := semantic.SemanticPatchChangedKeys(previous, current.Scene, incrementalRootPatchKeys)
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
		navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.structural_patch",
			"rootSetKeys", strings.Join(rootSetKeys, ","),
			"rootClearKeys", strings.Join(patch.Root.Clear, ","))
		return patch, nil, true
	}
	var acknowledgements []semanticSelectionAcknowledgement
	if currentHasShell {
		shellSet, shellClear := semantic.SemanticPatchChangedKeys(previousShell, currentShell, incrementalShellPatchKeys)
		shellPatch := &extui.ShellPatch{MapPatch: extui.MapPatch{Set: shellSet, Clear: shellClear}}
		previousPanels := semantic.SemanticPanelsBySide(previous)
		currentPanels := semantic.SemanticPanelsBySide(current.Scene)
		if len(previousPanels) != len(currentPanels) {
			navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
				"reason", "panel_count_changed",
				"previousPanels", len(previousPanels),
				"currentPanels", len(currentPanels))
			return extui.ScenePatch{}, nil, false
		}
		for side, panel := range currentPanels {
			before, present := previousPanels[side]
			if !present {
				navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
					"reason", "panel_side_missing", "side", side)
				return extui.ScenePatch{}, nil, false
			}
			previousPanelID := semantic.String(before["id"])
			currentPanelID := semantic.String(panel["id"])
			if previousPanelID != currentPanelID {
				navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
					"reason", "panel_id_changed", "side", side,
					"previousPanelId", previousPanelID,
					"currentPanelId", currentPanelID)
				return extui.ScenePatch{}, nil, false
			}
			previousCatalogRevision := semantic.Int64(before["catalogRevision"])
			currentCatalogRevision := semantic.Int64(panel["catalogRevision"])
			if previousCatalogRevision != currentCatalogRevision {
				navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
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
				navtrace.NavigationBenchmarkIncrementalEvent(
					"scene.incremental.panel_state_delta",
					"side", side,
					"keys", strings.Join(deltaKeys, ","),
					"fieldCount", len(deltaKeys),
					"rootSetKeys", strings.Join(rootSetKeys, ","),
					"shellSetKeys", strings.Join(shellSetKeys, ","),
					"galleryLayoutMode",
					semantic.String(panelStateDelta["galleryLayoutMode"]),
					"galleryDensity",
					semantic.Int(panelStateDelta["galleryDensity"]))
				shellPatch.Panels = append(shellPatch.Panels, extui.PanelPatch{
					Op: "state_update", Side: side,
					PanelID:         semantic.String(panel["id"]),
					CatalogRevision: semantic.Int64(panel["catalogRevision"]),
					State:           panelStateDelta,
				})
			}
			beforeSelection := semantic.Int64(before["selectionRevision"])
			currentSelection := semantic.Int64(panel["selectionRevision"])
			if beforeSelection != currentSelection {
				live := current.Panels[side]
				if live == nil {
					return extui.ScenePatch{}, nil, false
				}
				selection, ok := live.SemanticSelectionPatch(beforeSelection)
				if !ok || selection.SelectionRevision != currentSelection {
					navtrace.NavigationBenchmarkIncrementalEvent("scene.incremental.patch_rejected",
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
					semanticSelectionAcknowledgement{Panel: live, Revision: currentSelection})
			}
		}
		if len(shellPatch.Set) > 0 || len(shellPatch.Clear) > 0 || len(shellPatch.Panels) > 0 {
			patch.Shell = shellPatch
		}
	}
	return patch, acknowledgements, true
}
