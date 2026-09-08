package vtui

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

func TestScriptFallbacks_ScriptNamesResolve(t *testing.T) {
	seen := make(map[string]bool, len(scriptFallbacks))
	for _, entry := range scriptFallbacks {
		if seen[entry.script] {
			t.Errorf("duplicate script entry: %s", entry.script)
		}
		seen[entry.script] = true

		if _, ok := unicode.Scripts[entry.script]; !ok {
			t.Errorf("unknown unicode script name: %s", entry.script)
		}
		if len(entry.noto) == 0 && len(entry.win) == 0 && len(entry.mac) == 0 {
			t.Errorf("script %s lists no fonts at all", entry.script)
		}
	}
}

func TestScriptFallbackCandidates_CoversReportedScripts(t *testing.T) {
	// One rune per script from the issue that started this: a file listing
	// of Wikipedia names, where every one of these drew a .notdef box.
	cases := []struct {
		name string
		r    rune
		want string // a file name that must appear among the candidates
	}{
		{"Hindi", 'ह', "NotoSansDevanagari"},
		{"Bengali", 'ব', "NotoSansBengali"},
		{"Tamil", 'த', "NotoSansTamil"},
		{"Telugu", 'త', "NotoSansTelugu"},
		{"Kannada", 'ಕ', "NotoSansKannada"},
		{"Malayalam", 'മ', "NotoSansMalayalam"},
		{"Gujarati", 'ગ', "NotoSansGujarati"},
		{"Odia", 'ଓ', "NotoSansOriya"},
		{"Punjabi", 'ਪ', "NotoSansGurmukhi"},
		{"Sinhala", 'ස', "NotoSansSinhala"},
		{"Thai", 'ก', "NotoSansThai"},
		{"Lao", 'ວ', "NotoSansLao"},
		{"Khmer", 'វ', "NotoSansKhmer"},
		{"Burmese", 'မ', "NotoSansMyanmar"},
		{"Tibetan", 'བ', "Tibetan"},
		{"Amharic", 'አ', "NotoSansEthiopic"},
		{"Cherokee", 'Ꮳ', "NotoSansCherokee"},
		{"Inuktitut", 'ᐃ', "NotoSansCanadianAboriginal"},
		{"Gothic", '𐌲', "NotoSansGothic"},
		{"Javanese", 'ꦮ', "NotoSansJavanese"},
		{"Thaana", 'ދ', "NotoSansThaana"},
		{"Syriac", 'ܘ', "NotoSansSyriac"},
		{"Nko", 'ߥ', "NotoSansNKo"},
		{"Tifinagh", 'ⵡ', "NotoSansTifinagh"},
		{"Santali", 'ᱥ', "NotoSansOlChiki"},
		{"Meitei", 'ꯃ', "NotoSansMeeteiMayek"},
		{"Balinese", 'ᬯ', "NotoSansBalinese"},
		{"Buginese", 'ᨓ', "NotoSansBuginese"},
		{"Newar", '𑐣', "NotoSansNewa"},
		{"Sylheti", 'ꠍ', "NotoSansSylotiNagri"},
		{"TaiLe", 'ᥝ', "NotoSansTaiLe"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paths := scriptFallbackCandidates(tc.r)
			if len(paths) == 0 {
				t.Fatalf("no fallback candidates for U+%04X (%s)", tc.r, tc.name)
			}
			for _, path := range paths {
				if !filepath.IsAbs(path) {
					t.Errorf("candidate path must be absolute: %s", path)
				}
			}
			found := false
			for _, path := range paths {
				if strings.Contains(path, tc.want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("candidates for U+%04X do not mention %q: %v", tc.r, tc.want, paths)
			}
		})
	}
}

func TestScriptFallbackCandidates_PlatformFontsFirst(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the Windows font names are only offered on Windows")
	}
	paths := scriptFallbackCandidates('ह')
	if len(paths) == 0 {
		t.Fatal("no candidates for Devanagari")
	}
	if !strings.Contains(strings.ToLower(paths[0]), "nirmala") {
		t.Errorf("first Devanagari candidate = %q, want Nirmala UI", paths[0])
	}
}

func TestScriptFallbackCandidates_UnknownScript(t *testing.T) {
	if got := scriptFallbackCandidates('A'); got != nil {
		t.Errorf("scriptFallbackCandidates('A') = %v, want nil", got)
	}
}

// fakeFace stands in for a parsed font: it renders exactly the runes it was
// built with, which is all the chain asks of a face.
type fakeFace struct {
	path  string
	runes map[rune]bool
}

func newFakeChain(t *testing.T, fonts map[string]string) (*fontFallbackChain, *int) {
	t.Helper()
	opens := 0
	chain := &fontFallbackChain{
		logTag: "TEST_FONT",
		open: func(path string) (any, error) {
			opens++
			glyphs, ok := fonts[path]
			if !ok {
				return nil, errors.New("no such font")
			}
			face := &fakeFace{path: path, runes: map[rune]bool{}}
			for _, r := range glyphs {
				face.runes[r] = true
			}
			return face, nil
		},
		covers:  func(face any, r rune) bool { return face.(*fakeFace).runes[r] },
		renders: func(face any, r rune) bool { return face.(*fakeFace).runes[r] },
		drop:    func(any) {},
	}
	return chain, &opens
}

func TestChain_DiscoversScriptFontWhenListedFontsMiss(t *testing.T) {
	const devanagari = "/fonts/Devanagari.ttf"
	chain, _ := newFakeChain(t, map[string]string{
		"/fonts/Latin.ttf": "AB",
		devanagari:         "हिन्दी",
	})
	chain.entries = []fontFallbackEntry{{path: "/fonts/Latin.ttf"}}

	previous := discoverFallbackPaths
	discoverFallbackPaths = func(r rune) []string {
		if unicode.Is(unicode.Devanagari, r) {
			return []string{devanagari}
		}
		return nil
	}
	t.Cleanup(func() { discoverFallbackPaths = previous })

	face, ok := chain.faceFor('ह').(*fakeFace)
	if !ok || face.path != devanagari {
		t.Fatalf("faceFor('ह') = %#v, want the discovered Devanagari face", chain.faceFor('ह'))
	}
	if got := chain.faceFor('A').(*fakeFace).path; got != "/fonts/Latin.ttf" {
		t.Errorf("faceFor('A') = %q, want the listed font", got)
	}
	if chain.faceFor('葉') != nil {
		t.Error("faceFor for an uncovered rune must stay nil")
	}
}

func TestChain_DiscoveryIsMemoisedPerRune(t *testing.T) {
	const devanagari = "/fonts/Devanagari.ttf"
	chain, opens := newFakeChain(t, map[string]string{devanagari: "ह"})

	calls := 0
	previous := discoverFallbackPaths
	discoverFallbackPaths = func(rune) []string {
		calls++
		return []string{devanagari}
	}
	t.Cleanup(func() { discoverFallbackPaths = previous })

	for i := 0; i < 5; i++ {
		if chain.faceFor('ह') == nil {
			t.Fatalf("call %d: discovered face lost", i)
		}
	}
	if calls != 1 {
		t.Errorf("discovery ran %d times for one rune, want 1", calls)
	}
	if *opens != 1 {
		t.Errorf("font opened %d times, want 1", *opens)
	}
	if len(chain.entries) != 1 {
		t.Errorf("entries = %d, want the one discovered font", len(chain.entries))
	}

	// A second rune from the same font must reuse the entry rather than
	// append a duplicate.
	chain.faceFor('न')
	if len(chain.entries) != 1 {
		t.Errorf("entries after a second rune = %d, want 1", len(chain.entries))
	}
}

func TestChain_DiscoveryDoesNotDuplicateListedPaths(t *testing.T) {
	const path = "/fonts/Only.ttf"
	chain, _ := newFakeChain(t, map[string]string{path: "A"})
	chain.entries = []fontFallbackEntry{{path: path}}

	previous := discoverFallbackPaths
	discoverFallbackPaths = func(rune) []string { return []string{path} }
	t.Cleanup(func() { discoverFallbackPaths = previous })

	if chain.faceFor('ह') != nil {
		t.Error("faceFor must be nil when the only font lacks the rune")
	}
	if len(chain.entries) != 1 {
		t.Errorf("entries = %d, want no duplicate of the listed path", len(chain.entries))
	}
}

func TestDiscoverFallbackPaths_SkipsMissingFilesAndRespectsOptOut(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "Present.ttf")
	if err := os.WriteFile(real, []byte("font"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	previousScripts := scriptFallbacks
	scriptFallbacks = []scriptFallback{{script: "Devanagari", win: nil, mac: nil, noto: nil}}
	t.Cleanup(func() { scriptFallbacks = previousScripts })

	previousFC := runFontconfigForRune
	runFontconfigForRune = func(rune) ([]string, error) {
		return []string{real, filepath.Join(dir, "Absent.ttf")}, nil
	}
	t.Cleanup(func() { runFontconfigForRune = previousFC })

	if !fontconfigAvailable() {
		t.Skip("fontconfig discovery is not used on this platform")
	}

	got := discoverFallbackPaths('ह')
	if len(got) != 1 || got[0] != real {
		t.Fatalf("discoverFallbackPaths = %v, want [%s]", got, real)
	}

	t.Setenv("VTUI_NO_FONT_DISCOVERY", "1")
	if got := discoverFallbackPaths('ह'); got != nil {
		t.Errorf("discovery must be disabled by VTUI_NO_FONT_DISCOVERY, got %v", got)
	}
}
