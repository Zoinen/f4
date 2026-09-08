//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
	"unicode"
)

// The static list in gui_font.go answers for CJK, emoji and symbols. It
// cannot grow to cover the rest of Unicode: every entry on it is parsed by
// the warm() sweep and condensed into a 136 KB coverage bitmap, so listing a
// font per script would turn startup into a parse of every Indic, Ethiopic
// and South-East Asian font on the machine — in sessions that draw none of
// them.
//
// Discovery is the other half of the answer. A rune that no listed font
// renders asks, once, which files on this machine carry its script, and only
// those are opened. A file name in Devanagari costs one Nirmala UI (or Noto
// Devanagari) parse; a session that never meets one costs nothing. Without
// it, everything outside CJK and Latin — Hindi, Tamil, Thai, Amharic, Khmer,
// Cherokee — reached the end of the chain and drew the primary font's
// .notdef box, while the same text under a terminal emulator, which asks the
// platform for a font per run, rendered normally.

// scriptFallback names the fonts a system is likely to carry for one script.
// The script is a key into unicode.Scripts rather than a hand-written code
// point range: Devanagari alone spans two blocks, Ethiopic four, and the
// ranges move with each Unicode revision.
//
// noto holds file stems (expanded across the font directories and the Noto
// naming variants); win and mac hold plain file names, looked up in the
// platform's font directories. Nothing here is required to exist — a name
// that is absent costs one failed stat, once per script.
type scriptFallback struct {
	script string
	noto   []string
	win    []string
	mac    []string
}

// scriptFallbacks is ordered only for reading; a rune belongs to exactly one
// script, so the first entry that claims it is the entry.
var scriptFallbacks = []scriptFallback{
	// South Asia. Nirmala UI is Windows' pan-Indic font and covers most of
	// this group on its own; the legacy per-script fonts follow it because
	// they exist on installs where Nirmala does not.
	{script: "Devanagari", noto: []string{"NotoSansDevanagari", "NotoSerifDevanagari"},
		win: []string{"Nirmala.ttf", "mangal.ttf", "aparaj.ttf", "kokila.ttf", "utsaah.ttf"},
		mac: []string{"Kohinoor.ttc", "KohinoorDevanagari.ttc", "Devanagari Sangam MN.ttc", "ITFDevanagari.ttc"}},
	{script: "Bengali", noto: []string{"NotoSansBengali", "NotoSerifBengali"},
		win: []string{"Nirmala.ttf", "vrinda.ttf", "shonarb.ttf"},
		mac: []string{"KohinoorBangla.ttc", "Bangla Sangam MN.ttc", "Bangla MN.ttc"}},
	{script: "Gurmukhi", noto: []string{"NotoSansGurmukhi", "NotoSerifGurmukhi"},
		win: []string{"Nirmala.ttf", "raavi.ttf"},
		mac: []string{"Gurmukhi Sangam MN.ttc", "Gurmukhi MN.ttc", "Gurmukhi.ttf"}},
	{script: "Gujarati", noto: []string{"NotoSansGujarati", "NotoSerifGujarati"},
		win: []string{"Nirmala.ttf", "shruti.ttf"},
		mac: []string{"Gujarati Sangam MN.ttc", "Gujarati MT.ttc"}},
	{script: "Oriya", noto: []string{"NotoSansOriya", "NotoSerifOriya"},
		win: []string{"Nirmala.ttf", "kalinga.ttf"},
		mac: []string{"Oriya Sangam MN.ttc", "Oriya MN.ttc"}},
	{script: "Tamil", noto: []string{"NotoSansTamil", "NotoSerifTamil"},
		win: []string{"Nirmala.ttf", "latha.ttf", "vijaya.ttf"},
		mac: []string{"Tamil Sangam MN.ttc", "Tamil MN.ttc", "KohinoorTelugu.ttc"}},
	{script: "Telugu", noto: []string{"NotoSansTelugu", "NotoSerifTelugu"},
		win: []string{"Nirmala.ttf", "gautami.ttf", "vani.ttf"},
		mac: []string{"Telugu Sangam MN.ttc", "Telugu MN.ttc", "KohinoorTelugu.ttc"}},
	{script: "Kannada", noto: []string{"NotoSansKannada", "NotoSerifKannada"},
		win: []string{"Nirmala.ttf", "tunga.ttf"},
		mac: []string{"Kannada Sangam MN.ttc", "Kannada MN.ttc"}},
	{script: "Malayalam", noto: []string{"NotoSansMalayalam", "NotoSerifMalayalam"},
		win: []string{"Nirmala.ttf", "kartika.ttf"},
		mac: []string{"Malayalam Sangam MN.ttc", "Malayalam MN.ttc"}},
	{script: "Sinhala", noto: []string{"NotoSansSinhala", "NotoSerifSinhala"},
		win: []string{"Nirmala.ttf", "iskpota.ttf"},
		mac: []string{"Sinhala Sangam MN.ttc", "Sinhala MN.ttc"}},
	{script: "Ol_Chiki", noto: []string{"NotoSansOlChiki"}, win: []string{"Nirmala.ttf"}},
	{script: "Meetei_Mayek", noto: []string{"NotoSansMeeteiMayek"}, win: []string{"Nirmala.ttf"}},
	{script: "Syloti_Nagri", noto: []string{"NotoSansSylotiNagri"}, win: []string{"Nirmala.ttf"}},
	{script: "Chakma", noto: []string{"NotoSansChakma"}, win: []string{"Nirmala.ttf"}},
	{script: "Newa", noto: []string{"NotoSansNewa"}},
	{script: "Limbu", noto: []string{"NotoSansLimbu"}},
	{script: "Lepcha", noto: []string{"NotoSansLepcha"}},
	{script: "Thaana", noto: []string{"NotoSansThaana"}, win: []string{"mvboli.ttf"}},

	// South-East Asia and Tibet.
	{script: "Thai", noto: []string{"NotoSansThai", "NotoSerifThai"},
		win: []string{"tahoma.ttf", "LeelawUI.ttf", "leelawui.ttf", "LeelaUIb.ttf", "leelawad.ttf"},
		mac: []string{"Ayuthaya.ttf", "Thonburi.ttc", "Sathu.ttf"}},
	{script: "Lao", noto: []string{"NotoSansLao", "NotoSerifLao"},
		win: []string{"LeelawUI.ttf", "leelawui.ttf", "LeelaUIb.ttf", "dokchamp.ttf", "LaoUI.ttf"},
		mac: []string{"Lao Sangam MN.ttf", "Lao MN.ttc"}},
	{script: "Khmer", noto: []string{"NotoSansKhmer", "NotoSerifKhmer"},
		win: []string{"khmerui.ttf", "LeelawUI.ttf"},
		mac: []string{"Khmer Sangam MN.ttf", "Khmer MN.ttc"}},
	{script: "Myanmar", noto: []string{"NotoSansMyanmar", "NotoSerifMyanmar"},
		win: []string{"mmrtext.ttf"},
		mac: []string{"Myanmar Sangam MN.ttc", "Myanmar MN.ttc"}},
	{script: "Tibetan", noto: []string{"NotoSerifTibetan", "NotoSansTibetan"},
		win: []string{"himalaya.ttf", "monbaiti.ttf"},
		mac: []string{"Kailasa.ttc", "Kailasa.ttf"}},
	{script: "Javanese", noto: []string{"NotoSansJavanese"}, win: []string{"javatext.ttf"}},
	{script: "Balinese", noto: []string{"NotoSansBalinese"}},
	{script: "Sundanese", noto: []string{"NotoSansSundanese"}},
	{script: "Buginese", noto: []string{"NotoSansBuginese"}},
	{script: "Batak", noto: []string{"NotoSansBatak"}},
	{script: "Cham", noto: []string{"NotoSansCham"}},
	{script: "Tai_Le", noto: []string{"NotoSansTaiLe"}, win: []string{"taile.ttf"}},
	{script: "New_Tai_Lue", noto: []string{"NotoSansNewTaiLue"}, win: []string{"ntailu.ttf"}},
	{script: "Tai_Tham", noto: []string{"NotoSansTaiTham"}},
	{script: "Tai_Viet", noto: []string{"NotoSansTaiViet"}},
	{script: "Mongolian", noto: []string{"NotoSansMongolian"}, win: []string{"monbaiti.ttf"}},
	{script: "Phags_Pa", noto: []string{"NotoSansPhagsPa"}, win: []string{"phagspa.ttf"}},
	{script: "Yi", noto: []string{"NotoSansYi"}, win: []string{"msyi.ttf"}},

	// Africa and the Middle East. Ebrima is Windows' catch-all here.
	{script: "Ethiopic", noto: []string{"NotoSansEthiopic", "NotoSerifEthiopic"},
		win: []string{"ebrima.ttf", "nyala.ttf"},
		mac: []string{"Kefa.ttc"}},
	{script: "Nko", noto: []string{"NotoSansNKo"}, win: []string{"ebrima.ttf"}},
	{script: "Tifinagh", noto: []string{"NotoSansTifinagh"}, win: []string{"ebrima.ttf"}},
	{script: "Vai", noto: []string{"NotoSansVai"}, win: []string{"ebrima.ttf"}},
	{script: "Adlam", noto: []string{"NotoSansAdlam"}, win: []string{"ebrima.ttf"}},
	{script: "Osmanya", noto: []string{"NotoSansOsmanya"}, win: []string{"ebrima.ttf"}},
	{script: "Syriac", noto: []string{"NotoSansSyriac"}, win: []string{"estre.ttf", "seguisym.ttf"}},
	{script: "Hebrew", noto: []string{"NotoSansHebrew", "NotoSerifHebrew"},
		win: []string{"segoeui.ttf", "arial.ttf", "times.ttf"},
		mac: []string{"ArialHB.ttc", "Corsiva.ttc"}},
	{script: "Arabic", noto: []string{"NotoSansArabic", "NotoNaskhArabic"},
		win: []string{"segoeui.ttf", "arial.ttf", "tahoma.ttf"},
		mac: []string{"GeezaPro.ttc", "Baghdad.ttc"}},
	{script: "Armenian", noto: []string{"NotoSansArmenian", "NotoSerifArmenian"},
		win: []string{"sylfaen.ttf", "segoeui.ttf"},
		mac: []string{"Mshtakan.ttc"}},
	{script: "Georgian", noto: []string{"NotoSansGeorgian", "NotoSerifGeorgian"},
		win: []string{"sylfaen.ttf", "segoeui.ttf"}},

	// The Americas, and the scripts Windows keeps in Segoe UI Historic.
	{script: "Cherokee", noto: []string{"NotoSansCherokee"},
		win: []string{"gadugi.ttf"},
		mac: []string{"Plantagenet Cherokee.ttf", "Galvji.ttc"}},
	{script: "Canadian_Aboriginal", noto: []string{"NotoSansCanadianAboriginal"},
		win: []string{"gadugi.ttf"},
		mac: []string{"EuphemiaCAS.ttc", "Euphemia UCAS.ttc"}},
	{script: "Gothic", noto: []string{"NotoSansGothic"}, win: []string{"seguihis.ttf"}},
	{script: "Old_Italic", noto: []string{"NotoSansOldItalic"}, win: []string{"seguihis.ttf"}},
	{script: "Runic", noto: []string{"NotoSansRunic"}, win: []string{"seguihis.ttf"}},
	{script: "Ogham", noto: []string{"NotoSansOgham"}, win: []string{"seguihis.ttf"}},
	{script: "Deseret", noto: []string{"NotoSansDeseret"}, win: []string{"seguihis.ttf"}},
	{script: "Coptic", noto: []string{"NotoSansCoptic"}, win: []string{"seguihis.ttf"}},
	{script: "Glagolitic", noto: []string{"NotoSansGlagolitic"}, win: []string{"seguihis.ttf"}},
}

// notoFileSuffixes covers the two ways distributions ship a Noto family: one
// static file per weight, and the variable font whose axes are part of the
// file name.
var notoFileSuffixes = []string{
	"-Regular.ttf",
	"-Regular.otf",
	"[wght].ttf",
	"[wdth,wght].ttf",
}

var unixFontDirs = []string{
	"/usr/share/fonts/truetype/noto",
	"/usr/share/fonts/opentype/noto",
	"/usr/share/fonts/noto",
	"/usr/share/fonts/google-noto",
	"/usr/share/fonts/google-noto-sans-fonts",
	"/usr/share/fonts/truetype",
	"/usr/share/fonts/TTF",
	"/usr/share/fonts",
	"/usr/local/share/fonts",
}

var macFontDirs = []string{
	"/System/Library/Fonts",
	"/System/Library/Fonts/Supplemental",
	"/Library/Fonts",
}

// windowsFontDirs prefers %SystemRoot% over the hardcoded path — Windows is
// not always on C: — and includes the per-user directory, where a font
// installed without administrator rights lands.
func windowsFontDirs() []string {
	dirs := make([]string, 0, 3)
	if root := os.Getenv("SystemRoot"); root != "" {
		dirs = append(dirs, filepath.Join(root, "Fonts"))
	}
	dirs = append(dirs, `C:\Windows\Fonts`)
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
	}
	return dirs
}

// fontSearchDirs is where a named font file may be found on this platform,
// system directories first and the user's own last.
func fontSearchDirs() []string {
	var dirs []string
	switch runtime.GOOS {
	case "windows":
		dirs = append(dirs, windowsFontDirs()...)
	case "darwin":
		dirs = append(dirs, macFontDirs...)
		if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
		}
	default:
		dirs = append(dirs, unixFontDirs...)
		if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs,
				filepath.Join(home, ".local", "share", "fonts"),
				filepath.Join(home, ".fonts"),
			)
		}
	}
	return dirs
}

// candidates expands the entry into absolute paths for this platform. The
// platform's own fonts come first: they are the ones the system ships for
// the script, and on Windows they are what the terminal would have used.
func (s scriptFallback) candidates() []string {
	var names []string
	switch runtime.GOOS {
	case "windows":
		names = append(names, s.win...)
	case "darwin":
		names = append(names, s.mac...)
	}
	for _, stem := range s.noto {
		for _, suffix := range notoFileSuffixes {
			names = append(names, stem+suffix)
		}
	}

	dirs := fontSearchDirs()
	paths := make([]string, 0, len(names)*len(dirs))
	for _, name := range names {
		for _, dir := range dirs {
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	return paths
}

// scriptFallbackCandidates returns the paths worth trying for r, or nil when
// no entry claims its script. It touches no files, so it stays testable on a
// machine that has none of these fonts.
func scriptFallbackCandidates(r rune) []string {
	for _, entry := range scriptFallbacks {
		table, ok := unicode.Scripts[entry.script]
		if !ok || !unicode.Is(table, r) {
			continue
		}
		return entry.candidates()
	}
	return nil
}

const maxFontconfigRunePaths = 3

// fontconfigRuneTimeout bounds the one place a font lookup runs a subprocess
// while the renderer waits on it. fc-match answers in milliseconds; the
// timeout is for a broken or NFS-backed font configuration, where the
// alternative is a frozen window.
const fontconfigRuneTimeout = 3 * time.Second

// runFontconfigForRune asks fontconfig which installed fonts cover r. It is
// the general case the curated table cannot be: it finds the font whatever
// the distribution called it and wherever it was packaged, including fonts
// the user installed themselves.
var runFontconfigForRune = func(r rune) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fontconfigRuneTimeout)
	defer cancel()
	pattern := fmt.Sprintf(":charset=%04x", r)
	//nolint:gosec // G204: fixed binary, and the only variable argument is a hex code point this function formats itself
	out, err := exec.CommandContext(ctx, "fc-match", "-s", "-f", "%{file}\n", pattern).Output()
	if err != nil {
		return nil, err
	}
	paths := parseFontconfigPaths(string(out))
	if len(paths) > maxFontconfigRunePaths {
		paths = paths[:maxFontconfigRunePaths]
	}
	return paths, nil
}

// fontconfigAvailable reports whether this platform is one where fontconfig
// is the font database. Windows and macOS have their own, and no fc-match.
func fontconfigAvailable() bool {
	switch runtime.GOOS {
	case "windows", "darwin":
		return false
	}
	return true
}

func fontconfigPathsForRune(r rune) []string {
	if !fontconfigAvailable() {
		return nil
	}
	paths, err := runFontconfigForRune(r)
	if err != nil {
		return nil
	}
	return paths
}

// discoverFallbackPaths returns font files that exist on this machine and
// carry r's script. It is a variable so tests can drive the chain without
// depending on which fonts the test host happens to have installed.
//
// Existence is checked here rather than in the chain: the curated table is
// deliberately generous with names, most of which are absent on any given
// system, and the chain must not record a failed entry for each of them.
var discoverFallbackPaths = func(r rune) []string {
	if os.Getenv("VTUI_NO_FONT_DISCOVERY") != "" {
		return nil
	}

	var found []string
	seen := make(map[string]struct{})
	add := func(path string) {
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return
		}
		found = append(found, path)
	}

	for _, path := range scriptFallbackCandidates(r) {
		add(path)
	}
	for _, path := range fontconfigPathsForRune(r) {
		add(path)
	}
	return found
}
