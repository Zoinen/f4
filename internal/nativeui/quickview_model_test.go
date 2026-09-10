package nativeui

import (
	vtui "github.com/unxed/vtui"
	testing "testing"
)

func TestQuickViewSemanticQMLContract_AppScenePreservesNestedInt64Surface(t *testing.T) {
	legacy := map[string]any{
		"type":   "scene",
		"width":  100,
		"height": 40,
		"frames": []map[string]any{{
			"id":         "shell-contract",
			"kind":       "panels",
			"showPanels": true,
			"quickViews": []map[string]any{{
				"id":          "quick-contract",
				"kind":        "quickViewPanel",
				"side":        1,
				"sourceSide":  0,
				"active":      true,
				"title":       "Quick View",
				"bottomHint":  "B",
				"contentKey":  "content-42",
				"name":        "huge.txt",
				"path":        "/virtual/huge.txt",
				"sizeText":    "10 GiB",
				"previewKind": "text",
				"wrap":        true,
				"headerRows": []map[string]any{{
					"visualRow": 0, "text": "huge.txt",
				}},
				"surface": map[string]any{
					"id":                 "quick-contract",
					"kind":               "quick_view",
					"documentKey":        "content-42",
					"scrollAction":       "quickView.scroll",
					"scrollUnit":         "rows",
					"windowStart":        int64(5_000_000_000),
					"windowEnd":          int64(5_000_000_030),
					"viewportStart":      int64(5_000_000_005),
					"viewportSpan":       int64(20),
					"contentExtent":      int64(10_000_000_000),
					"contentExtentKnown": true,
					"viewportRow":        5,
					"viewportRows":       20,
					"windowGeneration":   int64(9),
					"rows": []map[string]any{{
						"visualRow": 5_000_000_005, "offset": int64(5_000_000_005),
						"endOffset": int64(5_000_000_006), "text": "visible",
					}},
					"windowRows": []map[string]any{{
						"visualRow": 5_000_000_000, "offset": int64(5_000_000_000),
						"endOffset": int64(5_000_000_001), "text": "overscan",
					}},
				},
			}},
		}},
	}

	scene := BuildAppSceneFromLegacy(&vtui.SemanticContext{Width: 100, Height: 40}, legacy)
	shell := scene["shell"].(map[string]any)
	quickViews := shell["quickViews"].([]map[string]any)
	if len(quickViews) != 1 {
		t.Fatalf("promoted quickViews = %#v", quickViews)
	}
	quick := quickViews[0]
	if quick["contentKey"] != "content-42" || quick["sourceSide"] != 0 ||
		quick["previewKind"] != "text" {
		t.Fatalf("Quick View chrome was not promoted: %#v", quick)
	}
	surface := quick["surface"].(map[string]any)
	if surface["documentKey"] != "content-42" || surface["scrollAction"] != "quickView.scroll" ||
		surface["windowStart"] != int64(5_000_000_000) ||
		surface["viewportStart"] != int64(5_000_000_005) ||
		surface["contentExtent"] != int64(10_000_000_000) ||
		surface["windowGeneration"] != uint64(9) {
		t.Fatalf("nested 64-bit surface was truncated or rewritten: %#v", surface)
	}
	windowRows := surface["windowRows"].([]map[string]any)
	if len(windowRows) != 1 || windowRows[0]["offset"] != int64(5_000_000_000) ||
		windowRows[0]["endOffset"] != int64(5_000_000_001) {
		t.Fatalf("nested window rows were not promoted: %#v", windowRows)
	}
}
