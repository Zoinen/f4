package extui

import (
	"testing"
)

func TestSceneToMapUsesAppSchema(t *testing.T) {
	scene := Scene{
		Width:         100,
		Height:        40,
		ActiveScreen:  1,
		Presentation:  "text",
		QmlIconSet:    "system",
		WorkspaceTabs: M{"visible": true, "mode": "always"},
		Shell: &ShellModel{
			ID:             "shell",
			Mode:           "panels",
			ActivePanel:    1,
			ShowPanels:     true,
			Wide:           true,
			WidePanel:      1,
			Fallback:       true,
			FallbackReason: "unsupported panel layout",
			Panels: []PanelModel{{
				ID:                     "panel:right",
				Side:                   1,
				Active:                 true,
				Path:                   "/tmp",
				GalleryLayoutMode:      "grid",
				GalleryColumnCount:     3,
				GalleryDensity:         184,
				GalleryLayoutRevision:  9,
				SourceKind:             "local",
				PreviewCapable:         true,
				CatalogRevision:        7,
				SelectionRevision:      3,
				SeparateFileExtensions: true,
				CursorEntryID:          "entry-alpha",
				GalleryColumns: []PanelColumnModel{{
					ID: "name", Role: "name", Index: 0, Title: "Name",
					Width: 30, Alignment: "left", SortMode: "name", Sortable: true,
				}},
				Entries: []FileEntryModel{{
					Index:            0,
					EntryID:          "entry-alpha",
					Name:             "alpha.txt",
					DisplayBaseName:  "alpha",
					DisplayExtension: "txt",
					LocalPath:        "/tmp/alpha.txt",
					MTimeNanos:       123,
					Version:          "123:4",
					Source: &ImageSourceModel{
						ResourceID: "resource-1", SourceKey: "source-1", Version: "123:4",
						VersionStrength: "localStat", Size: 4, SizeKnown: true,
						AccessProfile: "directLocal", StorageClass: "local",
					},
				}},
			}},
			InfoPanels: []InfoPanelModel{{
				ID:         "info:left",
				Side:       0,
				Title:      "Information",
				BottomHint: "B Units",
				Rows: []InfoPanelRowModel{{
					Kind: "row", Label: "Computer", Value: "host",
				}},
			}},
		},
	}

	out := scene.ToMap()
	if out["type"] != "scene" || out["schema"] != Schema || out["version"] != SceneVersion {
		t.Fatalf("unexpected scene envelope: %#v", out)
	}
	if out["presentation"] != "text" {
		t.Fatalf("presentation = %v", out["presentation"])
	}
	if out["qmlIconSet"] != "system" {
		t.Fatalf("qmlIconSet = %v", out["qmlIconSet"])
	}
	if tabs := out["workspaceTabs"].(M); tabs["mode"] != "always" || tabs["visible"] != true {
		t.Fatalf("workspaceTabs = %#v", tabs)
	}
	shell := out["shell"].(map[string]any)
	if shell["kind"] != "shell" || shell["activePanel"] != 1 {
		t.Fatalf("unexpected shell: %#v", shell)
	}
	if shell["wide"] != true || shell["widePanel"] != 1 {
		t.Fatalf("wide panel metadata was not serialized: %#v", shell)
	}
	if shell["fallback"] != true || shell["reason"] != "unsupported panel layout" {
		t.Fatalf("unexpected fallback metadata: %#v", shell)
	}
	panels := shell["panels"].([]map[string]any)
	if panels[0]["path"] != "/tmp" {
		t.Fatalf("panel path = %v", panels[0]["path"])
	}
	if panels[0]["catalogRevision"] != int64(7) || panels[0]["cursorEntryId"] != "entry-alpha" {
		t.Fatalf("panel v3 fields were not serialized: %#v", panels[0])
	}
	for _, retired := range []string{"presentation", "viewModeName", "columns", "top"} {
		if _, present := panels[0][retired]; present {
			t.Fatalf("retired panel field %q was serialized: %#v", retired, panels[0])
		}
	}
	if panels[0]["galleryLayoutMode"] != "grid" ||
		panels[0]["galleryColumnCount"] != 3 ||
		panels[0]["galleryDensity"] != 184 ||
		panels[0]["galleryLayoutRevision"] != int64(9) {
		t.Fatalf("gallery layout fields were not serialized: %#v", panels[0])
	}
	galleryColumns := panels[0]["galleryColumns"].([]map[string]any)
	if len(galleryColumns) != 1 || galleryColumns[0]["id"] != "name" ||
		galleryColumns[0]["role"] != "name" ||
		galleryColumns[0]["alignment"] != "left" ||
		galleryColumns[0]["sortMode"] != "name" {
		t.Fatalf("gallery columns were not serialized: %#v", galleryColumns)
	}
	if panels[0]["separateFileExtensions"] != true {
		t.Fatalf("panel extension alignment setting was not serialized: %#v", panels[0])
	}
	entries := panels[0]["entries"].([]map[string]any)
	if entries[0]["entryId"] != "entry-alpha" || entries[0]["localPath"] != "/tmp/alpha.txt" || entries[0]["version"] != "123:4" {
		t.Fatalf("entry v3 fields were not serialized: %#v", entries[0])
	}
	if entries[0]["displayBaseName"] != "alpha" || entries[0]["displayExtension"] != "txt" {
		t.Fatalf("entry display name parts were not serialized: %#v", entries[0])
	}
	source := entries[0]["source"].(map[string]any)
	if source["resourceId"] != "resource-1" || source["sourceKey"] != "source-1" ||
		source["versionStrength"] != "localStat" || source["accessProfile"] != "directLocal" ||
		source["storageClass"] != "local" || source["sizeKnown"] != true {
		t.Fatalf("entry source descriptor was not serialized: %#v", source)
	}
	infoPanels := shell["infoPanels"].([]map[string]any)
	if infoPanels[0]["kind"] != "infoPanel" || infoPanels[0]["side"] != 0 {
		t.Fatalf("unexpected info panel: %#v", infoPanels[0])
	}
	infoRows := infoPanels[0]["rows"].([]map[string]any)
	if infoRows[0]["label"] != "Computer" || infoRows[0]["value"] != "host" {
		t.Fatalf("unexpected info rows: %#v", infoRows)
	}
}

func TestWindowRowsContentKeyTracksTextAndCompleteRunStyle(t *testing.T) {
	rows := []TextRowModel{{
		Index: 3, VisualRow: 41, LogicalLine: 40, Offset: 100, EndOffset: 110,
		Runs: []RunModel{{
			Text: "hello", Attr: 42, Foreground: "#123456", Background: "#654321",
			Bold: true, Underline: true, Strikeout: true,
		}},
	}, {
		Index: 4, VisualRow: 42, LogicalLine: 41, Offset: 110, EndOffset: 116,
		Text: "world",
	}}
	original := WindowRowsContentKey(rows)
	if original == "" || original != WindowRowsContentKey(append([]TextRowModel(nil), rows...)) {
		t.Fatalf("unchanged rows produced unstable key %q", original)
	}

	mutations := map[string]func([]TextRowModel){
		"row text":       func(copy []TextRowModel) { copy[1].Text = "World" },
		"row extent":     func(copy []TextRowModel) { copy[1].EndOffset++ },
		"run text":       func(copy []TextRowModel) { copy[0].Runs[0].Text = "Hello" },
		"run attr":       func(copy []TextRowModel) { copy[0].Runs[0].Attr++ },
		"run foreground": func(copy []TextRowModel) { copy[0].Runs[0].Foreground = "#abcdef" },
		"run background": func(copy []TextRowModel) { copy[0].Runs[0].Background = "#fedcba" },
		"run bold":       func(copy []TextRowModel) { copy[0].Runs[0].Bold = false },
		"run underline":  func(copy []TextRowModel) { copy[0].Runs[0].Underline = false },
		"run strikeout":  func(copy []TextRowModel) { copy[0].Runs[0].Strikeout = false },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			copyRows := append([]TextRowModel(nil), rows...)
			copyRows[0].Runs = append([]RunModel(nil), rows[0].Runs...)
			mutate(copyRows)
			if changed := WindowRowsContentKey(copyRows); changed == original {
				t.Fatalf("%s mutation retained key %q", name, changed)
			}
		})
	}

	computed := SurfaceModel{WindowRows: rows}.ToMap()["windowContentKey"]
	if computed != original {
		t.Fatalf("surface serialized key=%v, want computed %q", computed, original)
	}
	const supplied = "renderer-revision-7"
	if got := (SurfaceModel{WindowRows: rows, WindowContentKey: supplied}).ToMap()["windowContentKey"]; got != supplied {
		t.Fatalf("surface discarded supplied content key: %v", got)
	}
}
