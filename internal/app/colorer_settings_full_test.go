//go:build !lite

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
)

func TestColorerConfigsDir_HonorsTheConfiguredCatalog(t *testing.T) {
	custom := t.TempDir()
	old := config.App.EditorColorerCatalog
	t.Cleanup(func() { config.App.EditorColorerCatalog = old })

	config.App.EditorColorerCatalog = "  " + custom + "  "
	if got := editor.ColorerConfigsDir(); got != custom {
		t.Errorf("Expected the configured folder %q, got %q", custom, got)
	}

	config.App.EditorColorerCatalog = ""
	if got := editor.ColorerConfigsDir(); got != filepath.Join(config.GetF4ConfigDir(), "colorer", "configs") {
		t.Errorf("Expected the default folder, got %q", got)
	}
}

func TestColorerSchemasExist_FollowsTheConfiguredCatalog(t *testing.T) {
	custom := t.TempDir()
	old := config.App.EditorColorerCatalog
	config.App.EditorColorerCatalog = custom
	t.Cleanup(func() { config.App.EditorColorerCatalog = old })

	if editor.SchemasExist() {
		t.Fatal("Expected no schemas in an empty folder")
	}
	if err := os.MkdirAll(filepath.Join(custom, "base"), 0700); err != nil {
		t.Fatalf("Cannot create the fixture directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(custom, "base", "catalog.xml"), []byte("<catalog/>"), 0600); err != nil {
		t.Fatalf("Cannot write the catalog: %v", err)
	}
	if !editor.SchemasExist() {
		t.Error("Expected the schemas of the configured folder to be found")
	}
}
