package extui

import "testing"

func TestModelToMapsExposeOptionalFieldsAndNestedModels(t *testing.T) {
	rows := []TextRowModel{
		{},
		{
			Index:       2,
			VisualRow:   3,
			LogicalLine: 4,
			Offset:      5,
			Text:        "row",
			Runs:        []RunModel{{Text: "run", Attr: 7}},
		},
	}
	control := ControlModel{
		ID:         "control",
		Kind:       "checkbox",
		Visible:    true,
		Focused:    true,
		Disabled:   true,
		Text:       "text",
		Title:      "title",
		Hotkey:     "h",
		State:      1,
		ThreeState: true,
		Default:    true,
		Password:   true,
		Cursor:     2,
		Left:       3,
		Selected:   []int{1},
		Items:      []string{"one"},
		Rows:       []M{{"row": "value"}},
		Children:   []ControlModel{{ID: "child"}},
		Legacy:     M{"kind": "legacy-kind", "legacyControl": true},
	}
	item := MenuItemModel{
		Index:     1,
		Text:      "item",
		RawText:   "raw",
		Hotkey:    "i",
		Shortcut:  "Ctrl-I",
		Command:   9,
		Separator: true,
		Disabled:  true,
		Items:     []MenuItemModel{{Text: "child item"}},
		Legacy:    M{"text": "legacy-text", "legacyItem": true},
	}
	menu := MenuModel{
		ID:       "menu",
		Role:     "main",
		Title:    "menu title",
		Active:   true,
		Selected: 1,
		Items:    []MenuItemModel{item},
		Legacy:   M{"title": "legacy-title", "legacyMenu": true},
	}
	scene := Scene{
		Width:          100,
		Height:         40,
		ActiveScreen:   1,
		WorkspaceCount: 3,
		Shell: &ShellModel{
			ID:             "shell",
			Title:          "shell title",
			Mode:           "panels",
			ActivePanel:    1,
			ShowPanels:     true,
			ShowKeyBar:     true,
			TerminalBusy:   true,
			TerminalActive: true,
			MacroRecording: true,
			Panels: []PanelModel{{
				ID:                "panel",
				Side:              1,
				Active:            true,
				Path:              "/tmp",
				Title:             "panel title",
				GalleryLayoutMode: "details",
				SortMode:          "name",
				SortReverse:       true,
				Cursor:            2,
				Loading:           true,
				FastFind:          true,
				FastFindText:      "find",
				SelectedCount:     4,
				SelectedSize:      5,
				TotalCount:        6,
				TotalSize:         7,
				Entries: []FileEntryModel{{
					Index:          1,
					Name:           "entry",
					Size:           2,
					SizeText:       "2 B",
					IsDir:          true,
					IsUp:           true,
					IsHidden:       true,
					IsExecutable:   true,
					EntryID:        "entry-1",
					Selected:       true,
					SizeCalculated: true,
					MTime:          "now",
					Mode:           "0755",
				}},
			}},
			CommandLine: &CommandLineModel{
				ID:         "command-line",
				Visible:    true,
				Focused:    true,
				Prompt:     ">",
				PromptRuns: []RunModel{{Text: "prompt", Attr: 1}},
				Text:       "command",
				Empty:      false,
			},
			Terminal: &TerminalModel{
				ID:        "terminal",
				Title:     "terminal title",
				Visible:   true,
				Focused:   true,
				AltScreen: true,
				Busy:      true,
				CursorX:   4,
				CursorY:   5,
				Rows:      rows,
			},
		},
		MenuBar: &menu,
		KeyBar: &KeyBarModel{
			ID:       "keybar",
			Visible:  true,
			Modifier: "Ctrl",
			Items:    []KeyBarItemModel{{Index: 1, Key: "F1", Text: "Help"}},
		},
		Toast: &ToastModel{Message: "saved"},
		Dialogs: []DialogModel{{
			ID:        "dialog",
			Kind:      "confirm",
			Title:     "dialog title",
			Modal:     true,
			Busy:      true,
			Progress:  50,
			ShowClose: true,
			Controls:  []ControlModel{control},
			Legacy:    M{"title": "legacy-dialog-title", "legacyDialog": true},
		}},
		Menus: []MenuModel{menu},
		Surface: &SurfaceModel{
			ID:           "surface",
			Kind:         "editor",
			Title:        "surface title",
			Path:         "/tmp/file",
			BaseName:     "file",
			Mode:         "text",
			Busy:         true,
			Dirty:        true,
			Saving:       true,
			HexMode:      true,
			WrapMode:     true,
			WordWrap:     true,
			Overtype:     true,
			TopOffset:    6,
			Size:         7,
			CursorLine:   8,
			CursorPos:    9,
			ScrollTop:    10,
			ScrollLeft:   11,
			Selection:    true,
			Autocomplete: M{"visible": true},
			Rows:         rows,
		},
		Legacy: M{"frames": []string{"frame"}, "screens": []int{1}},
	}

	out := scene.ToMap()
	if out["workspaceCount"] != 3 || out["frames"] == nil || out["screens"] == nil {
		t.Fatalf("scene optional fields missing: %#v", out)
	}
	shell := out["shell"].(M)
	if shell["commandLine"].(M)["text"] != "command" || shell["terminal"].(M)["cursorY"] != 5 {
		t.Fatalf("shell children missing: %#v", shell)
	}
	panel := shell["panels"].([]M)[0]
	if panel["entries"].([]M)[0]["mode"] != "0755" {
		t.Fatalf("panel entry missing: %#v", panel)
	}
	if out["keyBar"].(M)["items"].([]M)[0]["text"] != "Help" {
		t.Fatalf("key bar missing: %#v", out["keyBar"])
	}
	if out["toast"].(M)["message"] != "saved" {
		t.Fatalf("toast missing: %#v", out["toast"])
	}

	menuOut := out["menuBar"].(M)
	if menuOut["title"] != "menu title" || menuOut["legacyMenu"] != true {
		t.Fatalf("menu legacy merge is wrong: %#v", menuOut)
	}
	menuItemOut := menuOut["items"].([]M)[0]
	if menuItemOut["text"] != "item" || menuItemOut["legacyItem"] != true {
		t.Fatalf("menu item legacy merge is wrong: %#v", menuItemOut)
	}
	if out["menus"].([]M)[0]["id"] != "menu" {
		t.Fatalf("menus missing: %#v", out["menus"])
	}

	dialog := out["dialogs"].([]M)[0]
	if dialog["title"] != "dialog title" || dialog["legacyDialog"] != true {
		t.Fatalf("dialog legacy merge is wrong: %#v", dialog)
	}
	controlOut := dialog["children"].([]M)[0]
	if controlOut["kind"] != "checkbox" || controlOut["legacyControl"] != true {
		t.Fatalf("control legacy merge is wrong: %#v", controlOut)
	}
	if len(controlOut["children"].([]M)) != 1 || controlOut["rows"].([]M)[0]["row"] != "value" {
		t.Fatalf("control nested fields missing: %#v", controlOut)
	}

	surface := out["surface"].(M)
	if surface["autocomplete"].(M)["visible"] != true || len(surface["rows"].([]M)) != 2 {
		t.Fatalf("surface fields missing: %#v", surface)
	}
}
