//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// fallbackFontPaths is consulted in order, so the entries are sorted by how
// much of Unicode they carry rather than by how likely they are to exist: a
// missing file costs one failed stat, whereas a narrow font that happens to
// be listed first would answer for glyphs a later, wider font renders better.
//
// The list is deliberately long. Distributions disagree about where Noto CJK
// lives, and a Windows install outside a CJK locale has none of the Japanese
// supplemental fonts, so a short list quietly degrades to no fallback at all.
var fallbackFontPaths = []string{
	// Linux — CJK
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/opentype/noto/NotoSansCJK-VF.otf.ttc",
	"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/noto-cjk/NotoSansCJK-VF.otf.ttc",
	"/usr/share/fonts/google-noto-cjk/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/opentype/source-han-sans/SourceHanSans-Regular.otc",
	"/usr/share/fonts/adobe-source-han-sans/SourceHanSans-Regular.otc",
	"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
	"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
	"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	"/usr/share/fonts/wqy-microhei/wqy-microhei.ttc",
	"/usr/share/fonts/truetype/arphic/uming.ttc",
	"/usr/share/fonts/truetype/arphic/ukai.ttc",
	// Linux — emoji and general symbol coverage
	"/usr/share/fonts/noto/NotoColorEmoji.ttf",
	"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/TTF/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/unifont/unifont.ttf",
	// Windows — CJK. Only the Yu Gothic and Microsoft families ship outside a
	// CJK locale; msgothic/msmincho/meiryo arrive with the Japanese
	// supplemental fonts and are frequently absent.
	`C:\Windows\Fonts\msyh.ttc`,
	`C:\Windows\Fonts\msjh.ttc`,
	`C:\Windows\Fonts\YuGothM.ttc`,
	`C:\Windows\Fonts\YuGothR.ttc`,
	`C:\Windows\Fonts\simsun.ttc`,
	`C:\Windows\Fonts\malgun.ttf`,
	`C:\Windows\Fonts\msgothic.ttc`,
	`C:\Windows\Fonts\msmincho.ttc`,
	`C:\Windows\Fonts\meiryo.ttc`,
	`C:\Windows\Fonts\mingliu.ttc`,
	`C:\Windows\Fonts\batang.ttc`,
	`C:\Windows\Fonts\gulim.ttc`,
	`C:\Windows\Fonts\arialuni.ttf`,
	// Windows — emoji and symbols
	`C:\Windows\Fonts\seguiemj.ttf`,
	`C:\Windows\Fonts\seguisym.ttf`,
	`C:\Windows\Fonts\segoeui.ttf`,
	// macOS
	"/System/Library/Fonts/PingFang.ttc",
	"/System/Library/Fonts/Hiragino Sans GB.ttc",
	"/System/Library/Fonts/AppleSDGothicNeo.ttc",
	"/System/Library/Fonts/STHeiti Light.ttc",
	"/System/Library/Fonts/Supplemental/Songti.ttc",
	"/System/Library/Fonts/Apple Color Emoji.ttc",
	"/Library/Fonts/Arial Unicode.ttf",
}

func parseFontBytes(data []byte) (*opentype.Font, error) {
	f, err := opentype.Parse(data)
	if err == nil {
		return f, nil
	}
	col, err2 := opentype.ParseCollection(data)
	if err2 == nil && col.NumFonts() > 0 {
		f, err3 := col.Font(0)
		if err3 == nil {
			return f, nil
		}
	}
	return nil, err
}

type fallbackFace struct {
	faces []font.Face
}

func (f *fallbackFace) Close() error {
	var err error
	for _, face := range f.faces {
		if e := face.Close(); e != nil {
			err = e
		}
	}
	return err
}

func (f *fallbackFace) Metrics() font.Metrics {
	if len(f.faces) > 0 {
		return f.faces[0].Metrics()
	}
	return font.Metrics{}
}

func (f *fallbackFace) Kern(r0, r1 rune) fixed.Int26_6 {
	if len(f.faces) > 0 {
		return f.faces[0].Kern(r0, r1)
	}
	return 0
}

func (f *fallbackFace) GlyphBounds(r rune) (bounds fixed.Rectangle26_6, advance fixed.Int26_6, ok bool) {
	for _, face := range f.faces {
		bounds, advance, ok = face.GlyphBounds(r)
		if ok {
			return bounds, advance, ok
		}
	}
	if len(f.faces) > 0 {
		return f.faces[0].GlyphBounds(r)
	}
	return fixed.Rectangle26_6{}, 0, false
}

func (f *fallbackFace) GlyphAdvance(r rune) (advance fixed.Int26_6, ok bool) {
	for _, face := range f.faces {
		advance, ok = face.GlyphAdvance(r)
		if ok {
			return advance, ok
		}
	}
	if len(f.faces) > 0 {
		return f.faces[0].GlyphAdvance(r)
	}
	return 0, false
}

func (f *fallbackFace) Glyph(dot fixed.Point26_6, r rune) (dr image.Rectangle, mask image.Image, maskp image.Point, advance fixed.Int26_6, ok bool) {
	for _, face := range f.faces {
		dr, mask, maskp, advance, ok = face.Glyph(dot, r)
		if ok {
			return dr, mask, maskp, advance, ok
		}
	}
	if len(f.faces) > 0 {
		return f.faces[0].Glyph(dot, r)
	}
	return image.Rectangle{}, nil, image.Point{}, 0, false
}

func getFontCandidates(fontName string) []string {
	var candidates []string
	if fontName != "" {
		candidates = append(candidates, fontName)
		if !strings.HasSuffix(strings.ToLower(fontName), ".ttf") {
			candidates = append(candidates, fontName+".ttf")
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
			if !strings.HasSuffix(strings.ToLower(fontName), ".ttf") {
				candidates = append(candidates, filepath.Join(dir, fontName+".ttf"))
			}
			entries, err := os.ReadDir(dir)
			if err == nil {
				for _, e := range entries {
					if e.IsDir() {
						candidates = append(candidates, filepath.Join(dir, e.Name(), fontName))
						if !strings.HasSuffix(strings.ToLower(fontName), ".ttf") {
							candidates = append(candidates, filepath.Join(dir, e.Name(), fontName+".ttf"))
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
		"/System/Library/Fonts/Monaco.ttf",
	}
	candidates = append(candidates, defaultPaths...)
	return candidates
}

// loadBestFont attempts to find a suitable monospace TTF font on the system.
// If none is found, it falls back to a built-in bitmap font.
func loadBestFont(fontName string, size float64, dpi float64) (font.Face, int, int) {
	if size <= 0 {
		size = 18.0
	}

	var primaryFace font.Face
	var cellW, cellH int

	for _, path := range getFontCandidates(fontName) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		f, err := parseFontBytes(data)
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
		cellH = (metrics.Ascent + metrics.Descent).Ceil()
		advance, _ := face.GlyphAdvance('A')
		cellW = advance.Ceil()

		msg := fmt.Sprintf("GUI_FONT: Successfully loaded %s (%dx%d)", path, cellW, cellH)
		fmt.Fprintln(os.Stderr, msg)
		DebugLog("%s", msg)
		primaryFace = face
		break
	}

	if primaryFace == nil {
		// Fallback to basicfont if no TTF found
		DebugLog("GUI_FONT: CRITICAL - No TTF font found! Falling back to basicfont 7x13 (ASCII only!)")
		return basicfont.Face7x13, 7, 13
	}

	faces := []font.Face{primaryFace}

	for _, path := range fallbackFontPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f, err := parseFontBytes(data)
		if err != nil {
			continue
		}
		face, err := opentype.NewFace(f, &opentype.FaceOptions{
			Size:    size,
			DPI:     dpi,
			Hinting: font.HintingFull,
		})
		if err != nil {
			continue
		}
		faces = append(faces, face)
	}

	return &fallbackFace{faces: faces}, cellW, cellH
}
