package main

import (
	"testing"

	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func TestBuildAppSceneFromLegacyPromotesShellAndKeepsFallback(t *testing.T) {
	oldPresentation := AppConfig.GuiPresentation
	oldIconSet := AppConfig.QmlIconSet
	AppConfig.GuiPresentation = GuiPresentationGUI
	AppConfig.QmlIconSet = QmlIconSetSystem
	defer func() {
		AppConfig.GuiPresentation = oldPresentation
		AppConfig.QmlIconSet = oldIconSet
	}()

	legacy := map[string]any{
		"type":         "scene",
		"version":      1,
		"width":        100,
		"height":       40,
		"activeScreen": 0,
		"workspaceTabs": map[string]any{
			"visible": true,
			"mode":    "always",
			"tabs":    []map[string]any{{"id": "workspace-tab-7", "text": "Project", "action": "workspace.activate"}},
		},
		"frames": []map[string]any{
			{
				"id":          "panels",
				"kind":        "panels",
				"activePanel": 1,
				"showPanels":  true,
				"showKeyBar":  true,
				"panels": []map[string]any{
					{
						"id":            "left",
						"kind":          "filePanel",
						"side":          0,
						"path":          "/left",
						"viewModeName":  "medium",
						"sortModeName":  "name",
						"selectedCount": 0,
						"entries": []map[string]any{
							{
								"index":            0,
								"name":             "archive.tar.gz",
								"displayBaseName":  "archive.tar",
								"displayExtension": "gz",
							},
						},
					},
					{
						"id":           "right",
						"kind":         "filePanel",
						"side":         1,
						"active":       true,
						"path":         "/right",
						"viewModeName": "detailed",
						"sortModeName": "size",
					},
				},
				"commandLine": map[string]any{
					"id":      "cmd",
					"kind":    "commandLine",
					"visible": true,
					"prompt":  ">",
					"text":    "ls",
				},
			},
		},
	}

	scene := BuildAppSceneFromLegacy(&vtui.SemanticContext{Width: 100, Height: 40}, legacy)
	if scene["schema"] != extui.Schema || scene["version"] != extui.SceneVersion {
		t.Fatalf("unexpected app scene envelope: %#v", scene)
	}
	if scene["presentation"] != string(GuiPresentationGUI) {
		t.Fatalf("unexpected presentation mode: %#v", scene["presentation"])
	}
	if scene["qmlIconSet"] != string(QmlIconSetSystem) {
		t.Fatalf("unexpected QML icon set: %#v", scene["qmlIconSet"])
	}
	if scene["legacy"] == nil || scene["frames"] == nil {
		t.Fatalf("scene did not preserve legacy fallback: %#v", scene)
	}
	workspaceTabs := scene["workspaceTabs"].(map[string]any)
	if workspaceTabs["mode"] != "always" || workspaceTabs["visible"] != true {
		t.Fatalf("upstream workspace tab model was not preserved: %#v", workspaceTabs)
	}
	tabs := workspaceTabs["tabs"].([]map[string]any)
	if tabs[0]["id"] != "workspace-tab-7" || tabs[0]["action"] != "workspace.activate" {
		t.Fatalf("workspace tab actions were rewritten: %#v", tabs[0])
	}
	shell := scene["shell"].(map[string]any)
	if shell["kind"] != "shell" || shell["activePanel"] != 1 {
		t.Fatalf("unexpected shell: %#v", shell)
	}
	if shell["commandLine"].(map[string]any)["text"] != "ls" {
		t.Fatalf("typed command line was not promoted: %#v", shell["commandLine"])
	}
	legacyFrames := scene["frames"].([]map[string]any)
	if _, duplicated := legacyFrames[0]["commandLine"]; duplicated {
		t.Fatal("promoted command line was duplicated in the legacy fallback")
	}
	panels := shell["panels"].([]map[string]any)
	if panels[1]["path"] != "/right" || panels[1]["active"] != true {
		t.Fatalf("unexpected panel model: %#v", panels[1])
	}
	if panels[0]["sourceKind"] != "vfs" {
		t.Fatalf("legacy panel did not receive compatible v3 defaults: %#v", panels[0])
	}
	for _, retired := range []string{"presentation", "viewModeName", "columns", "top"} {
		if _, present := panels[0][retired]; present {
			t.Fatalf("retired panel field %q survived promotion: %#v", retired, panels[0])
		}
	}
	if panels[0]["galleryLayoutMode"] != "masonry" ||
		panels[0]["galleryColumnCount"] != 2 ||
		panels[0]["galleryDensity"] != 150 ||
		panels[0]["galleryLayoutRevision"] != int64(1) {
		t.Fatalf("legacy panel did not receive Gallery layout defaults: %#v", panels[0])
	}
	entries := panels[0]["entries"].([]map[string]any)
	if entries[0]["displayBaseName"] != "archive.tar" ||
		entries[0]["displayExtension"] != "gz" {
		t.Fatalf("legacy entry lost its display name fields: %#v", entries[0])
	}
}

func TestBuildAppSceneFromLegacyPreservesDeferredBaseAndStripsHeavyLegacyAlias(t *testing.T) {
	deferredFrame := map[string]any{
		"id": "shell", "kind": "panels", "showPanels": true,
		"panels": []map[string]any{{
			"id": "left", "kind": "filePanel", "path": `D:\work`,
			"catalogRevision": int64(4), "selectionRevision": int64(2),
			"metadataDeferred": true, "metadataRevision": int64(9),
			"highlightRevision": int64(8), "totalSize": int64(100),
			"highlightStyles": map[string]any{"style-a": map[string]any{"marker": "!"}},
			"entries": []map[string]any{{
				"index": 0, "entryId": "entry-a", "name": "a.jpg",
				"displayBaseName": "a", "displayExtension": "jpg",
				"path": `D:\work\a.jpg`, "isDir": false, "isUp": false,
				"isImage": true, "selected": true,
				"localPath": `D:\work\a.jpg`, "size": int64(100),
				"highlightStyleId": "style-a",
			}},
		}},
	}
	legacy := map[string]any{
		"type":   "scene",
		"frames": []map[string]any{deferredFrame},
		"screens": []map[string]any{{
			"index":  0,
			"frames": []map[string]any{deferredFrame},
		}},
	}

	scene := BuildAppSceneFromLegacy(nil, legacy)
	panel := scene["shell"].(map[string]any)["panels"].([]map[string]any)[0]
	if panel["metadataDeferred"] != true || panel["metadataRevision"] != int64(9) {
		t.Fatalf("deferred revision was not promoted: %#v", panel)
	}
	entry := panel["entries"].([]map[string]any)[0]
	if entry["isImage"] != true {
		t.Fatalf("minimal base entry was not promoted: %#v", entry)
	}
	if _, present := entry["path"]; present {
		t.Fatalf("promoted deferred entry retained logical path: %#v", entry)
	}
	if _, present := entry["localPath"]; present {
		t.Fatalf("promoted deferred entry retained heavy metadata: %#v", entry)
	}
	if _, present := panel["highlightStyles"]; present {
		t.Fatalf("promoted deferred panel retained highlights: %#v", panel)
	}

	aliasPanel := scene["frames"].([]map[string]any)[0]["panels"].([]map[string]any)[0]
	if _, present := aliasPanel["entries"]; present {
		t.Fatalf("legacy alias duplicated deferred entries: %#v", aliasPanel)
	}
	if _, present := aliasPanel["highlightStyles"]; present {
		t.Fatalf("legacy alias duplicated deferred highlights: %#v", aliasPanel)
	}
	screenPanel := scene["screens"].([]map[string]any)[0]["frames"].([]map[string]any)[0]["panels"].([]map[string]any)[0]
	if _, present := screenPanel["entries"]; present {
		t.Fatalf("screen alias duplicated deferred entries: %#v", screenPanel)
	}
	legacyPanel := scene["legacy"].(map[string]any)["screens"].([]map[string]any)[0]["frames"].([]map[string]any)[0]["panels"].([]map[string]any)[0]
	if _, present := legacyPanel["entries"]; present {
		t.Fatalf("nested legacy screen duplicated deferred entries: %#v", legacyPanel)
	}
}

func TestBuildAppSceneFromLegacyKeepsShellTerminalInsideShell(t *testing.T) {
	legacy := map[string]any{
		"type":   "scene",
		"width":  100,
		"height": 40,
		"frames": []map[string]any{
			{
				"id":             "panels",
				"kind":           "panels",
				"showPanels":     false,
				"terminalActive": true,
				"terminal": map[string]any{
					"id":    "shell-terminal",
					"kind":  "terminal",
					"title": "ls",
					"busy":  false,
					"rows": []map[string]any{
						{"index": 0, "text": "ls"},
						{"index": 1, "text": "app_model.go"},
					},
				},
			},
		},
	}

	scene := BuildAppSceneFromLegacy(nil, legacy)
	if _, duplicated := scene["surface"]; duplicated {
		t.Fatalf("shell terminal was promoted to a standalone document surface: %#v", scene["surface"])
	}
	shell := scene["shell"].(map[string]any)
	if shell["terminalActive"] != true || shell["mode"] != "terminal" {
		t.Fatalf("shell terminal state was not preserved: %#v", shell)
	}
	terminal := shell["terminal"].(map[string]any)
	if terminal["id"] != "shell-terminal" || terminal["title"] != "ls" {
		t.Fatalf("unexpected shell terminal: %#v", terminal)
	}
	rows := terminal["rows"].([]map[string]any)
	if len(rows) != 2 || rows[0]["text"] != "ls" || rows[1]["text"] != "app_model.go" {
		t.Fatalf("shell terminal rows were not preserved: %#v", rows)
	}
}

func TestBuildAppSceneFromLegacyKeepsTopLevelTerminalAsSurface(t *testing.T) {
	legacy := map[string]any{
		"type": "scene",
		"frames": []map[string]any{
			{
				"id":    "standalone-terminal",
				"kind":  "terminal",
				"title": "Terminal log",
				"rows": []map[string]any{
					{"index": 0, "text": "output"},
				},
			},
		},
	}

	scene := BuildAppSceneFromLegacy(nil, legacy)
	surface := scene["surface"].(map[string]any)
	if surface["id"] != "standalone-terminal" || surface["kind"] != "terminal" {
		t.Fatalf("top-level terminal was not promoted as a standalone surface: %#v", surface)
	}
	rows := surface["rows"].([]map[string]any)
	if len(rows) != 1 || rows[0]["text"] != "output" {
		t.Fatalf("top-level terminal rows were not preserved: %#v", rows)
	}
}

func TestAppWorkspaceTabsExposeTypedLucideIcons(t *testing.T) {
	legacy := map[string]any{
		"workspaceTabs": map[string]any{
			"visible": true,
			"tabs": []map[string]any{
				{"index": 0, "number": 1, "text": "P left ─ right", "closable": true},
				{"index": 1, "number": 2, "text": "T Terminal", "closable": true},
				{"index": 2, "number": 3, "text": "V report.txt", "closable": true},
				{"index": 3, "number": 4, "text": "E notes.md", "closable": true},
				{"index": 4, "number": 5, "text": "Operations Queue", "closable": true},
			},
		},
		"screens": []map[string]any{
			{"number": 1, "frames": []map[string]any{{"kind": "panels", "showPanels": true}}},
			{"number": 2, "frames": []map[string]any{{"kind": "panels", "showPanels": false}}},
			{"number": 3, "frames": []map[string]any{{"kind": "viewer"}}},
			{"number": 4, "frames": []map[string]any{{"kind": "editor"}}},
			{"number": 5, "frames": []map[string]any{{"kind": "operationsQueue", "canClose": false}}},
		},
	}

	tabs := appMapSlice(appQueueAwareWorkspaceTabs(legacy)["tabs"])
	wantKinds := []string{"panels", "terminal", "viewer", "editor", "operationsQueue"}
	wantIcons := []string{"panels-top-left", "square-terminal", "file-text", "file-pen-line", "list-checks"}
	wantTitles := []string{"left ─ right", "Terminal", "report.txt", "notes.md", "Operations Queue"}
	for index := range tabs {
		if got := semanticString(tabs[index]["surfaceKind"]); got != wantKinds[index] {
			t.Fatalf("tab %d surfaceKind = %q, want %q", index, got, wantKinds[index])
		}
		if got := semanticString(tabs[index]["iconName"]); got != wantIcons[index] {
			t.Fatalf("tab %d iconName = %q, want %q", index, got, wantIcons[index])
		}
		if got := semanticString(tabs[index]["text"]); got != wantTitles[index] {
			t.Fatalf("tab %d text = %q, want %q", index, got, wantTitles[index])
		}
		if got := semanticInt(tabs[index]["number"]); got != index+1 {
			t.Fatalf("tab %d number = %d, want %d", index, got, index+1)
		}
	}
	if appBool(tabs[4]["closable"]) {
		t.Fatal("active operations queue tab remained closable")
	}
}

func TestAppWorkspaceTabsPreserveStructuredTitlePrefixes(t *testing.T) {
	legacy := map[string]any{
		"workspaceTabs": map[string]any{
			"visible": true,
			"tabs": []map[string]any{
				{
					"index":  0,
					"number": 1,
					"marker": "P",
					"text":   "P projects ─ Documents",
				},
			},
		},
		"screens": []map[string]any{
			{"number": 1, "frames": []map[string]any{{"kind": "panels", "showPanels": true}}},
		},
	}

	tabs := appMapSlice(appQueueAwareWorkspaceTabs(legacy)["tabs"])
	if len(tabs) != 1 {
		t.Fatalf("workspace tabs = %#v, want one tab", tabs)
	}
	if got := semanticString(tabs[0]["text"]); got != "P projects ─ Documents" {
		t.Fatalf("structured workspace title = %q, want legitimate P prefix preserved", got)
	}
}

func TestAppWorkspaceTabTextRemovesOnlyUpstreamKindMarker(t *testing.T) {
	for input, want := range map[string]string{
		"P":             "",
		"V report.txt":  "report.txt",
		"E notes.md":    "notes.md",
		"T Terminal":    "Terminal",
		"Project files": "Project files",
	} {
		if got := appWorkspaceTabText(input); got != want {
			t.Fatalf("appWorkspaceTabText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildAppSceneExportsTextPresentation(t *testing.T) {
	oldPresentation := AppConfig.GuiPresentation
	AppConfig.GuiPresentation = GuiPresentationText
	defer func() { AppConfig.GuiPresentation = oldPresentation }()

	scene := BuildAppSceneFromLegacy(nil, map[string]any{"type": "scene"})
	if scene["presentation"] != string(GuiPresentationText) {
		t.Fatalf("presentation = %v", scene["presentation"])
	}
}

func TestAppEnrichLegacyTextWidgetsExportsPassiveLabels(t *testing.T) {
	label := vtui.NewText(4, 3, "Proceed &with update?", 0)
	vertical := vtui.NewVText(20, 3, "ABC", 0)
	nested := map[string]any{
		"id":   "group",
		"kind": "group",
		"children": []map[string]any{{
			"id":   vtui.SemanticID(vertical),
			"kind": "widget",
		}},
	}
	frame := map[string]any{
		"kind": "dialog",
		"children": []map[string]any{
			{"id": vtui.SemanticID(label), "kind": "widget"},
			nested,
		},
	}

	appEnrichLegacyTextWidgets(frame, map[string]vtui.UIElement{
		vtui.SemanticID(label):    label,
		vtui.SemanticID(vertical): vertical,
	})

	children := frame["children"].([]map[string]any)
	if children[0]["kind"] != "text" || children[0]["text"] != "Proceed with update?" {
		t.Fatalf("passive text label was not enriched: %#v", children[0])
	}
	if children[0]["hotkey"] != "w" {
		t.Fatalf("passive text label hotkey was not enriched: %#v", children[0])
	}
	nestedChildren := children[1]["children"].([]map[string]any)
	if nestedChildren[0]["kind"] != "text" || nestedChildren[0]["text"] != "ABC" {
		t.Fatalf("nested vertical label was not enriched: %#v", nestedChildren[0])
	}
}

func TestBuildAppSceneFromLegacyPromotesDocumentSurface(t *testing.T) {
	legacy := map[string]any{
		"type":   "scene",
		"width":  80,
		"height": 25,
		"frames": []map[string]any{
			{
				"id":                 "editor",
				"kind":               "editor",
				"defaultBackground":  "#242424",
				"title":              " Editor ",
				"path":               "/tmp/a.txt",
				"baseName":           "a.txt",
				"mode":               "text",
				"cursorVisualRow":    2,
				"cursorVisualColumn": 7,
				"cursorVisible":      true,
				"cursorShape":        "block",
				"scrollUnit":         "rows",
				"windowStart":        int64(18),
				"windowEnd":          int64(48),
				"viewportStart":      int64(20),
				"viewportSpan":       int64(10),
				"contentExtent":      int64(1000),
				"contentExtentKnown": true,
				"viewportRow":        2,
				"viewportRows":       10,
				"cursorAbsoluteRow":  int64(22),
				"windowGeneration":   int64(7),
				"windowContentKey":   "window-key-7",
				"rows": []map[string]any{
					{"index": 0, "runs": []map[string]any{{
						"text": "hello", "attr": uint64(42),
						"foreground": "#123456", "background": "#654321",
						"bold": true, "underline": true, "strikeout": true,
					}}},
				},
				"windowRows": []map[string]any{
					{"index": 0, "visualRow": 18, "offset": int64(180),
						"endOffset": int64(190), "text": "overscan"},
				},
			},
		},
	}

	scene := BuildAppSceneFromLegacy(nil, legacy)
	surface := scene["surface"].(map[string]any)
	if surface["kind"] != "editor" || surface["baseName"] != "a.txt" ||
		surface["defaultBackground"] != "#242424" {
		t.Fatalf("unexpected surface: %#v", surface)
	}
	if surface["cursorVisualRow"] != 2 || surface["cursorVisualColumn"] != 7 ||
		surface["cursorVisible"] != true || surface["cursorShape"] != "block" {
		t.Fatalf("editor cursor was not promoted: %#v", surface)
	}
	if surface["scrollUnit"] != "rows" || surface["windowStart"] != int64(18) ||
		surface["windowEnd"] != int64(48) || surface["viewportStart"] != int64(20) ||
		surface["viewportSpan"] != int64(10) || surface["contentExtent"] != int64(1000) ||
		surface["contentExtentKnown"] != true || surface["viewportRow"] != 2 ||
		surface["viewportRows"] != 10 || surface["cursorAbsoluteRow"] != int64(22) ||
		surface["windowGeneration"] != uint64(7) ||
		surface["windowContentKey"] != "window-key-7" {
		t.Fatalf("window metadata was not promoted: %#v", surface)
	}
	rows := surface["rows"].([]map[string]any)
	runs := rows[0]["runs"].([]map[string]any)
	if runs[0]["text"] != "hello" || runs[0]["foreground"] != "#123456" ||
		runs[0]["background"] != "#654321" || runs[0]["bold"] != true ||
		runs[0]["underline"] != true || runs[0]["strikeout"] != true {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	windowRows := surface["windowRows"].([]map[string]any)
	if len(windowRows) != 1 || windowRows[0]["visualRow"] != 18 ||
		windowRows[0]["offset"] != int64(180) || windowRows[0]["endOffset"] != int64(190) {
		t.Fatalf("window rows were not promoted: %#v", windowRows)
	}
}

func TestAppVMenuModelPreservesNativeMenuState(t *testing.T) {
	menu := vtui.NewVMenu(" &History ")
	menu.SetPosition(12, 4, 51, 12)
	menu.AddItem(vtui.MenuItem{Text: "✓ &First", Shortcut: "F3", Command: 101})
	menu.AddSeparator()
	menu.AddItem(vtui.MenuItem{Text: "&Second", Command: 102})
	menu.SetSelectPos(2)
	menu.TopPos = 1

	model := (appVMenu{frame: menu, menu: menu, bottomHint: " Esc "}).model().ToMap()
	if model["role"] != "vmenu" || model["title"] != " History " {
		t.Fatalf("unexpected VMenu identity: %#v", model)
	}
	if model["x"] != 12 || model["y"] != 4 || model["w"] != 40 || model["h"] != 9 {
		t.Fatalf("unexpected VMenu geometry: %#v", model)
	}
	if model["selected"] != 2 || model["top"] != 1 || model["bottomHint"] != " Esc " {
		t.Fatalf("unexpected VMenu viewport state: %#v", model)
	}
	items := model["items"].([]map[string]any)
	if items[0]["text"] != "First" || items[0]["hotkey"] != "f" || items[0]["shortcut"] != "F3" {
		t.Fatalf("unexpected first VMenu item: %#v", items[0])
	}
	if items[0]["checked"] != true {
		t.Fatalf("checked VMenu item was not normalized: %#v", items[0])
	}
	if items[1]["separator"] != true || items[2]["text"] != "Second" {
		t.Fatalf("unexpected remaining VMenu items: %#v", items)
	}
}

func TestAppAutocompleteStartsWithoutImplicitSelection(t *testing.T) {
	edit := vtui.NewEdit(0, 0, 40, "git st")
	edit.History = []string{"git status", "git stash"}
	menu := vtui.NewAutoCompleteMenu(edit)

	model := (appAutocompleteMenu{
		menu: menu,
		id:   "autocomplete",
		x:    1,
		y:    2,
		w:    40,
		h:    4,
	}).model().ToMap()
	if model["selected"] != -1 {
		t.Fatalf("autocomplete selected = %#v, want -1", model["selected"])
	}
	if model["query"] != "git st" {
		t.Fatalf("autocomplete query = %#v, want exact edit text", model["query"])
	}
	items := model["items"].([]map[string]any)
	if len(items) != 2 || items[0]["text"] != "git status" || items[1]["text"] != "git stash" {
		t.Fatalf("unexpected autocomplete items: %#v", items)
	}
}

func TestAppMenuItemFromLegacyNormalizesConsoleCheckmark(t *testing.T) {
	item := appMenuItemFromLegacy(map[string]any{
		"index":   1,
		"text":    "√Medium",
		"rawText": "√&Medium",
	})
	if item.Text != "Medium" || !item.Checked || item.RawText != "√&Medium" {
		t.Fatalf("legacy menu checkmark was not normalized: %#v", item)
	}
}

func TestAppLegacyWithoutVMenusRemovesOnlyPromotedFrame(t *testing.T) {
	menu := vtui.NewVMenu("Menu")
	menus := appVMenus{{frame: menu, menu: menu}}
	legacy := map[string]any{
		"frames": []map[string]any{
			{"id": "panels", "kind": "panels"},
			{"id": vtui.SemanticID(menu), "kind": "fallback"},
		},
	}

	filtered := appLegacyWithoutVMenus(legacy, menus)
	frames := filtered["frames"].([]map[string]any)
	if len(frames) != 1 || frames[0]["id"] != "panels" {
		t.Fatalf("unexpected filtered frames: %#v", frames)
	}
	if len(legacy["frames"].([]map[string]any)) != 2 {
		t.Fatal("legacy scene was mutated in place")
	}
}

func TestAppDetectsMenuBarSubmenuByAuthoritativeGeometry(t *testing.T) {
	menuBar := vtui.NewMenuBar([]string{"&Files", "&Commands"})
	menuBar.SetPosition(0, 0, 79, 0)
	menuBar.Active = true
	menuBar.SelectPos = 1

	submenu := vtui.NewVMenu("Commands")
	x := menuBar.GetItemX(menuBar.SelectPos)
	submenu.SetPosition(x, 1, x+23, 8)
	if !appIsMenuBarSubmenu(submenu, menuBar) {
		t.Fatal("menu-bar submenu was not detected")
	}

	submenu.MoveRelative(0, 1)
	if appIsMenuBarSubmenu(submenu, menuBar) {
		t.Fatal("standalone menu was mistaken for a menu-bar submenu")
	}
}
