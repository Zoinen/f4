package main

import (
	"testing"

	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func TestBuildAppSceneFromLegacyPromotesShellAndKeepsFallback(t *testing.T) {
	legacy := map[string]any{
		"type":         "scene",
		"version":      1,
		"width":        100,
		"height":       40,
		"activeScreen": 0,
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
							{"index": 0, "name": "..", "isDir": true, "isUp": true},
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
	if scene["legacy"] == nil || scene["frames"] == nil {
		t.Fatalf("scene did not preserve legacy fallback: %#v", scene)
	}
	shell := scene["shell"].(map[string]any)
	if shell["kind"] != "shell" || shell["activePanel"] != 1 {
		t.Fatalf("unexpected shell: %#v", shell)
	}
	panels := shell["panels"].([]map[string]any)
	if panels[1]["path"] != "/right" || panels[1]["active"] != true {
		t.Fatalf("unexpected panel model: %#v", panels[1])
	}
}

func TestBuildAppSceneFromLegacyPromotesDocumentSurface(t *testing.T) {
	legacy := map[string]any{
		"type":   "scene",
		"width":  80,
		"height": 25,
		"frames": []map[string]any{
			{
				"id":       "viewer",
				"kind":     "viewer",
				"title":    " Viewer ",
				"path":     "/tmp/a.txt",
				"baseName": "a.txt",
				"mode":     "text",
				"rows": []map[string]any{
					{"index": 0, "text": "hello"},
				},
			},
		},
	}

	scene := BuildAppSceneFromLegacy(nil, legacy)
	surface := scene["surface"].(map[string]any)
	if surface["kind"] != "viewer" || surface["baseName"] != "a.txt" {
		t.Fatalf("unexpected surface: %#v", surface)
	}
	rows := surface["rows"].([]map[string]any)
	if rows[0]["text"] != "hello" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}
