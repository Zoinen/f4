package app

import (
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/semantic"
	"testing"
)

func TestSemanticDropCapabilitySurvivesFullSceneRebuild(t *testing.T) {
	// Dialogs and workspace changes rebuild the typed scene from the legacy
	// projection. Writable panels must remain drop targets after that rebuild.
	for _, allowed := range []bool{true, false} {
		legacy := productionPanelCatalogLegacy(`D:\source`, 1, 1, false, "item.txt")
		frames := legacy["frames"].([]map[string]any)
		for _, panel := range frames[0]["panels"].([]map[string]any) {
			panel["dropAllowed"] = allowed
		}
		scene := nativeui.BuildAppSceneFromLegacy(nil, legacy)
		panels, ok := semantic.SemanticScenePanelMaps(scene)
		if !ok || len(panels) != 2 {
			t.Fatal("rebuilt scene lost its panels")
		}
		for _, panel := range panels {
			if panel["dropAllowed"] != allowed {
				t.Fatalf("scene rebuild changed drop capability: want %v, got %v", allowed, panel["dropAllowed"])
			}
		}
	}
}
