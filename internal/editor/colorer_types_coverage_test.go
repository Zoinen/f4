package editor

import (
	"testing"

	colorer "github.com/unxed/colorer4go"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestColorerRegionSpansDropsEmptyRegions(t *testing.T) {
	got := colorerRegionSpans([]colorer.Region{
		{Start: 0, End: 0},
		{Start: 1, End: 4},
		{Start: 4, End: 4},
		{Start: 5, End: -1},
	})
	want := []colorerRegionSpan{{1, 4}, {5, -1}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("colorerRegionSpans=%v, want %v", got, want)
	}
}

func TestColorerTypeFrameRebuildAndProfileParam(t *testing.T) {
	_ = config.GetF4ConfigDir()
	oldConfigDir := config.CachedF4ConfigDir
	config.CachedF4ConfigDir = t.TempDir()
	t.Cleanup(func() { config.CachedF4ConfigDir = oldConfigDir })

	f := &colorerTypeFrame{
		VMenu: vtui.NewVMenu("types"),
		types: []colorerTypeEntry{
			{name: "go", group: "Languages", description: "Go"},
			{name: "json", group: "Data", description: "JSON", favorite: true},
			{name: "yaml", group: "Data", description: "YAML"},
		},
		current: "go",
		profile: map[string]map[string]string{},
	}
	f.rebuild(-1)
	if len(f.Items) != 7 || len(f.rowType) != 7 {
		t.Fatalf("rebuilt rows=%d types=%v", len(f.Items), f.rowType)
	}
	if got := f.selectedType(); got != 0 {
		t.Fatalf("current type selected=%d, want go index 0", got)
	}
	f.rebuild(2)
	if got := f.selectedType(); got != 2 {
		t.Fatalf("explicit type selected=%d, want yaml index 2", got)
	}
	f.setParam(2, colorerParamHotkey, "Y")
	got := loadColorerProfile()
	if got["yaml"][colorerParamHotkey] != "Y" {
		t.Fatalf("saved profile=%v", got)
	}
}

func TestColorerTypeFrameShowPaintsSeparatorsAndTotal(t *testing.T) {
	vtui.SetDefaultPalette()
	f := &colorerTypeFrame{
		VMenu: vtui.NewVMenu("types"),
		types: []colorerTypeEntry{{name: "go", group: "Languages", description: "Go"}},
	}
	f.rebuild(-1)
	f.SetPosition(1, 1, 30, 10)
	f.Show(vtui.NewSilentScreenBuf())
}
