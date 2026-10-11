package netfox

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/sdk/f4settings"
)

func TestSettingsRenamePreservesOptionsAndCredentials(t *testing.T) {
	store := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	ctx := context.Background()
	if err := store.SaveConfig("original", NetFoxConfig{Type: "sftp", Host: "example.test", Pass: "secret", Options: map[string]string{"future": "keep"}}); err != nil {
		t.Fatal(err)
	}
	p := &settingsProvider{store: store}
	d, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, name := range []string{"renamed", "again"} {
		d.Records["netfox.connections"][0].Values["netfox.Name"] = name
		if r := d.Commit(ctx); len(r.Errors) > 0 {
			t.Fatal(r.Errors)
		}
		cfg := store.getConfigs()[name]
		if cfg.Options["future"] != "keep" || cfg.Pass != "secret" {
			t.Fatal("rename lost hidden options or credentials")
		}
	}
	if err := store.SaveConfig("other", NetFoxConfig{Type: "ftp", Host: "elsewhere"}); err != nil {
		t.Fatal(err)
	}
	d.Records["netfox.connections"][0].Values["netfox.Host"] = "changed"
	if r := d.Commit(ctx); len(r.Errors) == 0 || !d.Dirty("netfox.connections") {
		t.Fatal("concurrent update was overwritten")
	}
}

func TestSettingsPreservesBothSSHImportAndSecondHopPreferences(t *testing.T) {
	store := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	provider := &settingsProvider{store: store}
	fields := map[string]bool{}
	for _, field := range provider.Catalog().Fields {
		fields[field.ID] = true
	}
	const secondHop = "netfox.AllowServerToServerPasswordAuth"
	if !fields[importSSHProfilesSettingID] || !fields[secondHop] {
		t.Fatalf("missing merged settings: %v", fields)
	}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	for _, values := range [][2]string{{"false", "true"}, {"true", "false"}} {
		draft.Values[importSSHProfilesSettingID] = values[0]
		draft.Values[secondHop] = values[1]
		if result := draft.Commit(context.Background()); len(result.Errors) != 0 {
			t.Fatal(result.Errors)
		}
		fresh, err := provider.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		func(d *f4settings.Draft) {
			defer d.Close()
			if d.Values[importSSHProfilesSettingID] != values[0] || d.Values[secondHop] != values[1] {
				t.Fatalf("reopened settings = %v, want %v", d.Values, values)
			}
		}(fresh)
	}
}
