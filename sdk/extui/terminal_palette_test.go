package extui

import "testing"

func TestRunPaletteMetadataPreservesExactIndices(t *testing.T) {
	// A true-color background occupies the high 24 bits, making a JS double
	// unsuitable for extracting the remaining flags. Keep metadata exact.
	attr := uint64(0xffffff)<<40 | uint64(12)<<16 | 0x0200 | 0x1000
	run := (RunModel{Attr: attr}).ToMap()
	if run["foregroundPaletteIndex"] != 12 || run["backgroundPaletteIndex"] != -1 || run["foregroundDim"] != true {
		t.Fatalf("palette metadata: %v", run)
	}
	run = (RunModel{Attr: attr | 0x4000}).ToMap()
	if run["foregroundPaletteIndex"] != -1 || run["backgroundPaletteIndex"] != 12 || run["backgroundDim"] != true {
		t.Fatalf("reverse palette metadata: %v", run)
	}
	run = (RunModel{Attr: uint64(200) << 16}).ToMap()
	if run["foregroundPaletteIndex"] != 200 {
		t.Fatal("extended palette index was changed")
	}
}
