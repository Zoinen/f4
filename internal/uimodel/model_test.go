package uimodel

import "testing"

func TestSceneToMapUsesAppSchema(t *testing.T) {
	scene := Scene{
		Width:        100,
		Height:       40,
		ActiveScreen: 1,
		Shell: &ShellModel{
			ID:          "shell",
			Mode:        "panels",
			ActivePanel: 1,
			ShowPanels:  true,
			Panels: []PanelModel{{
				ID:     "panel:right",
				Side:   1,
				Active: true,
				Path:   "/tmp",
				Entries: []FileEntryModel{{
					Index: 0,
					Name:  "alpha.txt",
				}},
			}},
		},
	}

	out := scene.ToMap()
	if out["type"] != "scene" || out["schema"] != Schema || out["version"] != SceneVersion {
		t.Fatalf("unexpected scene envelope: %#v", out)
	}
	shell := out["shell"].(map[string]any)
	if shell["kind"] != "shell" || shell["activePanel"] != 1 {
		t.Fatalf("unexpected shell: %#v", shell)
	}
	panels := shell["panels"].([]map[string]any)
	if panels[0]["path"] != "/tmp" {
		t.Fatalf("panel path = %v", panels[0]["path"])
	}
}
