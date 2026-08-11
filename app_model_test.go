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
	panels := shell["panels"].([]map[string]any)
	if panels[1]["path"] != "/right" || panels[1]["active"] != true {
		t.Fatalf("unexpected panel model: %#v", panels[1])
	}
	if panels[0]["presentation"] != "list" || panels[0]["sourceKind"] != "vfs" {
		t.Fatalf("legacy panel did not receive compatible v3 defaults: %#v", panels[0])
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
				"title":              " Editor ",
				"path":               "/tmp/a.txt",
				"baseName":           "a.txt",
				"mode":               "text",
				"cursorVisualRow":    2,
				"cursorVisualColumn": 7,
				"cursorVisible":      true,
				"cursorShape":        "block",
				"rows": []map[string]any{
					{"index": 0, "runs": []map[string]any{{
						"text": "hello", "attr": uint64(42),
						"foreground": "#123456", "background": "#654321",
						"bold": true, "underline": true, "strikeout": true,
					}}},
				},
			},
		},
	}

	scene := BuildAppSceneFromLegacy(nil, legacy)
	surface := scene["surface"].(map[string]any)
	if surface["kind"] != "editor" || surface["baseName"] != "a.txt" {
		t.Fatalf("unexpected surface: %#v", surface)
	}
	if surface["cursorVisualRow"] != 2 || surface["cursorVisualColumn"] != 7 ||
		surface["cursorVisible"] != true || surface["cursorShape"] != "block" {
		t.Fatalf("editor cursor was not promoted: %#v", surface)
	}
	rows := surface["rows"].([]map[string]any)
	runs := rows[0]["runs"].([]map[string]any)
	if runs[0]["text"] != "hello" || runs[0]["foreground"] != "#123456" ||
		runs[0]["background"] != "#654321" || runs[0]["bold"] != true ||
		runs[0]["underline"] != true || runs[0]["strikeout"] != true {
		t.Fatalf("unexpected rows: %#v", rows)
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
