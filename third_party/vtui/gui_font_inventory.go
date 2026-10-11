//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font/opentype"
)

// The curated table in gui_font_scripts.go names the files a platform is
// likely to carry for a script. Likely is not installed: on the machine in
// f4 #926 not one of the Indic names it offers (Nirmala.ttf, kalinga.ttf and
// the rest) existed in any font directory, so discovery appended nothing and
// every Indic script drew the primary font's .notdef box -- while a terminal
// on the same machine, which asks the platform which font carries the
// character, drew them.
//
// This is the general answer to that class of miss. Linux already has one:
// fc-match reads fontconfig's database and answers whatever the distribution
// named the file. Windows and macOS have no fc-match, so the question goes to
// the system's own records instead: the font directories, plus -- on Windows
// -- the Fonts registry key, which also names fonts installed per user or
// delivered as an optional feature, and therefore living outside
// %SystemRoot%\Fonts.
//
// Coverage is read from each file's cmap through sfnt's io.ReaderAt parse, so
// a probe reads the tables it needs rather than the whole font: answering
// "does this file have U+0B13" does not pull a 20 MB CJK collection into
// memory.

// maxInstalledFontMatches bounds what one miss contributes to the chain. The
// first file that renders the rune wins, so the rest are there only for the
// case where x/image can parse a font but not rasterize this glyph.
const maxInstalledFontMatches = 4

var fontFileExtensions = map[string]struct{}{
	".ttf": {},
	".ttc": {},
	".otf": {},
	".otc": {},
}

// isFontFileName reports whether name looks like a font file sfnt can parse.
// Bitmap .fon and Type 1 .pfb files are left out: they would be opened, fail
// to parse and be marked failed, once per file, for nothing.
func isFontFileName(name string) bool {
	_, ok := fontFileExtensions[strings.ToLower(filepath.Ext(name))]
	return ok
}

// fontFilesInDir lists the font files directly in dir, in os.ReadDir's sorted
// order. A directory that does not exist is not an error here: fontSearchDirs
// offers every location the platform might use.
func fontFilesInDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !isFontFileName(entry.Name()) {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	return paths
}

// fontPathKey folds paths the way the platform's filesystem does, so the same
// file reached through %SystemRoot% and through C:\Windows is stored once.
func fontPathKey(path string) string {
	clean := filepath.Clean(path)
	switch runtime.GOOS {
	case "windows", "darwin":
		return strings.ToLower(clean)
	}
	return clean
}

var (
	installedFontsOnce  sync.Once
	installedFontsCache []string
)

// installedFontFiles returns every font file the system says is installed,
// directories first and the registry's own entries after them. It is a
// variable so tests can drive the probing without depending on the fonts the
// test host happens to carry. The list is built once: the answer changes only
// when fonts are installed, which is not something a running frame has to
// notice.
var installedFontFiles = func() []string {
	installedFontsOnce.Do(func() {
		seen := make(map[string]struct{})
		add := func(path string) bool {
			if path == "" {
				return false
			}
			key := fontPathKey(path)
			if _, ok := seen[key]; ok {
				return false
			}
			seen[key] = struct{}{}
			if info, err := os.Stat(path); err != nil || info.IsDir() {
				return false
			}
			installedFontsCache = append(installedFontsCache, path)
			return true
		}

		dirs := fontSearchDirs()
		for _, dir := range dirs {
			for _, path := range fontFilesInDir(dir) {
				add(path)
			}
		}
		fromDirs := len(installedFontsCache)
		for _, path := range registryFontFiles() {
			add(path)
		}
		DebugLog("FONT_INVENTORY: %d installed font files: %d in %d directories, %d more named by the system font registry",
			len(installedFontsCache), fromDirs, len(dirs), len(installedFontsCache)-fromDirs)
	})
	return installedFontsCache
}

var errEmptyFontCollection = errors.New("font collection holds no fonts")

// parseFontReaderAt parses the font the chain would open from this file. A
// collection yields its first font and nothing else, because that is the one
// openFace builds a face from -- reporting coverage from any other font in
// the file would hand the chain a path it then cannot draw with.
func parseFontReaderAt(src io.ReaderAt) (*opentype.Font, error) {
	if f, err := opentype.ParseReaderAt(src); err == nil {
		return f, nil
	}
	col, err := opentype.ParseCollectionReaderAt(src)
	if err != nil {
		return nil, err
	}
	if col.NumFonts() == 0 {
		return nil, errEmptyFontCollection
	}
	return col.Font(0)
}

// fontFileCoversRune reports whether the font in this file has a glyph for r.
// It is the cmap question only; whether x/image can rasterize that glyph is
// the chain's own probe, and it stays there.
func fontFileCoversRune(path string, r rune) bool {
	file, err := os.Open(path) //nolint:gosec // G304: the path comes from the platform's own font directories and registry
	if err != nil {
		return false
	}
	defer file.Close()

	f, err := parseFontReaderAt(file)
	if err != nil {
		return false
	}
	index, err := f.GlyphIndex(nil, r)
	return err == nil && index != 0
}

// installedFontPathsForRune returns installed font files whose cmap carries r,
// for the platforms where no font database answers that question. The scan
// stops once it has enough of them; a rune nothing covers walks the whole
// inventory once, and the memoised nil in the chain keeps it to once.
func installedFontPathsForRune(r rune) []string {
	paths := installedFontFiles()
	if len(paths) == 0 {
		return nil
	}
	started := time.Now()
	var found []string
	scanned := 0
	for _, path := range paths {
		scanned++
		if !fontFileCoversRune(path, r) {
			continue
		}
		found = append(found, path)
		if len(found) >= maxInstalledFontMatches {
			break
		}
	}
	DebugLog("FONT_INVENTORY: probed %d of %d installed fonts for U+%04X in %v: %d carry it",
		scanned, len(paths), r, time.Since(started), len(found))
	return found
}
