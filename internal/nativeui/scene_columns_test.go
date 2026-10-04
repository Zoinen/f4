package nativeui

import (
	"slices"
	"testing"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
)

func TestPanelColumnSizingSurvivesFullSceneProjection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		autoWidth bool
	}{
		{name: "automatic", autoWidth: true},
		{name: "manual", autoWidth: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			columns := []extui.PanelColumnModel{
				{ID: "name", Role: "name", Title: "Name", Width: 50,
					AutoWidth: tc.autoWidth, Alignment: "left", Sortable: true, SortMode: "name"},
				{ID: "size", Role: "size", Index: 1, Title: "Size", Width: 14,
					AutoWidth: tc.autoWidth, Alignment: "right", Sortable: true, SortMode: "size"},
				{ID: "exif.iso", Role: "fileField", Index: 2, Title: "ISO", Width: 12,
					AutoWidth: tc.autoWidth, Alignment: "right", Sortable: true, SortMode: "exif.iso"},
			}
			input := extui.PanelModel{GalleryColumns: columns}
			output := appPanelFromLegacy(input.ToMap())
			if !slices.Equal(output.GalleryColumns, columns) {
				t.Fatalf("column sizing changed in full scene: got %+v, want %+v", output.GalleryColumns, columns)
			}
		})
	}
	legacy := appPanelFromLegacy(map[string]any{
		"galleryColumns": []map[string]any{{"id": "size", "width": 14}},
	})
	if len(legacy.GalleryColumns) != 1 || legacy.GalleryColumns[0].AutoWidth {
		t.Fatal("legacy columns without autoWidth must retain relative sizing")
	}
}

func TestNavigationDoesNotResendOppositePanelColumnSizing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		movingSide int
	}{
		{name: "left navigation", movingSide: 0},
		{name: "right navigation", movingSide: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			panels := make([]extui.PanelModel, 2)
			for side := range panels {
				node := incrementalTestPanel(side, nil)
				node["galleryLayoutMode"] = "details"
				panels[side] = appPanelFromLegacy(node)
				panels[side].GalleryColumns = []extui.PanelColumnModel{
					{ID: "name", Role: "name", Width: 50, AutoWidth: true},
					{ID: "size", Role: "size", Width: 14, AutoWidth: true},
					{ID: "exif.iso", Role: "fileField", Width: 12},
				}
			}
			shell := extui.ShellModel{ID: "panels", Mode: "panels", Panels: panels}
			scene := extui.Scene{Shell: &shell}
			for _, path := range []string{"/parent/child", "/parent", "/parent/child"} {
				moving := &panels[tc.movingSide]
				moving.Path = path
				moving.CatalogRevision++
				moving.Loading = true
				// A directory/catalog change falls back to a full scene. The
				// next row-free update uses the typed incremental projection.
				full := scene.ToMap()
				full["shell"] = appShellFromLegacy(shell.ToMap()).ToMap()
				previous := semantic.CompactAppSemanticScene(full)
				moving.Loading = false
				current := semantic.CompactAppSemanticScene(scene.ToMap())
				patch, _, ok := BuildAppScenePatch(previous, &appIncrementalScene{Scene: current})
				if !ok || patch.Shell == nil {
					t.Fatal("loading completion must produce an incremental shell update")
				}
				for _, update := range patch.Shell.Panels {
					if _, changed := update.State["galleryColumns"]; changed {
						t.Errorf("navigation changed column sizing on side %d: %+v", update.Side, update.State["galleryColumns"])
					}
					if update.Side != tc.movingSide {
						t.Errorf("navigation resent unchanged opposite panel: %+v", update)
					}
				}
				t.Logf("[FIX:column-sizing] movingSide=%d path=%s panelUpdates=%d",
					tc.movingSide, path, len(patch.Shell.Panels))
			}
		})
	}
}
