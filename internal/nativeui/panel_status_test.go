package nativeui

import "testing"

func TestPanelStatusCapacitySurvivesProjection(t *testing.T) {
	input := map[string]any{
		"totalFiles": 12, "totalDirectories": 3, "selectedFiles": 2, "selectedDirectories": 1,
		"totalSize": int64(2048), "selectedSize": int64(512),
		"diskTotalSpace": uint64(1 << 40), "freeSpace": uint64(1 << 38), "freeSpaceKnown": true,
	}
	model := appPanelFromLegacy(input)
	output := appPanelFromLegacy(model.ToMap())
	if output.TotalFiles != 12 || output.TotalDirectories != 3 ||
		output.DiskTotalSpace != 1<<40 || output.FreeSpace != 1<<38 || !output.FreeSpaceKnown {
		t.Fatalf("status lost in typed projection: %+v", output)
	}
}
