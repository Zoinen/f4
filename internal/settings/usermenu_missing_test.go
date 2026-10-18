package settings

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
)

// A local user menu file that is not in the panel's folder is an empty menu,
// not an error: read through the panel's file system, the missing
// FarMenu.ini reached the Settings window as "open C:\FarMenu.ini: the system
// cannot find the file specified" (f4#1976, mitek1977-coder).
func TestUserMenuStore_MissingLocalFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []settingsMenuSource{
		{id: "local", label: "Local", path: filepath.Join(dir, panel.FarMenuFileName), mode: panel.MenuModeLocal, filesystem: vfs.NewOSVFS(dir)},
		{id: "local", label: "Local", path: filepath.Join(dir, panel.FarMenuFileName), mode: panel.MenuModeLocal},
	} {
		rows, err := newUserMenuSettingsStore(src).load()
		if err != nil {
			t.Fatalf("missing %s (file system %v): %v", src.path, src.filesystem != nil, err)
		}
		if len(rows) != 0 {
			t.Fatalf("missing menu file gave %d rows", len(rows))
		}
	}
}
