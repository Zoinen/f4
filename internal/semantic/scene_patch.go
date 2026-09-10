package semantic

import (
	"github.com/unxed/f4/sdk/extui"
	"reflect"
	"sort"
)

var CompactAppRootKeys = []string{
	"type", "schema", "version", "width", "height", "activeScreen",
	"workspaceCount", "workspaceTabs", "presentation", "qmlIconSet",
	"menuBar", "keyBar", "toast", "dialogs", "menus", "surface",
	"operationsQueue",
}

var SemanticMenuStateRootPatchKeys = []string{
	"width", "height", "activeScreen", "workspaceCount", "workspaceTabs",
	"presentation", "qmlIconSet", "menuBar", "keyBar", "toast", "menus",
}

func CompactAppSemanticScene(scene map[string]any) map[string]any {
	if scene == nil {
		return nil
	}
	out := make(map[string]any, len(CompactAppRootKeys)+1)
	for _, key := range CompactAppRootKeys {
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
		panels := AppMapSlice(source["panels"])
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

func SemanticPatchChangedKeys(previous, current map[string]any, keys []string) (set map[string]any, clear []string) {
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

func SemanticPanelsBySide(scene map[string]any) map[int]map[string]any {
	result := make(map[int]map[string]any)
	shell, _ := scene["shell"].(map[string]any)
	for index, panel := range AppMapSlice(shell["panels"]) {
		side := Int(panel["side"])
		if _, present := panel["side"]; !present {
			side = index
		}
		result[side] = panel
	}
	return result
}

func ScenePatchEmpty(patch extui.ScenePatch) bool {
	return patch.Root == nil && patch.Shell == nil && patch.Surface == nil
}

func ApplyMapPatch(target map[string]any, patch *extui.MapPatch) {
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

func SemanticSceneStructuralCopy(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copyMap := make(map[string]any, len(typed))
		for key, nested := range typed {
			switch key {
			case "shell", "surface", "panels", "frames", "screens", "legacy":
				copyMap[key] = SemanticSceneStructuralCopy(nested)
			default:
				copyMap[key] = nested
			}
		}
		return copyMap
	case []map[string]any:
		copySlice := make([]map[string]any, len(typed))
		for index, nested := range typed {
			copySlice[index], _ = SemanticSceneStructuralCopy(nested).(map[string]any)
		}
		return copySlice
	case []any:
		copySlice := make([]any, len(typed))
		for index, nested := range typed {
			copySlice[index] = SemanticSceneStructuralCopy(nested)
		}
		return copySlice
	default:
		return value
	}
}

func SemanticSceneStructuralMapCopy(scene map[string]any) map[string]any {
	copyScene, _ := SemanticSceneStructuralCopy(scene).(map[string]any)
	return copyScene
}

func SemanticNormalizeSparsePanelCatalogs(scene map[string]any) map[string]any {
	panels, ok := SemanticScenePanelMaps(scene)
	if !ok {
		return scene
	}
	normalized := scene
	copied := false
	for side, panel := range panels {
		if panel == nil || !Bool(panel["metadataDeferred"]) ||
			Bool(panel["catalogProvisional"]) ||
			Bool(panel["catalogRowsDeferred"]) {
			continue
		}
		entries, entriesOK := SemanticMapSlice(panel["entries"])
		if !entriesOK {
			continue
		}
		totalCount := Int(panel["totalCount"])
		if totalCount <= len(entries) {
			continue
		}
		if !copied {
			normalized = SemanticSceneStructuralMapCopy(scene)
			copied = true
		}
		replacement := make(map[string]any, len(panel)+1)
		for key, value := range panel {
			replacement[key] = value
		}
		replacement["catalogRowsDeferred"] = true
		SemanticReplacePanelCatalogAliases(
			normalized, String(panel["id"]), side, replacement)
	}
	return normalized
}

func SemanticReplacePanelCatalogAliases(value any, panelID string, side int, replacement map[string]any) any {
	switch typed := value.(type) {
	case map[string]any:
		if String(typed["kind"]) == "filePanel" &&
			String(typed["id"]) == panelID {
			return replacement
		}
		for _, key := range []string{"shell", "panels", "frames", "screens", "legacy"} {
			if nested, present := typed[key]; present {
				typed[key] = SemanticReplacePanelCatalogAliases(nested, panelID, side, replacement)
			}
		}
		return typed
	case []map[string]any:
		for index, nested := range typed {
			typed[index], _ = SemanticReplacePanelCatalogAliases(
				nested, panelID, side, replacement).(map[string]any)
		}
		return typed
	case []any:
		for index, nested := range typed {
			typed[index] = SemanticReplacePanelCatalogAliases(nested, panelID, side, replacement)
		}
		return typed
	default:
		return value
	}
}

func ApplyAppScenePatchToSnapshot(scene map[string]any, patch extui.ScenePatch) {
	if scene == nil {
		return
	}
	ApplyMapPatch(scene, patch.Root)
	if patch.Surface != nil {
		surface, _ := scene["surface"].(map[string]any)
		if surface != nil && String(surface["id"]) == patch.Surface.SurfaceID {
			ApplyMapPatch(surface, &patch.Surface.MapPatch)
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
	ApplyMapPatch(shell, &patch.Shell.MapPatch)
	panels := AppMapSlice(shell["panels"])
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
			if panel == nil || operation.CatalogOffset != len(AppMapSlice(panel["entries"])) {
				continue
			}
			entries := AppMapSlice(panel["entries"])
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
			SemanticReplacePanelCatalogAliases(
				scene, operation.PanelID, operation.Side, operation.Panel)
		}
	}
}
