package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func writeColorerCatalogFixture(t *testing.T) string {
	t.Helper()

	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(filepath.Join(base, "hrd", "rgb"), 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}

	catalog := `<?xml version="1.0" encoding="UTF-8"?>
<catalog>
  <hrd-sets>
    <hrd class="console" name="console" description="Console style">
      <location link="hrd/console/console.hrd"/>
    </hrd>
    <hrd class="rgb" name="test" description="Test style">
      <location link="hrd/rgb/test.hrd"/>
    </hrd>
  </hrd-sets>
</catalog>`
	hrd := `<?xml version="1.0" encoding="UTF-8"?>
<hrd>
  <assign name="def:Comment" fore="#123456"/>
  <assign name="def:String" fore="#00FF00" back="#101010"/>
  <assign name="def:Ignored"/>
</hrd>`

	catalogPath := filepath.Join(base, "catalog.xml")
	if err := os.WriteFile(catalogPath, []byte(catalog), 0644); err != nil {
		t.Fatalf("Cannot write the catalog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "hrd", "rgb", "test.hrd"), []byte(hrd), 0644); err != nil {
		t.Fatalf("Cannot write the color style: %v", err)
	}
	return catalogPath
}

// writeColorerEntityCatalogFixture builds a FarColorer-shaped catalog: the
// main file declares the hrc locations only and pulls the hrd sets in with an
// external entity. The entity path is returned separately, so a test can point
// the environment variable at it or leave it undefined.
func writeColorerEntityCatalogFixture(t *testing.T, entityLink string) (string, string) {
	t.Helper()

	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(filepath.Join(base, "hrd", "rgb"), 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}

	catalog := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE catalog [
  <!ENTITY hrd-sets SYSTEM "` + entityLink + `">
]>
<catalog>
  <hrc-sets>
    <location link="hrc/proto/base.xml"/>
  </hrc-sets>
  &hrd-sets;
</catalog>`
	hrdSets := `<hrd-sets>
  <hrd class="console" name="console" description="Console style">
    <location link="hrd/console/console.hrd"/>
  </hrd>
  <hrd class="rgb" name="zebra" description="Zebra style">
    <location link="hrd/rgb/zebra.hrd"/>
  </hrd>
  <hrd class="rgb" name="amber" description="Amber style">
    <location link="hrd/rgb/amber.hrd"/>
  </hrd>
</hrd-sets>`

	catalogPath := filepath.Join(base, "catalog.xml")
	entityPath := filepath.Join(base, "hrd", "catalog-rgb.xml")
	if err := os.WriteFile(catalogPath, []byte(catalog), 0644); err != nil {
		t.Fatalf("Cannot write the catalog: %v", err)
	}
	if err := os.WriteFile(entityPath, []byte(hrdSets), 0644); err != nil {
		t.Fatalf("Cannot write the hrd sets: %v", err)
	}
	return catalogPath, entityPath
}

func assertEntityCatalogSchemes(t *testing.T, catalogPath string) {
	t.Helper()

	schemes := parseColorerCatalog(catalogPath)
	if len(schemes) != 2 {
		t.Fatalf("Expected two rgb styles behind the entity, got %v", schemes)
	}
	if schemes[0].Name != "zebra" || schemes[1].Name != "amber" {
		t.Fatalf("Expected the styles in the catalog order, got %v", schemes)
	}

	wantPath := filepath.Join(filepath.Dir(catalogPath), "hrd", "rgb", "zebra.hrd")
	if schemes[0].Path != wantPath {
		t.Errorf("Expected the location resolved against the catalog as %q, got %q", wantPath, schemes[0].Path)
	}
}

func TestColorer_ParseCatalogFollowsEntity(t *testing.T) {
	// The variable is undefined, so the remainder of the path has to be
	// looked up next to the catalog itself.
	catalogPath, _ := writeColorerEntityCatalogFixture(t, "env:$F4_COLORER_TEST_HOME/hrd/catalog-rgb.xml")
	assertEntityCatalogSchemes(t, catalogPath)
}

func TestColorer_ParseCatalogExpandsEntityEnv(t *testing.T) {
	catalogPath, entityPath := writeColorerEntityCatalogFixture(t, "env:$F4_COLORER_TEST_HOME/catalog-rgb.xml")
	t.Setenv("F4_COLORER_TEST_HOME", filepath.Dir(entityPath))
	assertEntityCatalogSchemes(t, catalogPath)
}

func TestColorer_ParseCatalogSurvivesBrokenEntity(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}

	catalog := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE catalog [
  <!ENTITY missing SYSTEM "env:$F4_COLORER_TEST_HOME/nowhere.xml">
]>
<catalog>
  <hrd-sets>
    &missing;
    <hrd class="rgb" name="test" description="Test style">
      <location link="hrd/rgb/test.hrd"/>
    </hrd>
  </hrd-sets>
</catalog>`
	catalogPath := filepath.Join(base, "catalog.xml")
	if err := os.WriteFile(catalogPath, []byte(catalog), 0644); err != nil {
		t.Fatalf("Cannot write the catalog: %v", err)
	}

	schemes := parseColorerCatalog(catalogPath)
	if len(schemes) != 1 || schemes[0].Name != "test" {
		t.Fatalf("An unresolvable entity must not hide the inline styles, got %v", schemes)
	}
}

func TestColorer_ParseCatalogAndScheme(t *testing.T) {
	inlinePath := writeColorerCatalogFixture(t)

	schemes := parseColorerCatalog(inlinePath)
	if len(schemes) != 1 || schemes[0].Name != "test" {
		t.Fatalf("Expected exactly one rgb style, got %v", schemes)
	}

	styles := loadColorerScheme(schemes[0].Path)
	if len(styles) != 2 {
		t.Fatalf("Expected two defined regions, got %d", len(styles))
	}
	if style := styles["def:comment"]; !style.hasFore || style.fore != 0x123456 || style.hasBack {
		t.Errorf("Unexpected style for def:Comment: %+v", style)
	}
	if style := styles["def:string"]; !style.hasBack || style.back != 0x101010 {
		t.Errorf("Unexpected style for def:String: %+v", style)
	}
}

func TestColorer_ParseColorerColor(t *testing.T) {
	cases := []struct {
		value string
		want  uint32
		ok    bool
	}{
		{"#AABBCC", 0xAABBCC, true},
		{"0xAABBCC", 0xAABBCC, true},
		{"AABBCC", 0xAABBCC, true},
		{"#FFAABBCC", 0xAABBCC, true},
		{"0x7", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseColorerColor(c.value)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Value %q parsed to %06X, %v; expected %06X, %v", c.value, got, ok, c.want, c.ok)
		}
	}
}

// installColorerTestScheme activates a color style for the duration of a test
// and restores the previous one afterwards.
func installColorerTestScheme(t *testing.T, styles map[string]colorerRegionStyle) {
	t.Helper()

	keys := make([]string, 0, len(styles))
	for key := range styles {
		keys = append(keys, key)
	}
	sortColorerKeys(keys)

	schemeMu.Lock()
	oldName, oldStyles, oldKeys, oldMemo := schemeName, schemeStyles, schemeKeys, schemeMemo
	schemeName = "test"
	schemeStyles = styles
	schemeKeys = keys
	schemeMemo = make(map[string]colorerRegionStyle)
	schemeMu.Unlock()

	t.Cleanup(func() {
		schemeMu.Lock()
		schemeName, schemeStyles, schemeKeys, schemeMemo = oldName, oldStyles, oldKeys, oldMemo
		schemeMu.Unlock()
	})
}

// useColorerHighlighter switches the editor over to Colorer and back.
func useColorerHighlighter(t *testing.T, background bool) {
	t.Helper()

	oldHighlighter, oldBackground := AppConfig.EditorHighlighter, AppConfig.EditorColorerBackground
	AppConfig.EditorHighlighter = "Colorer"
	AppConfig.EditorColorerBackground = background

	t.Cleanup(func() {
		AppConfig.EditorHighlighter = oldHighlighter
		AppConfig.EditorColorerBackground = oldBackground
	})
}

func TestColorer_EditorBaseAttrFollowsDefText(t *testing.T) {
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:text": {fore: 0x00FF00, back: 0x101010, hasFore: true, hasBack: true},
	})
	useColorerHighlighter(t, true)

	base := vtui.SetRGBBoth(0, 0xD3D7CF, 0x000000)
	got := ColorerEditorBaseAttr(base)
	if vtui.GetRGBFore(got) != 0x00FF00 || vtui.GetRGBBack(got) != 0x101010 {
		t.Errorf("Expected def:Text to paint the editor, got %06X on %06X",
			vtui.GetRGBFore(got), vtui.GetRGBBack(got))
	}

	AppConfig.EditorColorerBackground = false
	if kept := ColorerEditorBaseAttr(base); kept != base {
		t.Error("The palette must be kept when the option is off")
	}

	AppConfig.EditorColorerBackground = true
	AppConfig.EditorHighlighter = "Chroma"
	if kept := ColorerEditorBaseAttr(base); kept != base {
		t.Error("The palette must be kept when Colorer is not the active highlighter")
	}
}

func TestColorer_EditorBaseAttrIgnoresMissingDefText(t *testing.T) {
	// def:Comment must not leak into the canvas through the substring rule.
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:comment": {fore: 0x123456, hasFore: true},
	})
	useColorerHighlighter(t, true)

	base := vtui.SetRGBBoth(0, 0xD3D7CF, 0x000000)
	if kept := ColorerEditorBaseAttr(base); kept != base {
		t.Error("A style without def:Text must not repaint the editor")
	}
}

func TestColorer_SchemeOverridesBuiltinColors(t *testing.T) {
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:comment": {fore: 0x123456, hasFore: true},
	})

	if got := vtui.GetRGBFore(getColorerAttr("def:CommentContent", 0)); got != 0x123456 {
		t.Errorf("Expected the color style to win, got %06X", got)
	}
	if got := getColorerAttr("unknown_region", 0); got != 0 {
		t.Errorf("Expected the base attribute for an unknown region, got %d", got)
	}
}
func TestColorer_CatalogEntityDeclarations(t *testing.T) {
	directive := `DOCTYPE catalog [
  <!ENTITY hrd-sets SYSTEM "env:$FAR_HOME/hrd/catalog-console.xml">
  <!ENTITY extra PUBLIC "-//Colorer//catalog//EN" 'hrd/catalog-rgb.xml'>
  <!ENTITY % params SYSTEM "params.xml">
  <!ENTITY inline "no path here">
]`

	want := []string{
		"env:$FAR_HOME/hrd/catalog-console.xml",
		"hrd/catalog-rgb.xml",
		"params.xml",
	}
	got := colorerCatalogEntities(directive)
	if len(got) != len(want) {
		t.Fatalf("Expected %d entity paths, got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Entity %d read as %q, expected %q", i, got[i], want[i])
		}
	}
}

func TestColorer_ExpandCatalogEnv(t *testing.T) {
	t.Setenv("F4_COLORER_TEST_HOME", "/opt/far2l")

	cases := []struct {
		value string
		want  string
	}{
		{"$F4_COLORER_TEST_HOME/hrd", "/opt/far2l/hrd"},
		{"${F4_COLORER_TEST_HOME}/hrd", "/opt/far2l/hrd"},
		{"%F4_COLORER_TEST_HOME%/hrd", "/opt/far2l/hrd"},
		{"$F4_COLORER_TEST_MISSING/hrd", "/hrd"},
		{"hrd/catalog.xml", "hrd/catalog.xml"},
	}
	for _, c := range cases {
		if got := expandColorerCatalogEnv(c.value); got != c.want {
			t.Errorf("Expanded %q to %q, expected %q", c.value, got, c.want)
		}
	}
}

func TestColorer_ParseCatalogHandlesCycleAndDuplicates(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(filepath.Join(base, "hrd"), 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}

	// The catalog refers back to itself, and the first entity file pulls in a
	// second one which redeclares a style the first file already provides.
	catalog := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE catalog [
  <!ENTITY first SYSTEM "hrd/first.xml">
  <!ENTITY loop SYSTEM "catalog.xml">
]>
<catalog>
  <hrc-sets>
    <location link="hrc/proto/base.xml"/>
  </hrc-sets>
  &first;
  &loop;
</catalog>`
	first := `<!DOCTYPE hrd-sets [
  <!ENTITY second SYSTEM "second.xml">
]>
<hrd-sets>
  <hrd class="rgb" name="amber" description="Amber style">
    <location link="hrd/rgb/amber.hrd"/>
  </hrd>
  &second;
</hrd-sets>`
	second := `<hrd-sets>
  <hrd class="rgb" name="Amber" description="Duplicate style">
    <location link="hrd/rgb/other.hrd"/>
  </hrd>
  <hrd class="rgb" name="zebra" description="Zebra style">
    <location link="hrd/rgb/zebra.hrd"/>
  </hrd>
</hrd-sets>`

	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Cannot write %q: %v", path, err)
		}
	}

	catalogPath := filepath.Join(base, "catalog.xml")
	write(catalogPath, catalog)
	write(filepath.Join(base, "hrd", "first.xml"), first)
	write(filepath.Join(base, "hrd", "second.xml"), second)

	schemes := parseColorerCatalog(catalogPath)
	if len(schemes) != 2 {
		t.Fatalf("Expected the duplicate to be dropped and the cycle to stop, got %v", schemes)
	}
	if schemes[0].Name != "amber" || schemes[1].Name != "zebra" {
		t.Fatalf("Expected amber and zebra, got %v", schemes)
	}

	wantPath := filepath.Join(base, "hrd", "rgb", "amber.hrd")
	if schemes[0].Path != wantPath {
		t.Errorf("Expected the first declaration to win with %q, got %q", wantPath, schemes[0].Path)
	}
}
func TestColorer_ParseCatalogResolvesEnvLocation(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(filepath.Join(base, "hrd", "rgb"), 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}
	t.Setenv("F4_COLORER_TEST_HOME", base)

	catalog := `<?xml version="1.0" encoding="UTF-8"?>
<catalog>
  <hrd-sets>
    <hrd class="rgb" name="grayscale" description="Grayscale style">
      <location link="env:$F4_COLORER_TEST_HOME/hrd/rgb/grayscale.hrd"/>
    </hrd>
  </hrd-sets>
</catalog>`
	style := `<?xml version="1.0" encoding="UTF-8"?>
<hrd>
  <assign name="def:Text" fore="#101010" back="#E0E0E0"/>
</hrd>`

	catalogPath := filepath.Join(base, "catalog.xml")
	stylePath := filepath.Join(base, "hrd", "rgb", "grayscale.hrd")
	if err := os.WriteFile(catalogPath, []byte(catalog), 0644); err != nil {
		t.Fatalf("Cannot write the catalog: %v", err)
	}
	if err := os.WriteFile(stylePath, []byte(style), 0644); err != nil {
		t.Fatalf("Cannot write the color style: %v", err)
	}

	schemes := parseColorerCatalog(catalogPath)
	if len(schemes) != 1 {
		t.Fatalf("Expected one rgb style, got %v", schemes)
	}
	if schemes[0].Path != stylePath {
		t.Fatalf("Expected the env: location resolved to %q, got %q", stylePath, schemes[0].Path)
	}

	styles := loadColorerScheme(schemes[0].Path)
	if len(styles) != 1 || !styles["def:text"].hasBack || styles["def:text"].back != 0xE0E0E0 {
		t.Errorf("Expected def:Text to be read from the style, got %+v", styles)
	}
}
func TestColorer_ParseCatalogWithInternalEntities(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(filepath.Join(base, "hrd", "rgb"), 0755); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}

	catalog := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE catalog [
  <!ENTITY hrd "hrd">
]>
<catalog>
  <hrd-sets>
    <hrd class="rgb" name="grayscale" description="Grayscale style">
      <location link="&hrd;/rgb/grayscale.hrd"/>
    </hrd>
  </hrd-sets>
</catalog>`
	style := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE hrd [
  <!ENTITY white "#FFFFFF">
]>
<hrd>
  <assign name="def:Text" fore="&white;" back="#000000"/>
</hrd>`

	catalogPath := filepath.Join(base, "catalog.xml")
	stylePath := filepath.Join(base, "hrd", "rgb", "grayscale.hrd")
	if err := os.WriteFile(catalogPath, []byte(catalog), 0644); err != nil {
		t.Fatalf("Cannot write the catalog: %v", err)
	}
	if err := os.WriteFile(stylePath, []byte(style), 0644); err != nil {
		t.Fatalf("Cannot write the color style: %v", err)
	}

	schemes := parseColorerCatalog(catalogPath)
	if len(schemes) != 1 {
		t.Fatalf("Expected one rgb style, got %v", schemes)
	}
	if !strings.HasSuffix(filepath.ToSlash(schemes[0].Path), "hrd/rgb/grayscale.hrd") {
		t.Fatalf("Expected path resolved via internal entity, got %q", schemes[0].Path)
	}

	styles := loadColorerScheme(schemes[0].Path)
	if len(styles) != 1 || !styles["def:text"].hasFore || styles["def:text"].fore != 0xFFFFFF {
		t.Errorf("Expected def:Text to be read with expanded entity, got %+v", styles)
	}
}

func TestColorer_SchemeGenerationChangesOnSwitch(t *testing.T) {
	installColorerTestScheme(t, map[string]colorerRegionStyle{
		"def:text": {fore: 0x00FF00, hasFore: true},
	})

	before := ColorerSchemeGeneration()
	// Switching back to the built-in colors must be visible to the caches.
	SetColorerScheme("")
	if after := ColorerSchemeGeneration(); after == before {
		t.Errorf("Expected the generation to change, it stayed at %d", after)
	}
}
