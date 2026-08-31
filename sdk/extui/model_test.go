package extui

import (
	"testing"
)

func TestMenuItemToMapKeepsIconOptional(t *testing.T) {
	withIcon := (MenuItemModel{Index: 1, Text: "Drive", Icon: "hard-drive"}).ToMap()
	if withIcon["icon"] != "hard-drive" {
		t.Fatalf("menu item icon was not serialized: %#v", withIcon)
	}
	withoutIcon := (MenuItemModel{Index: 2, Text: "Plain"}).ToMap()
	if _, exists := withoutIcon["icon"]; exists {
		t.Fatalf("empty menu item icon was serialized: %#v", withoutIcon)
	}
}

func TestKeyBarItemToMapKeepsIconOptional(t *testing.T) {
	withIcon := (KeyBarItemModel{Index: 2, Key: "F3", Text: "View", Icon: "eye"}).ToMap()
	if withIcon["icon"] != "eye" {
		t.Fatalf("key-bar icon was not serialized: %#v", withIcon)
	}
	withoutIcon := (KeyBarItemModel{Index: 3, Key: "F4", Text: ""}).ToMap()
	if _, exists := withoutIcon["icon"]; exists {
		t.Fatalf("empty key-bar icon was serialized: %#v", withoutIcon)
	}
	withAlternative := (KeyBarItemModel{
		Index: 2, Key: "F3", Text: "View", Icon: "eye",
		Alternatives: []KeyBarAlternativeModel{
			{Modifier: "shift", Text: "Edit", Icon: "file-pen-line"},
		},
	}).ToMap()
	alternatives, ok := withAlternative["alternatives"].([]M)
	if !ok || len(alternatives) != 1 || alternatives[0]["modifier"] != "shift" ||
		alternatives[0]["text"] != "Edit" || alternatives[0]["icon"] != "file-pen-line" {
		t.Fatalf("key-bar alternatives were not serialized: %#v", withAlternative)
	}
}

func TestScenePatchSerializesBoundedSurfaceState(t *testing.T) {
	patch := ScenePatch{
		BaseRevision: 7,
		Revision:     8,
		Surface: &SurfacePatch{
			SurfaceID: "editor:music.svg",
			MapPatch: MapPatch{Set: M{
				"cursorLine": 4,
				"cursorPos":  12,
			}},
		},
	}
	out := patch.ToMap()
	if out["type"] != "scene_patch" || out["baseRevision"] != uint64(7) ||
		out["revision"] != uint64(8) {
		t.Fatalf("unexpected surface patch envelope: %#v", out)
	}
	surface, ok := out["surface"].(M)
	if !ok || surface["id"] != "editor:music.svg" {
		t.Fatalf("surface identity was not serialized: %#v", out["surface"])
	}
	set, ok := surface["set"].(M)
	if !ok || set["cursorLine"] != 4 || set["cursorPos"] != 12 {
		t.Fatalf("surface scalar state was not serialized: %#v", surface)
	}
	if _, leaked := surface["rows"]; leaked {
		t.Fatalf("surface patch unexpectedly carried document rows: %#v", surface)
	}
}

func TestSceneToMapUsesAppSchema(t *testing.T) {
	scene := Scene{
		Width:         100,
		Height:        40,
		ActiveScreen:  1,
		Presentation:  "text",
		QmlIconSet:    "system",
		WorkspaceTabs: M{"visible": true, "mode": "always"},
		Shell: &ShellModel{
			ID:          "shell",
			Mode:        "panels",
			ActivePanel: 1,
			ShowPanels:  true,
			Wide:        true,
			WidePanel:   1,
			PanelLayout: PanelLayoutModel{
				Columns: 100, SplitColumn: 44,
				LeftBottomInsetRows: 2, RightBottomInsetRows: 3,
			},
			Fallback:       true,
			FallbackReason: "unsupported panel layout",
			Panels: []PanelModel{{
				ID:                 "panel:right",
				Side:               1,
				Active:             true,
				Path:               "/tmp",
				ShowFileInfo:       true,
				GalleryLayoutMode:  "grid",
				GalleryColumnCount: 3,
				GalleryDensity:     184,
				GalleryDensities: map[string]int{
					"masonry": 150, "grid": 184, "icons": 64,
				},
				GalleryLayoutRevision:  9,
				SourceKind:             "local",
				PreviewCapable:         true,
				CatalogProvisional:     true,
				CatalogRevision:        7,
				SelectionRevision:      3,
				SeparateFileExtensions: true,
				CursorEntryID:          "entry-alpha",
				FastFind:               true,
				FastFindText:           "*pha",
				FastFindMatchColor:     "#c678dd",
				FastFindMatches: map[string]FastFindMatchModel{
					"entry-alpha": {Start: 2, Length: 3},
				},
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
	layout := shell["panelLayout"].(M)
	if layout["columns"] != 100 || layout["splitColumn"] != 44 ||
		layout["leftBottomInsetRows"] != 2 || layout["rightBottomInsetRows"] != 3 {
		t.Fatalf("panel layout metadata was not serialized: %#v", layout)
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
	if panels[0]["catalogProvisional"] != true {
		t.Fatalf("catalog provisional state was not serialized: %#v", panels[0])
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
	densities := panels[0]["galleryDensities"].(M)
	if densities["masonry"] != 150 || densities["grid"] != 184 ||
		densities["icons"] != 64 || len(densities) != 3 {
		t.Fatalf("bounded per-mode densities were not serialized: %#v", densities)
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
	if panels[0]["showFileInfo"] != true {
		t.Fatalf("panel file-information setting was not serialized: %#v", panels[0])
	}
	if panels[0]["fastFind"] != true || panels[0]["fastFindText"] != "*pha" ||
		panels[0]["fastFindMatchColor"] != "#c678dd" {
		t.Fatalf("panel quick-search state was not serialized: %#v", panels[0])
	}
	matches := panels[0]["fastFindMatches"].(M)
	match := matches["entry-alpha"].(M)
	if match["start"] != 2 || match["length"] != 3 {
		t.Fatalf("panel quick-search span was not serialized: %#v", matches)
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

func TestDeferredPanelToMapKeepsBaseCatalogMinimal(t *testing.T) {
	panel := PanelModel{
		ID: "panel:left", Path: `D:\work`, CatalogRevision: 4,
		SelectionRevision: 2, MetadataDeferred: true, MetadataRevision: 9,
		HighlightRevision: 8, TotalSize: 99, SelectedSize: 22,
		Entries: []FileEntryModel{{
			Index: 0, EntryID: "entry-a", Name: "a.jpg", DisplayBaseName: "a",
			DisplayExtension: "jpg", Path: `D:\work\a.jpg`, LocalPath: `D:\work\a.jpg`,
			Size: 99, IsHidden: true, IsImage: true, Selected: true,
			HighlightStyleID: "style-a",
		}},
		HighlightStyles: map[string]HighlightStyleModel{"style-a": {Marker: "!"}},
	}
	out := panel.ToMap()
	if out["metadataDeferred"] != true || out["metadataRevision"] != int64(9) {
		t.Fatalf("missing deferred metadata envelope: %#v", out)
	}
	for _, key := range []string{"highlightRevision", "totalSize", "selectedSize"} {
		if _, ok := out[key]; ok {
			t.Fatalf("deferred panel leaked %q: %#v", key, out)
		}
	}
	styles := out["highlightStyles"].(M)
	if styles["style-a"].(M)["marker"] != "!" {
		t.Fatalf("minimal catalog lost its immediately resolvable style: %#v", styles)
	}
	entry := out["entries"].([]M)[0]
	if entry["isImage"] != true || entry["isHidden"] != true || entry["selected"] != true {
		t.Fatalf("minimal entry lacks interactive fields: %#v", entry)
	}
	if entry["highlightStyleId"] != "style-a" {
		t.Fatalf("minimal entry lost its style identity: %#v", entry)
	}
	for _, key := range []string{"path", "localPath", "size", "mtimeNanos"} {
		if _, ok := entry[key]; ok {
			t.Fatalf("minimal entry leaked %q: %#v", key, entry)
		}
	}
	if _, present := (FileEntryModel{Name: "visible"}).MinimalToMap()["isHidden"]; present {
		t.Fatalf("ordinary rows must not pay for a false hidden flag")
	}
}

func TestPanelCatalogMetadataChunkToMap(t *testing.T) {
	chunk := PanelCatalogMetadataModel{
		PanelID: "panel:left", Path: "/tmp", CatalogRevision: 3, MetadataRevision: 7,
		HighlightRevision: 11, Offset: 64, Limit: 64, Total: 70, TotalSize: 123,
		Final: true,
		Entries: []FileEntryMetadataModel{{
			Index: 64, EntryID: "entry-64", LocalPath: "/tmp/64", Size: 123,
			SizeText: "123", MTime: "2026-08-17 12:34",
			MTimeNanos: 123456, Mode: "-rw-r--r--", HighlightStyleID: "style-a",
		}},
		HighlightStyles: map[string]HighlightStyleModel{"style-a": {Marker: "!"}},
	}.ToMap()
	if chunk["type"] != "panel_catalog_metadata" || chunk["offset"] != 64 ||
		chunk["limit"] != 64 || chunk["total"] != 70 || chunk["final"] != true ||
		chunk["totalSize"] != int64(123) {
		t.Fatalf("unexpected metadata chunk envelope: %#v", chunk)
	}
	entry := chunk["entries"].([]M)[0]
	if entry["entryId"] != "entry-64" || entry["localPath"] != "/tmp/64" ||
		entry["size"] != int64(123) || entry["sizeText"] != "123" ||
		entry["mtime"] != "2026-08-17 12:34" ||
		entry["mtimeNanos"] != int64(123456) || entry["mode"] != "-rw-r--r--" ||
		entry["highlightStyleId"] != "style-a" {
		t.Fatalf("unexpected metadata row: %#v", entry)
	}
	if _, duplicated := entry["isHidden"]; duplicated {
		t.Fatalf("base hidden state was duplicated in metadata: %#v", entry)
	}
	if len(entry) != 9 {
		t.Fatalf("metadata row schema grew unexpectedly: %#v", entry)
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
	rowKey := TextRowContentKey(rows[0])
	reindexed := rows[0]
	reindexed.Index++
	if got := TextRowContentKey(reindexed); got != rowKey {
		t.Fatalf("window-local row index changed absolute content key %q -> %q",
			rowKey, got)
	}
	repainted := rows[0]
	repainted.Runs = append([]RunModel(nil), rows[0].Runs...)
	repainted.Runs[0].Foreground = "#abcdef"
	if got := TextRowContentKey(repainted); got == rowKey {
		t.Fatalf("row repaint retained content key %q", got)
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
	if got := (SurfaceModel{DefaultBackground: "#242424"}).ToMap()["defaultBackground"]; got != "#242424" {
		t.Fatalf("surface discarded default background: %v", got)
	}
}
