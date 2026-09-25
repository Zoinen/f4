package nativeui

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/semantic"
)

func TestDirectoryPreviewAuthoritySurvivesCatalogSceneProjection(t *testing.T) {
	descriptor := map[string]any{
		"resourceId": "directory-album", "sourceKey": "vfs/album", "version": "observation-1",
	}
	for _, grouping := range []string{"None", "Extension"} {
		t.Run(grouping, func(t *testing.T) {
			panel := incrementalTestPanel(1, []map[string]any{
				{"index": 0, "entryId": "parent", "name": "..", "isDir": true},
				{"index": 1, "entryId": "album", "name": "album", "isDir": true, "directorySource": descriptor},
			})
			panel["groupBy"] = grouping
			legacy := map[string]any{"frames": []map[string]any{
				{"kind": "panels", "panels": []map[string]any{panel}},
			}}
			projected := BuildAppSceneFromLegacy(nil, legacy)
			shell := semantic.AppMap(projected["shell"])
			panels := semantic.AppMapSlice(shell["panels"])
			if len(panels) != 1 {
				t.Fatalf("projected panels = %#v", panels)
			}
			entries := semantic.AppMapSlice(panels[0]["entries"])
			if len(entries) != 2 || !reflect.DeepEqual(entries[1]["directorySource"], descriptor) {
				t.Fatalf("folder preview authority lost in catalog projection: %#v", entries)
			}
			if entries[0]["directorySource"] != nil || entries[1]["source"] != nil {
				t.Fatal("directory authority leaked to parent or image byte source")
			}
		})
	}
}
