package dialog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFar3UserMenuPath(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "Roaming")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	menu := filepath.Join(profile, "FarMenu.ini")
	if err := os.WriteFile(menu, []byte("a: Test\n echo !.!\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Far.exe.ini"), []byte("[General]\nUseSystemProfiles=0\nUserProfileDir=%FARHOME%/Roaming\nUserLocalProfileDir=%FARHOME%/Other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{root, profile, menu, "\"" + menu + "\""} {
		got, err := Far3UserMenuPath(source)
		if err != nil || got != menu {
			t.Fatalf("%q: %q, %v", source, got, err)
		}
	}
	direct := filepath.Join(root, "FarMenu.ini")
	if err := os.WriteFile(direct, []byte("a: global"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := Far3UserMenuPath(root); err != nil || got != direct {
		t.Fatalf("global: %q %v", got, err)
	}
	for _, source := range []string{"", t.TempDir(), filepath.Join(root, "missing")} {
		if _, err := Far3UserMenuPath(source); err == nil {
			t.Fatalf("accepted %q", source)
		}
	}
}

func TestFar3UserMenuUsesRoamingSystemProfile(t *testing.T) {
	root, roaming, local := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("APPDATA", roaming)
	t.Setenv("LOCALAPPDATA", local)
	if err := os.WriteFile(filepath.Join(root, "Far.exe"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(roaming, "Far Manager")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	menu := filepath.Join(dir, "FarMenu.ini")
	if err := os.WriteFile(menu, []byte("a: test"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{root, filepath.Join(root, "Far.exe")} {
		if got, err := Far3UserMenuPath(source); err != nil || got != menu {
			t.Fatalf("%q %v", got, err)
		}
	}
}
