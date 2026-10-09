package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestColorerParamChoicesAndEdit(t *testing.T) {
	cross := ColorerParam{Name: "show-cross", Default: "none"}
	if choices, fixed := ColorerParamChoices(cross); !fixed || len(choices) != 5 || choices[4] != "<default-none>" {
		t.Errorf("show-cross choices %v fixed %v", choices, fixed)
	}
	if choices, fixed := ColorerParamChoices(ColorerParam{Name: "maxlinelength", Default: "5000"}); fixed || len(choices) != 1 || choices[0] != "<default-5000>" {
		t.Errorf("maxlinelength choices %v fixed %v", choices, fixed)
	}
	if ColorerParamText(cross) != "<default-none>" {
		t.Errorf("text %q", ColorerParamText(cross))
	}

	if _, changed := ColorerParamEdit(&cross, "<default-none>"); changed {
		t.Error("picking the default without a user value is a change")
	}
	if v, changed := ColorerParamEdit(&cross, " both "); !changed || v == nil || *v != "both" || !cross.UserSet {
		t.Errorf("setting both: %v %v %+v", v, changed, cross)
	}
	if _, changed := ColorerParamEdit(&cross, "both"); changed {
		t.Error("the same value again is a change")
	}
	if v, changed := ColorerParamEdit(&cross, "<default-none>"); !changed || v != nil || cross.UserSet || cross.Value != "none" {
		t.Errorf("taking the value back: %v %v %+v", v, changed, cross)
	}
}

func TestSaveColorerParams(t *testing.T) {
	config.GetF4ConfigDir()
	old := config.CachedF4ConfigDir
	config.CachedF4ConfigDir = t.TempDir()
	t.Cleanup(func() { config.CachedF4ConfigDir = old })
	if err := saveColorerProfile(map[string]map[string]string{"json": {"favorite": "true", "hotkey": "J"}}); err != nil {
		t.Fatal(err)
	}
	both := "both"
	if err := SaveColorerParams(map[string]map[string]*string{"json": {"hotkey": nil, "show-cross": &both}, "c": {"favorite": nil}}); err != nil {
		t.Fatal(err)
	}
	got := loadColorerProfile()
	if got["json"]["favorite"] != "true" || got["json"]["show-cross"] != "both" || got["json"]["hotkey"] != "" || len(got["c"]) != 0 {
		t.Errorf("profile %v", got)
	}
}

// The dialog's data from a real session: parameters come from the
// configuration's plug/hrcsettings.xml, user values from HrcSettings.ini.
func TestLoadColorerTypeParams(t *testing.T) {
	config.GetF4ConfigDir()
	old := config.CachedF4ConfigDir
	config.CachedF4ConfigDir = t.TempDir()
	t.Cleanup(func() { config.CachedF4ConfigDir = old })
	ResetColorerSessions()
	t.Cleanup(ResetColorerSessions)

	// HRC settings reach only types the catalog already has when they load,
	// as in Colorer's HrcLibrary::Impl::updatePrototype, so "default" is in
	// the catalog itself, as in far2l's proto.hrc.
	configs := checkConfigs(t)
	for rel, content := range map[string]string{
		"base/catalog.xml": `<?xml version="1.0" encoding="UTF-8"?>
<catalog xmlns="http://colorer.github.io/schema/v1/catalog">
  <hrc-sets><location link="hrc/default.hrc"/></hrc-sets>
  <hrd-sets>
    <hrd class="rgb" name="default" description="Default"><location link="hrd/default.hrd"/></hrd>
  </hrd-sets>
</catalog>
`,
		"base/hrc/default.hrc": `<?xml version="1.0" encoding="UTF-8"?>
<hrc version="take5" xmlns="http://colorer.sf.net/2003/hrc">
  <prototype name="default" group="other" description="default type">
    <location link="default.hrc"/>
  </prototype>
  <type name="default"><scheme name="default"/></type>
</hrc>
`,
	} {
		path := filepath.Join(configs, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(configs, "plug"), 0o700); err != nil {
		t.Fatal(err)
	}
	settings := `<hrc-settings><prototype name="default">
<param name="show-cross" value="none" description="Visibility of the cross"/>
<param name="hotkey" value="" description="Menu key"/>
</prototype></hrc-settings>`
	if err := os.WriteFile(filepath.Join(configs, "plug", "hrcsettings.xml"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)
	if err := saveColorerProfile(map[string]map[string]string{"pairtest": {"show-cross": "both"}}); err != nil {
		t.Fatal(err)
	}

	types, err := LoadColorerTypeParams(ColorerSource{ConfigsDir: configs, UserHRC: user})
	if err != nil {
		t.Fatalf("LoadColorerTypeParams: %v", err)
	}
	for _, typ := range types {
		if typ.Name != "pairtest" {
			continue
		}
		params := map[string]ColorerParam{}
		for _, p := range typ.Params {
			params[p.Name] = p
		}
		if p := params["show-cross"]; p.Value != "both" || !p.UserSet || p.Default != "none" || p.Description == "" {
			t.Errorf("show-cross %+v", p)
		}
		if p := params["hotkey"]; p.UserSet || p.Description != "Menu key" {
			t.Errorf("hotkey %+v", p)
		}
		return
	}
	t.Fatalf("pairtest not among %d types", len(types))
}
