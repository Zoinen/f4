package visren

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestSaveConfigKeepsPreviousFileOnSuccessiveAtomicSave(t *testing.T) {
	dir := t.TempDir()
	oldDir := vfs.CustomConfigDir
	vfs.CustomConfigDir = dir
	defer func() { vfs.CustomConfigDir = oldDir }()

	if err := saveConfig(config{WordDiv: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(config{WordDiv: "second"}); err != nil {
		t.Fatal(err)
	}
	if got := loadConfig().WordDiv; got != "second" {
		t.Fatalf("loaded WordDiv = %q, want second", got)
	}
	if leftovers, err := filepath.Glob(filepath.Join(dir, ".visren-*.tmp")); err != nil {
		t.Fatal(err)
	} else if len(leftovers) != 0 {
		t.Fatalf("temporary config files remain: %v", leftovers)
	}
	if mode, err := os.Stat(configPath()); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && mode.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", mode.Mode().Perm())
	}
}
