package app

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestCompareCheckStateTrueCoverageBatch35(t *testing.T) {
	if got := compareCheckState(true); got != 1 {
		t.Fatalf("compareCheckState(true) = %d, want 1", got)
	}
}

func TestCompareCheckStateFalseCoverageBatch35(t *testing.T) {
	if got := compareCheckState(false); got != 0 {
		t.Fatalf("compareCheckState(false) = %d, want 0", got)
	}
}

func TestCompareCaptionWidthCoverageBatch35(t *testing.T) {
	if got := compareCaptionWidth(3, "Caption"); got <= 3+4 {
		t.Fatalf("compareCaptionWidth returned %d, want caption width plus indent and prefix", got)
	}
}

func TestCompareCaptionWidthStripsMnemonicCoverageBatch35(t *testing.T) {
	plain := compareCaptionWidth(0, "Caption")
	mnemonic := compareCaptionWidth(0, "&Caption")
	if plain != mnemonic {
		t.Fatalf("mnemonic width = %d, plain width = %d; want equal", mnemonic, plain)
	}
}

func TestCompareRadioWidthEmptyCoverageBatch35(t *testing.T) {
	if got := compareRadioWidth(2, nil); got != 8 {
		t.Fatalf("compareRadioWidth empty = %d, want 8", got)
	}
}

func TestCompareRadioWidthUsesWidestLabelCoverageBatch35(t *testing.T) {
	if got := compareRadioWidth(2, []string{"Short", "Much longer"}); got <= compareRadioWidth(2, []string{"Short"}) {
		t.Fatalf("radio width did not grow for the widest label: %d", got)
	}
}

func TestNormalizeMenuSeparatorsDropsEdgeSeparatorsCoverageBatch35(t *testing.T) {
	items := []vtui.MenuItem{
		{Separator: true},
		{Text: "first"},
		{Separator: true},
		{Text: "last"},
		{Separator: true},
	}
	got := normalizeMenuSeparators(items)
	if len(got) != 3 || got[0].Text != "first" || !got[1].Separator || got[2].Text != "last" {
		t.Fatalf("normalized edge separators = %#v, want item/separator/item", got)
	}
}

func TestNormalizeMenuSeparatorsCollapsesRunsCoverageBatch35(t *testing.T) {
	items := []vtui.MenuItem{{Text: "first"}, {Separator: true}, {Separator: true}, {Text: "last"}}
	got := normalizeMenuSeparators(items)
	if len(got) != 3 || !got[1].Separator {
		t.Fatalf("normalized separator run = %#v, want one separator", got)
	}
}

func TestNormalizeMenuSeparatorsNormalizesNestedItemsCoverageBatch35(t *testing.T) {
	items := []vtui.MenuItem{{Text: "group", SubItems: []vtui.MenuItem{{Separator: true}, {Text: "child"}, {Separator: true}}}}
	got := normalizeMenuSeparators(items)
	if len(got) != 1 || len(got[0].SubItems) != 1 || got[0].SubItems[0].Text != "child" {
		t.Fatalf("normalized nested items = %#v, want one child", got)
	}
}

func TestNormalizeMenuSeparatorsPreservesSubmenuContentCoverageBatch35(t *testing.T) {
	items := []vtui.MenuItem{{Text: "group", SubItems: []vtui.MenuItem{{Text: "one"}, {Separator: true}, {Text: "two"}}}}
	got := normalizeMenuSeparators(items)
	if len(got) != 1 || len(got[0].SubItems) != 3 || !got[0].SubItems[1].Separator {
		t.Fatalf("normalized submenu = %#v, want two children separated once", got)
	}
}
