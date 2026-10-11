//go:build lite

package settings

import (
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestSettingsSchemeEnumerationWithoutColorer(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.EditorColorerCatalog = t.TempDir()
	config.App.EditorColorerUserHrd = t.TempDir()
	if schemes := settingsColorerSchemes(); len(schemes) != 0 {
		t.Fatalf("lite build enumerated Colorer schemes: %+v", schemes)
	}
}
