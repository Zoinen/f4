package visren

import (
	"context"
	"github.com/unxed/f4/vfs"
	"testing"
)

func TestSettingsDelimiterSavePreservesEditorFormat(t *testing.T) {
	old := vfs.CustomConfigDir
	vfs.CustomConfigDir = t.TempDir()
	defer func() { vfs.CustomConfigDir = old }()
	if err := saveConfig(config{WordDiv: " _", EditorFormat: editorFormatTargetsOnly}); err != nil {
		t.Fatal(err)
	}
	d, err := settingsProvider().Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	d.Values["visren.WordDiv"] = "-_"
	if r := d.Commit(context.Background()); len(r.Errors) > 0 {
		t.Fatal(r.Errors)
	}
	d.Values["visren.EditorFormat"] = editorFormatSourceTarget
	d.Close()
	if c := loadConfig(); c.EditorFormat != editorFormatTargetsOnly || c.WordDiv != "-_" {
		t.Fatal(c)
	}
}
