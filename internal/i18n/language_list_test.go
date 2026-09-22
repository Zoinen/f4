package i18n

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExecutableResourceDirsForMacOSBundle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS bundle layout is platform-specific")
	}

	original := os.Args[0]
	os.Args[0] = "/tmp/F4.app/Contents/MacOS/f4"
	t.Cleanup(func() { os.Args[0] = original })

	want := []string{
		"/tmp/F4.app/Contents/Resources",
		"/tmp/F4.app/Contents/MacOS",
	}
	if got := ExecutableResourceDirs(); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ExecutableResourceDirs() = %v, want %v", got, want)
	}

	if got := SearchDirs(""); got[0] != filepath.Join(want[0], "lang") {
		t.Fatalf("SearchDirs(\"\")[0] = %q, want bundle resources", got[0])
	}
}

// The language dialog must offer every translation embedded in the binary,
// not only what a lang/ directory on disk happens to hold: a bare binary
// otherwise lists English alone while a configured non-English language
// keeps working invisibly, with no way to see or change it from the UI.
func TestListAvailableUILanguages_IncludesEmbedded(t *testing.T) {
	langs := ListAvailable("")

	if len(langs) == 0 || langs[0].Code != "en" {
		t.Fatalf("English must stay the first entry, got %+v", langs)
	}

	byCode := make(map[string]string, len(langs))
	for _, l := range langs {
		if _, dup := byCode[l.Code]; dup {
			t.Errorf("duplicate language code %q", l.Code)
		}
		byCode[l.Code] = l.Name
	}

	for code, name := range map[string]string{"ru": "Русский", "ka": "ქართული"} {
		if got, ok := byCode[code]; !ok {
			t.Errorf("embedded language %q missing from the dialog list", code)
		} else if got != name {
			t.Errorf("language %q listed as %q, want %q", code, got, name)
		}
	}
}
