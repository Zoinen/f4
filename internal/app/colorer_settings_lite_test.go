//go:build lite

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
)

func TestColorerLiteConfiguredSchemasRemainUnavailable(t *testing.T) {
	oldCatalog := config.App.EditorColorerCatalog
	t.Cleanup(func() { config.App.EditorColorerCatalog = oldCatalog })
	catalogDir := t.TempDir()
	config.App.EditorColorerCatalog = catalogDir
	if err := os.MkdirAll(filepath.Join(catalogDir, "base"), 0o700); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(catalogDir, "base", "catalog.xml")
	const contents = "<catalog/>"
	if err := os.WriteFile(catalog, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if editor.SchemasExist() {
		t.Fatal("lite must not advertise Colorer even when full-build schemas exist")
	}
	if got := editor.ColorerConfigsDir(); got != "" {
		t.Fatalf("lite ColorerConfigsDir = %q, want unavailable", got)
	}
	if got := editor.DefaultColorerConfigsDir(); got != "" {
		t.Fatalf("lite default ColorerConfigsDir = %q, want unavailable", got)
	}
	if got := editor.CurrentColorerSource(); got != (editor.ColorerSource{}) {
		t.Fatalf("lite Colorer source = %#v, want empty", got)
	}
	got, err := os.ReadFile(catalog)
	if err != nil || string(got) != contents {
		t.Fatalf("existing full-build catalog changed: %q, %v", got, err)
	}
	t.Log("[FIX:lite-app] existing Colorer configuration is retained but inactive")
}
