package app

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
