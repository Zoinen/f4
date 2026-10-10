//go:build !lite

package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
)

// The installed catalog (far2l's colorer/configs) lists its colour styles
// through external XML entities such as &catalog-rgb;. The encoding/xml reader
// this used to be stopped at the first one, and the Settings Center offered no
// styles at all. Colorer reads the catalog now, and brings the user's own
// colour styles along (issue #277).
func TestSettingsSchemeEnumerationAndWorktreeIdentity(t *testing.T) {
	oldCatalog, oldUserHrd := config.App.EditorColorerCatalog, config.App.EditorColorerUserHrd
	defer func() { config.App.EditorColorerCatalog, config.App.EditorColorerUserHrd = oldCatalog, oldUserHrd }()
	dir := t.TempDir()
	config.App.EditorColorerCatalog = dir
	base := filepath.Join(dir, "base")
	_ = os.MkdirAll(filepath.Join(base, "hrd"), 0700)
	_ = os.WriteFile(filepath.Join(base, "catalog.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE catalog [
    <!ENTITY hrd "hrd">
    <!ENTITY catalog-rgb SYSTEM "hrd/catalog-rgb.xml">
]>
<catalog xmlns="http://colorer.github.io/schema/v1/catalog">
    <hrc-sets/>
    <hrd-sets>
        &catalog-rgb;
    </hrd-sets>
</catalog>
`), 0600)
	_ = os.WriteFile(filepath.Join(base, "hrd", "catalog-rgb.xml"), []byte(`
        <hrd class="rgb" name="first" description="First scheme">
            <location link="&hrd;/first.hrd"/>
        </hrd>
        <hrd class="text" name="other" description="Other">
            <location link="&hrd;/first.hrd"/>
        </hrd>
`), 0600)
	_ = os.WriteFile(filepath.Join(base, "hrd", "first.hrd"), []byte(`<hrd xmlns="http://colorer.sf.net/2003/hrd"/>`), 0600)
	user := t.TempDir()
	_ = os.WriteFile(filepath.Join(user, "mine.hrd"), []byte(`<hrd xmlns="http://colorer.sf.net/2003/hrd" class="rgb" name="mine" description="My style"/>`), 0600)
	config.App.EditorColorerUserHrd = user

	names := map[string]bool{}
	for _, scheme := range settingsColorerSchemes() {
		names[scheme.Name] = true
	}
	if !names["first"] || !names["mine"] || names["other"] {
		t.Fatalf("styles listed: %v; want first and mine, and no text-class style", names)
	}
}

// A user's hrd-sets file can live anywhere on disk, and a <location link> in
// it names a sibling .hrd file relative to that file, not to catalog.xml
// (f4#277, reported by montoner0). Unlike the folder-of-.hrd-files case
// above, an hrd-sets file's <location link> is real indirection Colorer
// itself resolves, so this exercises the fix through an actual session
// instead of only the rewrite helper (colorer_userhrd_location_test.go).
func TestSettingsSchemeEnumerationUserHrdLocationLinkElsewhere(t *testing.T) {
	oldCatalog, oldUserHrd := config.App.EditorColorerCatalog, config.App.EditorColorerUserHrd
	defer func() { config.App.EditorColorerCatalog, config.App.EditorColorerUserHrd = oldCatalog, oldUserHrd }()
	dir := t.TempDir()
	config.App.EditorColorerCatalog = dir
	base := filepath.Join(dir, "base")
	_ = os.MkdirAll(filepath.Join(base, "hrd"), 0700)
	_ = os.WriteFile(filepath.Join(base, "catalog.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<catalog xmlns="http://colorer.github.io/schema/v1/catalog">
    <hrc-sets/>
    <hrd-sets/>
</catalog>
`), 0600)

	// The user's own files live in a directory with no relation to dir/base,
	// exactly the montoner0 case: "пользовательские файлы могут лежать где
	// угодно" ("user files can live anywhere").
	user := t.TempDir()
	_ = os.WriteFile(filepath.Join(user, "elsewhere.hrd"), []byte(`<hrd xmlns="http://colorer.sf.net/2003/hrd" class="rgb" name="elsewhere" description="Elsewhere"/>`), 0600)
	_ = os.WriteFile(filepath.Join(user, "styles.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<hrd-sets>
    <hrd class="rgb" name="mine" description="My style">
        <location link="elsewhere.hrd"/>
    </hrd>
</hrd-sets>
`), 0600)
	config.App.EditorColorerUserHrd = filepath.Join(user, "styles.xml")

	names := map[string]bool{}
	for _, scheme := range settingsColorerSchemes() {
		names[scheme.Name] = true
	}
	if !names["mine"] {
		t.Fatalf("styles listed: %v; want mine, whose hrd-sets file links to elsewhere.hrd relative to itself", names)
	}
}
