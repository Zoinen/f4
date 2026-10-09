package editor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestColorerParamIntAndHex(t *testing.T) {
	for in, want := range map[string]int{"5000": 5000, " 12abc": 12, "-3": -3, "": 7, "x1": 7} {
		if got := colorerParamInt(in, 7); got != want {
			t.Errorf("colorerParamInt(%q) = %d, want %d", in, got, want)
		}
	}
	for in, want := range map[string]int{"#102030": 0x102030, "ff": 0xff, "0x10": 0x10, "": -1, "zz": -1} {
		if got := colorerParamHex(in, -1); got != want {
			t.Errorf("colorerParamHex(%q) = %#x, want %#x", in, got, want)
		}
	}
}

func TestColorerTypeSettingsHelpers(t *testing.T) {
	s := colorerTypeSettings{maxLineLength: 3, showCross: "vertical", fore: 0x112233, foreSet: true}
	if got := s.truncate("aЖbcd"); got != "aЖb" {
		t.Errorf("truncate = %q", got)
	}
	if got := (colorerTypeSettings{}).truncate("abcdef"); got != "abcdef" {
		t.Errorf("no limit truncated to %q", got)
	}
	if h, v := s.crossAxes(); h || !v {
		t.Errorf("vertical axes %v %v", h, v)
	}
	attr := s.baseAttr(0)
	if vtui.GetRGBFore(attr) != 0x112233 {
		t.Errorf("base fore %#x", vtui.GetRGBFore(attr))
	}

	// The zero value is what a highlighter holds until its session has been
	// read, and it must leave the editor's own colours alone rather than
	// force RGB 000000 on both halves of the attribute.
	marker := vtui.SetRGBBack(vtui.SetRGBFore(0, 0xC0FFEE), 0x123456)
	if got := (colorerTypeSettings{}).baseAttr(marker); got != marker {
		t.Errorf("zero settings changed the base attribute: %#x -> %#x", marker, got)
	}
}

// FarEditor::reloadTypeSettings from a real session: "default" from
// plug/hrcsettings.xml, the file's type on top from the user's profile.
func TestReadColorerTypeSettings(t *testing.T) {
	config.GetF4ConfigDir()
	old := config.CachedF4ConfigDir
	config.CachedF4ConfigDir = t.TempDir()
	t.Cleanup(func() { config.CachedF4ConfigDir = old })
	ResetColorerSessions()
	t.Cleanup(ResetColorerSessions)

	configs := checkConfigs(t)
	files := map[string]string{
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
		"plug/hrcsettings.xml": `<hrc-settings><prototype name="default">
<param name="show-cross" value="none"/>
<param name="maxlinelength" value="5000"/>
<param name="fullback" value="yes"/>
<param name="default-fore" value=""/>
<param name="default-back" value=""/>
</prototype></hrc-settings>`,
	}
	for rel, content := range files {
		path := filepath.Join(configs, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)
	if err := saveColorerProfile(map[string]map[string]string{"pairtest": {"show-cross": "both", "fullback": "no", "default-back": "#202020", "maxlinelength": "80"}}); err != nil {
		t.Fatal(err)
	}

	session, err := acquireCancelableColorerSession(context.Background(), ColorerSource{ConfigsDir: configs, UserHRC: user})
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer session.Close()
	if ok, err := session.SetFileType("pairtest"); err != nil || !ok {
		t.Fatalf("SetFileType: %v, %v", ok, err)
	}
	got := readColorerTypeSettings(session)
	want := colorerTypeSettings{maxLineLength: 80, plainEOL: true, showCross: "both", back: 0x202020, backSet: true}
	if got != want {
		t.Errorf("settings %+v, want %+v", got, want)
	}

	if ok, _ := session.SetFileType("default"); !ok {
		t.Fatal("SetFileType(default)")
	}
	if got := readColorerTypeSettings(session); got != (colorerTypeSettings{maxLineLength: 5000, showCross: "none"}) {
		t.Errorf("default settings %+v", got)
	}
}

// default-back alone must reach the attribute without dragging a foreground
// colour along, and a value that does not parse must leave the colour unset
// rather than turn it into a real black.
func TestColorerTypeSettings_BackWithoutFore(t *testing.T) {
	marker := vtui.SetRGBFore(0, 0xC0FFEE)
	s := colorerTypeSettings{back: 0x202020, backSet: true}
	attr := s.baseAttr(marker)
	if got := vtui.GetRGBBack(attr); got != 0x202020 {
		t.Errorf("base back %#x, want 0x202020", got)
	}
	if got := vtui.GetRGBFore(attr); got != 0xC0FFEE {
		t.Errorf("an unset foreground was changed to %#x", got)
	}
}

func TestColorerParamHexValue(t *testing.T) {
	for _, tc := range []struct {
		in    string
		want  int
		valid bool
	}{
		{in: "#102030", want: 0x102030, valid: true},
		{in: "0xFF", want: 0xff, valid: true},
		{in: "ff", want: 0xff, valid: true},
		{in: ""},
		{in: "zz"},
		// Longer than a uint32 takes: ParseUint refuses it, and a colour that
		// cannot be read stays unset.
		{in: "1122334455"},
	} {
		got, valid := colorerParamHexValue(tc.in)
		if valid != tc.valid || (valid && got != tc.want) {
			t.Errorf("colorerParamHexValue(%q) = %#x, %v; want %#x, %v", tc.in, got, valid, tc.want, tc.valid)
		}
	}
}

// default-fore is read the same way default-back is; only the profile in the
// other test happens to set the background.
func TestReadColorerTypeSettings_ForegroundFromTheProfile(t *testing.T) {
	config.GetF4ConfigDir()
	old := config.CachedF4ConfigDir
	config.CachedF4ConfigDir = t.TempDir()
	t.Cleanup(func() { config.CachedF4ConfigDir = old })
	ResetColorerSessions()
	t.Cleanup(ResetColorerSessions)

	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)
	settingsPath := filepath.Join(t.TempDir(), "hrcsettings.xml")
	if err := os.WriteFile(settingsPath, []byte(`<hrc-settings><prototype name="pairtest"><param name="default-fore" value="#334455"/></prototype></hrc-settings>`), 0o600); err != nil {
		t.Fatal(err)
	}

	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRC: user, UserHRCSettings: settingsPath}
	session, err := acquireCancelableColorerSession(context.Background(), src)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer session.Close()
	if ok, err := session.SetFileType("pairtest"); err != nil || !ok {
		t.Fatalf("SetFileType: %v, %v", ok, err)
	}

	got := readColorerTypeSettings(session)
	if !got.foreSet || got.fore != 0x334455 {
		t.Errorf("settings %+v, want fore 0x334455 set", got)
	}
	if got.backSet {
		t.Errorf("an unset background was read as set: %+v", got)
	}
}
