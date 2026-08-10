package extui

import "testing"

func TestSceneToMapUsesAppSchema(t *testing.T) {
	scene := Scene{
		Width:        100,
		Height:       40,
		ActiveScreen: 1,
		Presentation: "text",
		QmlIconSet:   "system",
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
				Presentation:           "gallery",
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
	if panels[0]["presentation"] != "gallery" || panels[0]["catalogRevision"] != int64(7) || panels[0]["cursorEntryId"] != "entry-alpha" {
		t.Fatalf("panel v3 fields were not serialized: %#v", panels[0])
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
	infoPanels := shell["infoPanels"].([]map[string]any)
	if infoPanels[0]["kind"] != "infoPanel" || infoPanels[0]["side"] != 0 {
		t.Fatalf("unexpected info panel: %#v", infoPanels[0])
	}
	infoRows := infoPanels[0]["rows"].([]map[string]any)
	if infoRows[0]["label"] != "Computer" || infoRows[0]["value"] != "host" {
		t.Fatalf("unexpected info rows: %#v", infoRows)
	}
}
