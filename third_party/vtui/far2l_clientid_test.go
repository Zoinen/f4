package vtui

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestFar2lClientID_CreatedAndPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app", "far2l-client-id")
	first := loadOrCreateFar2lClientID(path)
	if !validFar2lClientID(first) {
		t.Fatalf("generated ID %q is not valid for far2l", first)
	}
	if again := loadOrCreateFar2lClientID(path); again != first {
		t.Errorf("ID changed between runs: %q != %q", first, again)
	}
	if st, err := os.Stat(path); err != nil {
		t.Fatalf("ID file not written: %v", err)
	} else if runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		// Windows permissions are ACL-based; Go's Mode().Perm() does not report
		// the effective access policy and os.WriteFile's mode is ignored there.
		t.Errorf("ID file is accessible by others: %v", st.Mode())
	}
}

func TestFar2lClientID_DiffersBetweenInstallations(t *testing.T) {
	a := loadOrCreateFar2lClientID(filepath.Join(t.TempDir(), "id"))
	b := loadOrCreateFar2lClientID(filepath.Join(t.TempDir(), "id"))
	if a == b {
		t.Errorf("two installations got the same ID %q", a)
	}
	if a == "vtui-stateful-terminal-client-persistent-id-32chars" {
		t.Error("the old well-known constant is still handed out")
	}
}

func TestFar2lClientID_InvalidStoredReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id")
	if err := os.WriteFile(path, []byte("short\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	id := loadOrCreateFar2lClientID(path)
	if !validFar2lClientID(id) || id == "short" {
		t.Fatalf("invalid stored ID was not replaced: %q", id)
	}
	if data, _ := os.ReadFile(path); string(data) != id+"\n" {
		t.Errorf("replacement not stored: %q", data)
	}
}

func TestFar2lClientID_NoPathStillRandom(t *testing.T) {
	a, b := loadOrCreateFar2lClientID(""), loadOrCreateFar2lClientID("")
	if !validFar2lClientID(a) || a == b {
		t.Errorf("without storage the ID must still be valid and random: %q %q", a, b)
	}
}

func TestValidFar2lClientID(t *testing.T) {
	long := make([]byte, far2lClientIDMaxLen+1)
	for i := range long {
		long[i] = 'a'
	}
	cases := map[string]bool{
		"":                                 false,
		"too-short":                        false,
		"0123456789abcdef0123456789abcdef": true,
		"0123456789abcdef0123456789abcde ": false,
		"0123456789abcdef0123456789abcdeé": false,
		string(long):                       false,
		string(long[:far2lClientIDMaxLen]): true,
	}
	for id, want := range cases {
		if got := validFar2lClientID(id); got != want {
			t.Errorf("validFar2lClientID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestFar2lClientID_Cached(t *testing.T) {
	// Keep the test out of the real user config directory on every platform.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())
	resetFar2lClientID := func() {
		far2lClientIDOnce = sync.Once{}
		far2lClientIDValue = ""
	}
	resetFar2lClientID()
	t.Cleanup(resetFar2lClientID)
	if a, b := far2lClientID(), far2lClientID(); a != b || !validFar2lClientID(a) {
		t.Errorf("far2lClientID not stable/valid: %q %q", a, b)
	}
}
