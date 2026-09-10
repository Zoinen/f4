package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/theme"
	"os"
	"path/filepath"
	"testing"
)

// iniResolvers lists per-INI path functions and the fragment they append to
// the f4 config dir in both portable and non-portable modes. Portable resolves
// under <exeDir>/Profile, non-portable under <config.UserConfigDir>/f4, so the same
// fragment works for both bases.
var iniResolvers = []struct {
	name string
	fn   func() string
	rel  string
}{
	{"BookmarksFilePath", panel.BookmarksFilePath, "settings/bookmarks.ini"},
	{"AssociationsFilePath", panel.AssociationsFilePath, "settings/associations.ini"},
	{"MainMenuFilePath", panel.MainMenuFilePath, "settings/user_menu.ini"},
	{"GetSessionIniPath", GetSessionIniPath, "session.ini"},
	{"vtvibeIniPath", vtvibeIniPath, "vtvibe.ini"},
	{"theme.UserColorOverridesPath", theme.UserColorOverridesPath, "farcolors.ini"},
}

// setupPortableIni points config.Executable at a mock f4 binary accompanied by an
// f4.ini that selects the given UseSystemProfiles value, then drops the cached
// config directory so config.GetF4ConfigDir/config.IsPortableProfile re-detect from scratch.
// It returns the directory holding the mock executable.
func setupPortableIni(t *testing.T, useSystemProfiles string) string {
	t.Helper()
	tmpDir := t.TempDir()
	mockExe := filepath.Join(tmpDir, "f4.exe")
	if err := os.WriteFile(mockExe, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	ini := "[General]\nUseSystemProfiles = " + useSystemProfiles + "\n"
	if err := os.WriteFile(mockExe+".ini", []byte(ini), 0600); err != nil {
		t.Fatal(err)
	}

	origExe := config.Executable
	config.Executable = func() (string, error) { return mockExe, nil }
	t.Cleanup(func() {
		config.Executable = origExe
		config.ResetConfigDirForTest()
		config.CachedF4Portable = false
		config.CachedF4ConfigDir = ""
	})
	config.ResetConfigDirForTest()
	return tmpDir
}

func TestIniPaths_PortableProfile(t *testing.T) {
	tmpDir := setupPortableIni(t, "0")
	for _, tc := range iniResolvers {
		want := filepath.Join(tmpDir, "Profile", tc.rel)
		if got := tc.fn(); got != want {
			t.Errorf("%s() portable = %q, want %q", tc.name, got, want)
		}
	}
}

func TestIniPaths_NonPortableProfile(t *testing.T) {
	// Redirect the user config dir seam so the system path resolves inside a
	// temp dir instead of the developer's real profile.
	cfgDir := t.TempDir()
	orig := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfgDir, nil }
	t.Cleanup(func() { config.UserConfigDir = orig })

	setupPortableIni(t, "1")
	for _, tc := range iniResolvers {
		want := filepath.Join(cfgDir, "f4", tc.rel)
		if got := tc.fn(); got != want {
			t.Errorf("%s() non-portable = %q, want %q", tc.name, got, want)
		}
	}
}

func TestConfigIniPaths_PortableProfile(t *testing.T) {
	tmpDir := setupPortableIni(t, "0")
	paths := config.GetConfigIniPaths()
	want := []string{filepath.Join(tmpDir, "Profile", "settings.ini")}
	if len(paths) != len(want) || paths[0] != want[0] {
		t.Errorf("config.GetConfigIniPaths() portable = %v, want exactly %v (machine-wide paths must not leak)", paths, want)
	}
}

func TestConfigIniPaths_NonPortableProfile(t *testing.T) {
	cfgDir := t.TempDir()
	orig := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfgDir, nil }
	t.Cleanup(func() { config.UserConfigDir = orig })

	setupPortableIni(t, "1")
	paths := config.GetConfigIniPaths()
	userPath := filepath.Join(cfgDir, "f4", "settings.ini")
	found := false
	for _, p := range paths {
		if p == userPath {
			found = true
		}
	}
	if !found {
		t.Errorf("config.GetConfigIniPaths() non-portable = %v, want it to contain user path %q", paths, userPath)
	}
	if len(paths) < 2 {
		t.Errorf("config.GetConfigIniPaths() non-portable = %v, want at least system + user paths", paths)
	}
}
