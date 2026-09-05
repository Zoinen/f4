package main

import (
	"strings"
	"testing"

	"github.com/unxed/f4/textlayout"
	"github.com/unxed/vtui"
)

func TestLayoutViewerTextRowKeepsIndicClustersTogether(t *testing.T) {
	// स्कृ is one cluster of two cells (its column sum, what the terminal
	// advances), so three columns hold exactly the first two clusters.
	text := "संस्कृतम्\n"
	first := layoutViewerTextRow([]byte(text), 3, 8, true)
	if got, want := string([]byte(text)[:first.textLen]), "संस्कृ"; got != want {
		t.Fatalf("first wrapped row = %q, want %q", got, want)
	}
	if first.lineLen != first.textLen || first.foundNewline {
		t.Fatalf("first row metadata = %+v, want an intermediate wrapped row", first)
	}

	second := layoutViewerTextRow([]byte(text)[first.lineLen:], 3, 8, true)
	if !second.foundNewline || second.lineLen != len([]byte("तम्\n")) {
		t.Fatalf("second row metadata = %+v, want the terminating row", second)
	}

	cells, _ := viewerTextCells(string([]byte(text)[:first.textLen]), 0, 8, 3)
	if len(cells) != 3 {
		t.Fatalf("first row rendered %d cells, want 3", len(cells))
	}
}

func TestViewerTextCellsKeepsRTLClustersAndOffsets(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	t.Cleanup(func() { vtui.DefaultBidiMode = oldMode })
	vtui.DefaultBidiMode = vtui.BidiFull

	text := "ދިވެހިބަސް"
	cells, offsets := viewerTextCells(text, 0, 8, 5)
	if len(cells) != 5 || len(offsets) != 5 {
		t.Fatalf("RTL render returned %d cells and %d offsets, want five of each", len(cells), len(offsets))
	}
	want := []int{16, 12, 8, 4, 0}
	for i := range want {
		if offsets[i] != want[i] {
			t.Fatalf("visual cell %d offset = %d, want %d", i, offsets[i], want[i])
		}
	}
}

func TestViewerTextCellsKeepsIndicClustersInsideBidiParagraph(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	t.Cleanup(func() { vtui.DefaultBidiMode = oldMode })
	vtui.DefaultBidiMode = vtui.BidiFull

	text := "संस्कृतम् ދިވެހިބަސް"
	clusters := textlayout.VisualClustersInVisualOrder(text)
	wantCells := 0
	for _, cluster := range clusters {
		wantCells += cluster.Width
	}
	cells, offsets := viewerTextCells(text, 0, 8, 100)
	if len(cells) != wantCells || len(offsets) != wantCells {
		t.Fatalf("rendered %d cells/%d offsets for %d clusters of %d columns", len(cells), len(offsets), len(clusters), wantCells)
	}
	seen := make(map[int]bool, len(offsets))
	for _, offset := range offsets {
		seen[offset] = true
	}
	if len(seen) != len(clusters) {
		t.Fatalf("rendered offsets covered %d clusters, want %d", len(seen), len(clusters))
	}
}

func TestLayoutViewerTextRowDoesNotSplitCombiningSequence(t *testing.T) {
	text := "a" + strings.Repeat("\u0301", 32) + "b"
	row := layoutViewerTextRow([]byte(text), 1, 8, true)
	if row.textLen <= len("a") || row.textLen >= len(text) {
		t.Fatalf("combining row ended at byte %d of %d, expected the first grapheme", row.textLen, len(text))
	}
	cells, _ := viewerTextCells(text[:row.textLen], 0, 8, 1)
	if len(cells) != 1 {
		t.Fatalf("combining row rendered %d cells, want one", len(cells))
	}
}

func TestApplyViewerSearchAttrHighlightsOnlyCurrentMatch(t *testing.T) {
	text := "needle before needle after"
	baseAttr := vtui.SetRGBBoth(0, 0xD0D0D0, 0x232323)
	selectedAttr := vtui.SetRGBBoth(0, 0x2E3436, 0xFCE94F)
	cells, offsets := viewerTextCells(text, baseAttr, 8, 100)

	matchStart := strings.Index(text, "needle")
	matchEnd := matchStart + len("needle")
	applyViewerSearchAttr(cells, text, offsets, matchStart, matchEnd, selectedAttr)

	for i, offset := range offsets {
		want := baseAttr
		if offset >= matchStart && offset < matchEnd {
			want = selectedAttr
		}
		if cells[i].Attributes != want {
			t.Fatalf("cell %d at source offset %d has attributes %016x, want %016x", i, offset, cells[i].Attributes, want)
		}
	}
}

func TestApplyViewerSearchAttrKeepsGraphemeTogether(t *testing.T) {
	text := "a\u0301 tail"
	baseAttr := vtui.SetRGBBoth(0, 0xD0D0D0, 0x232323)
	selectedAttr := vtui.SetRGBBoth(0, 0x2E3436, 0xFCE94F)
	cells, offsets := viewerTextCells(text, baseAttr, 8, 100)

	// The search result covers only the base rune, but the terminal cell also
	// owns its combining mark. Highlight the complete visible grapheme.
	applyViewerSearchAttr(cells, text, offsets, 0, len("a"), selectedAttr)
	if len(cells) == 0 || cells[0].Attributes != selectedAttr {
		t.Fatalf("base grapheme was not highlighted: %#v", cells)
	}
	if len(cells) < 2 || cells[1].Attributes != baseAttr {
		t.Fatalf("text after the matched grapheme changed attributes")
	}
}
