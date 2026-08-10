//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

func getFontCandidates(fontName string) []string {
	var candidates []string
	if fontName != "" {
		// GUI callers normally specify a font family, while the raster/GPU
		// renderers need an actual file. Keep the well-known platform families
		// ahead of generic filename probing so they do not silently fall back.
		switch {
		case strings.EqualFold(fontName, "Consolas"):
			candidates = append(candidates, `C:\Windows\Fonts\consola.ttf`)
		case strings.EqualFold(fontName, "Menlo"):
			candidates = append(candidates, "/System/Library/Fonts/Menlo.ttc")
		case strings.EqualFold(fontName, "Monaco"):
			candidates = append(candidates, "/System/Library/Fonts/Monaco.ttf")
		}
		candidates = append(candidates, fontName)
		if filepath.Ext(fontName) == "" {
			candidates = append(candidates, fontName+".ttf", fontName+".ttc", fontName+".otf")
		}
		dirs := []string{
			`C:\Windows\Fonts`,
			"/usr/share/fonts/truetype",
			"/usr/share/fonts/TTF",
			"/usr/local/share/fonts",
			"/System/Library/Fonts/Supplemental",
			"/System/Library/Fonts",
		}
		for _, dir := range dirs {
			candidates = append(candidates, filepath.Join(dir, fontName))
			if filepath.Ext(fontName) == "" {
				candidates = append(candidates,
					filepath.Join(dir, fontName+".ttf"),
					filepath.Join(dir, fontName+".ttc"),
					filepath.Join(dir, fontName+".otf"))
			}
			entries, err := os.ReadDir(dir)
			if err == nil {
				for _, e := range entries {
					if e.IsDir() {
						candidates = append(candidates, filepath.Join(dir, e.Name(), fontName))
						if filepath.Ext(fontName) == "" {
							candidates = append(candidates,
								filepath.Join(dir, e.Name(), fontName+".ttf"),
								filepath.Join(dir, e.Name(), fontName+".ttc"),
								filepath.Join(dir, e.Name(), fontName+".otf"))
						}
					}
				}
			}
		}
	}

	defaultPaths := []string{
		`C:\Windows\Fonts\consola.ttf`,
		`C:\Windows\Fonts\lucon.ttf`,
		`C:\Windows\Fonts\cour.ttf`,
		`C:\Windows\Fonts\arial.ttf`,
		"/usr/share/fonts/truetype/ubuntu/UbuntuMono-R.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/TTF/DejaVuSansMono.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
		"/System/Library/Fonts/Menlo.ttc",
		"/System/Library/Fonts/Monaco.ttf",
	}
	candidates = append(candidates, defaultPaths...)
	return candidates
}

// loadBestFont attempts to find a suitable monospace TTF font on the system.
// If none is found, it falls back to a built-in bitmap font.
func loadBestFont(fontName string, size float64, dpi float64) (font.Face, int, int) {
	if size <= 0 {
		size = 16.0
	}

	for _, path := range getFontCandidates(fontName) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		f, err := opentype.Parse(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "GUI_FONT: Error parsing %s: %v\n", path, err)
			continue
		}

		face, err := opentype.NewFace(f, &opentype.FaceOptions{
			Size:    size,
			DPI:     dpi,
			Hinting: font.HintingFull,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "GUI_FONT: Error creating face for %s: %v\n", path, err)
			continue
		}

		metrics := face.Metrics()
		cellH := (metrics.Ascent + metrics.Descent).Ceil()
		advance, _ := face.GlyphAdvance('A')
		cellW := advance.Ceil()

		msg := fmt.Sprintf("GUI_FONT: Successfully loaded %s (%dx%d)", path, cellW, cellH)
		fmt.Fprintln(os.Stderr, msg)
		DebugLog("%s", msg)
		return face, cellW, cellH
	}

	// Fallback to basicfont if no TTF found
	DebugLog("GUI_FONT: CRITICAL - No TTF font found! Falling back to basicfont 7x13 (ASCII only!)")
	return basicfont.Face7x13, 7, 13
}
