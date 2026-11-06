package nativeui

import (
	"encoding/json"
	"testing"

	"github.com/unxed/f4/internal/semantic"
)

func TestDocumentRoundTripRetainsUnchangedPanelCatalog(t *testing.T) {
	for _, kind := range []string{"viewer", "editor"} {
		t.Run(kind, func(t *testing.T) {
			rows := []map[string]any{{"index": 0, "entryId": "file-0", "name": "test.txt"}}
			full := map[string]any{"type": "scene", "schema": "app", "version": 4,
				"shell": map[string]any{"id": "shell", "kind": "shell", "activePanel": 0,
					"panels": []map[string]any{incrementalTestPanel(0, rows)}},
			}
			prior := semantic.CompactAppSemanticScene(full)
			for tick := 0; tick < 2; tick++ {
				document := &appIncrementalScene{Scene: map[string]any{
					"type": "scene", "schema": "app", "version": 4,
					"surface": map[string]any{"id": "document", "kind": kind, "cursorRow": tick},
				}}
				patch, _, ok := BuildAppScenePatch(prior, document)
				if !ok {
					t.Fatal("opening document rejected")
				}
				semantic.ApplyAppScenePatchToSnapshot(full, patch)
				if len(semantic.AppMapSlice(semantic.SemanticPanelsBySide(full)[0]["entries"])) != 1 {
					t.Fatal("opening document discarded retained catalog")
				}
				prior = document.Scene
			}
			for _, changed := range []string{"unchanged", "revision", "identity"} {
				t.Run(changed, func(t *testing.T) {
					panel := incrementalTestPanel(0, nil)
					if changed == "revision" {
						panel["catalogRevision"] = int64(999)
					}
					if changed == "identity" {
						panel["id"] = "different-workspace-panel"
					}
					current := &appIncrementalScene{Scene: map[string]any{
						"type": "scene", "schema": "app", "version": 4,
						"shell": map[string]any{"id": "shell", "kind": "shell", "activePanel": 0,
							"panels": []map[string]any{panel}},
					}}
					patch, _, ok := BuildAppScenePatch(prior, current)
					if changed != "unchanged" {
						if ok {
							t.Fatal("changed catalog must use authoritative full export")
						}
						return
					}
					if !ok {
						t.Fatal("returning to unchanged panels forced a full export")
					}
					restored := semantic.SemanticSceneStructuralMapCopy(full)
					semantic.ApplyAppScenePatchToSnapshot(restored, patch)
					if _, present := restored["surface"]; present {
						t.Fatal("document surface not removed")
					}
					if len(semantic.AppMapSlice(semantic.SemanticPanelsBySide(restored)[0]["entries"])) != 1 {
						t.Fatal("return patch lost cached rows")
					}
					payload, err := json.Marshal(patch.ToMap())
					if err != nil {
						t.Fatal(err)
					}
					if len(payload) > 1024 {
						t.Fatalf("return patch unexpectedly large: %d", len(payload))
					}
				})
			}
		})
	}
}
