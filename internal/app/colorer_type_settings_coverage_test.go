package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
)

func TestActionColorerTypeSettingsBuildsCatalogDialog(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	configs := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		name := filepath.Join(configs, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("base/catalog.xml", `<?xml version="1.0" encoding="UTF-8"?>
<catalog xmlns="http://colorer.github.io/schema/v1/catalog">
  <hrc-sets><location link="hrc/default.hrc"/></hrc-sets>
  <hrd-sets><hrd class="rgb" name="default" description="Default"><location link="hrd/default.hrd"/></hrd></hrd-sets>
</catalog>
`)
	write("base/hrc/default.hrc", `<?xml version="1.0" encoding="UTF-8"?>
<hrc version="take5" xmlns="http://colorer.sf.net/2003/hrc">
  <prototype name="demo" group="tests" description="Demo type"><location link="default.hrc"/></prototype>
  <type name="demo"><scheme name="demo"/></type>
</hrc>
`)
	write("base/hrd/default.hrd", `<hrd xmlns="http://colorer.sf.net/2003/hrd"/>`)
	write("plug/hrcsettings.xml", `<hrc-settings><prototype name="demo"><param name="hotkey" value="D" description="Menu key"/></prototype></hrc-settings>`)
	editor.ResetColorerSessions()
	t.Cleanup(editor.ResetColorerSessions)

	actionColorerTypeSettings(editor.ColorerSource{ConfigsDir: configs})
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		t.Fatal("type settings dialog was not opened")
	}
	top.Close()
}
