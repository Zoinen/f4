package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

// installColorerTestRegions activates a region graph for the duration of a
// test and restores the previous one afterwards.
func installColorerTestRegions(t *testing.T, parents map[string]string) {
	t.Helper()

	hrcMu.Lock()
	oldParents, oldOrphans, oldDir := hrcParents, hrcOrphans, hrcDir
	hrcParents, hrcOrphans, hrcDir = parents, nil, "test"
	hrcMu.Unlock()

	t.Cleanup(func() {
		hrcMu.Lock()
		hrcParents, hrcOrphans, hrcDir = oldParents, oldOrphans, oldDir
		hrcMu.Unlock()
	})
}

// writeColorerHrcFixture builds a two file schema tree. The base one declares
// a legacy encoding on purpose: a fair number of the shipped schemas do, and
// they used to be skipped without a word.
func writeColorerHrcFixture(t *testing.T) string {
	t.Helper()

	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(filepath.Join(base, "hrc", "auto"), 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}

	def := `<?xml version="1.0" encoding="windows-1251"?>
<hrc version="take5">
  <type name="def">
    <region name="Text"/>
    <region name="Syntax"/>
    <region name="Comment" parent="Syntax"/>
    <region name="CommentContent" parent="Comment"/>
    <region name="PairStart" parent="Syntax"/>
  </type>
</hrc>`
	clang := `<?xml version="1.0" encoding="UTF-8"?>
<hrc version="take5">
  <type name="c">
    <import type="def"/>
    <region name="BracketStart" parent="def:PairStart"/>
    <region name="LineComment" parent="Comment"/>
  </type>
</hrc>`

	if err := os.WriteFile(filepath.Join(base, "hrc", "def.hrc"), []byte(def), 0644); err != nil {
		t.Fatalf("Cannot write the base schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "hrc", "auto", "c.hrc"), []byte(clang), 0644); err != nil {
		t.Fatalf("Cannot write the c schema: %v", err)
	}
	return base
}

func TestColorerRegions_ParentsComeFromTheSchemas(t *testing.T) {
	parents, _ := scanColorerRegions(writeColorerHrcFixture(t))

	want := map[string]string{
		"def:comment":        "def:syntax",
		"def:commentcontent": "def:comment",
		"def:pairstart":      "def:syntax",
		"c:bracketstart":     "def:pairstart",
		"c:linecomment":      "def:comment",
	}
	for name, parent := range want {
		if got := parents[name]; got != parent {
			t.Errorf("Expected %q to inherit from %q, got %q", name, parent, got)
		}
	}
	if _, ok := parents["def:text"]; ok {
		t.Error("Expected a region without a parent to stay out of the graph")
	}
}

func TestColorerRegions_UnresolvableParentIsDropped(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(filepath.Join(base, "hrc"), 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}
	schema := `<hrc version="take5">
  <type name="c">
    <region name="Bracket" parent="Nowhere"/>
  </type>
</hrc>`
	if err := os.WriteFile(filepath.Join(base, "hrc", "c.hrc"), []byte(schema), 0644); err != nil {
		t.Fatalf("Cannot write the schema: %v", err)
	}

	if parents, _ := scanColorerRegions(base); len(parents) != 0 {
		t.Errorf("Expected a parent that belongs to no type to be dropped, got %v", parents)
	}
}

func TestColorerScheme_ResolvesThroughTheParentChain(t *testing.T) {
	installColorerTestRegions(t, map[string]string{
		"c:linecomment":      "def:commentcontent",
		"def:commentcontent": "def:comment",
		"def:comment":        "def:syntax",
	})
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:comment": {fore: 0x123456, hasFore: true},
	})

	if got := vtui.GetRGBFore(getColorerAttr("c:LineComment", 0)); got != 0x123456 {
		t.Errorf("Expected the color of the nearest declared parent, got %06X", got)
	}
}
func TestColorerScheme_AccumulatesStylesThroughParentChain(t *testing.T) {
	installColorerTestRegions(t, map[string]string{
		"c:linecomment":      "def:commentcontent",
		"def:commentcontent": "def:comment",
		"def:comment":        "def:syntax",
	})
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:comment":        {fore: 0x123456, hasFore: true},
		"def:commentcontent": {back: 0x654321, hasBack: true, style: colorerStyleBold, hasStyle: true},
		"c:linecomment":      {fore: 0x999999, hasFore: true},
	})

	style, found := colorerSchemeStyle("c:LineComment")
	if !found {
		t.Fatal("Expected to find style")
	}
	if style.fore != 0x999999 || style.back != 0x654321 || style.style != colorerStyleBold {
		t.Errorf("Expected accumulated style, got %+v", style)
	}
}

func TestColorerScheme_UnknownRegionStaysTransparent(t *testing.T) {
	installColorerTestRegions(t, map[string]string{
		"def:comment": "def:syntax",
	})
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:comment": {fore: 0x123456, hasFore: true},
	})

	base := vtui.SetRGBBoth(0, 0xD3D7CF, 0x000000)
	// "keyword" sits in the built-in color map, and falling back to it is what
	// used to drop foreign colors into a monochrome style.
	if got := getColorerAttr("c:Keyword", base); got != base {
		t.Errorf("Expected a region the style leaves out to stay transparent, got %016X", got)
	}
}

func TestColorerScheme_StyleBitsBecomeAttributes(t *testing.T) {
	style, ok := parseColorerStyleBits("5")
	if !ok || style != 5 {
		t.Fatalf("Expected the style bits 5, got %d (ok=%v)", style, ok)
	}

	attr := applyColorerStyle(0, colorerRegionStyle{style: 5, hasStyle: true})
	if attr&vtui.ForegroundIntensity == 0 {
		t.Error("Expected the bold bit to become an intensity attribute")
	}
	if attr&vtui.CommonLvbUnderscore == 0 {
		t.Error("Expected the underline bit to become an underscore attribute")
	}
	if attr&vtui.CommonLvbStrikeout != 0 {
		t.Error("Expected the strikeout attribute to stay clear")
	}
}

func TestColorerScheme_StyleIsReadFromTheHrd(t *testing.T) {
	base := t.TempDir()
	hrd := `<?xml version="1.0" encoding="UTF-8"?>
<hrd>
  <assign name="def:Keyword" fore="#FFFFFF" style="1"/>
  <define name="def:Error" back="#FF0000"/>
  <parameters name="def:NotAnAssign" fore="#00FF00"/>
</hrd>`
	path := filepath.Join(base, "style.hrd")
	if err := os.WriteFile(path, []byte(hrd), 0644); err != nil {
		t.Fatalf("Cannot write the color style: %v", err)
	}

	styles := loadColorerScheme(path)
	if got := styles["def:keyword"]; !got.hasStyle || got.style != 1 {
		t.Errorf("Expected the bold style bit on def:Keyword, got %+v", got)
	}
	if got := styles["def:error"]; !got.hasBack || got.back != 0xFF0000 {
		t.Errorf("Expected <define> to be read as well, got %+v", got)
	}
	if _, ok := styles["def:notanassign"]; ok {
		t.Error("Expected an element that is not an assign to be ignored")
	}
}

func TestColorerScheme_LabelPrefersTheDescription(t *testing.T) {
	described := ColorerScheme{Name: "grayscale", Description: "Grayscale (colour neutral)"}
	if got := colorerSchemeLabel(described); got != "Grayscale (colour neutral)" {
		t.Errorf("Expected the description, got %q", got)
	}
	if got := colorerSchemeLabel(ColorerScheme{Name: "grayscale"}); got != "grayscale" {
		t.Errorf("Expected the name as a fallback, got %q", got)
	}
}
func TestColorerRegions_FragmentRegionsAreKeptByLocalName(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(filepath.Join(base, "hrc", "gen"), 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}
	// A fragment has no <type> of its own: it is pulled into one through an
	// external entity, which the decoder never expands.
	fragment := `<region xmlns="http://colorer.sf.net/2003/hrc" name="php.function" parent="def:FunctionKeyword"/>
<region xmlns="http://colorer.sf.net/2003/hrc" name="php.const" parent="def:Constant"/>`
	if err := os.WriteFile(filepath.Join(base, "hrc", "gen", "php-gen.ent.hrc"), []byte(fragment), 0644); err != nil {
		t.Fatalf("Cannot write the fragment: %v", err)
	}

	parents, orphans := scanColorerRegions(base)
	if len(parents) != 0 {
		t.Errorf("Expected no qualified names from a fragment, got %v", parents)
	}
	if got := orphans["php.function"]; got != "def:functionkeyword" {
		t.Errorf("Expected php.function to be kept, got %q", got)
	}
	if got := orphans["php.const"]; got != "def:constant" {
		t.Errorf("Expected every region of the fragment to be kept, got %q", got)
	}
}

func TestColorerScheme_NameRulesRunBehindTheGraph(t *testing.T) {
	installColorerTestRegions(t, map[string]string{
		"def:comment": "def:syntax",
	})
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:comment": {fore: 0x123456, hasFore: true},
	})

	// The graph knows nothing about this one, so the prefix rule has to carry
	// it. An incomplete graph must not switch the name rules off.
	if got := vtui.GetRGBFore(getColorerAttr("def:CommentContent", 0)); got != 0x123456 {
		t.Errorf("Expected the prefix rule to still reach def:Comment, got %06X", got)
	}
}

func TestColorerScheme_ForeignTypeMatchesByLocalName(t *testing.T) {
	installColorerTestRegions(t, map[string]string{})
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:tag": {fore: 0x729FCF, hasFore: true},
	})

	if got := vtui.GetRGBFore(getColorerAttr("html:htmlOpenTag", 0)); got != 0x729FCF {
		t.Errorf("Expected a foreign tag region to reach def:Tag, got %06X", got)
	}
}
