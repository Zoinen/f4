// Command fontprobe reports what two independent font stacks see in one font
// file: github.com/gogpu/gg/text, which the gogpu backend draws with, and
// golang.org/x/image/font/sfnt, which the X11 and Wayland backends draw with.
//
// It exists because a .notdef box on screen is ambiguous. The font may lack
// the glyph, gg may pick the wrong subfont out of a .ttc collection, or gg may
// find the glyph and fail to rasterise it. Those need different fixes and two
// of them belong upstream, so the probe prints the glyph index each library
// resolves for the same rune and lets the disagreement point at the layer.
//
// Usage:
//
//	go run ./cmd/fontprobe /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc
//	go run ./cmd/fontprobe 'C:\Windows\Fonts\msgothic.ttc' 18
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/gogpu/gg/text"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// probeRunes covers the scripts f4 actually shows in its own UI, plus the two
// Latin Extended letters visible in the language menu. A font that answers for
// Latin and refuses Han is a very different bug from one that refuses both.
var probeRunes = []struct {
	r    rune
	what string
}{
	{'A', "latin capital"},
	{'l', "latin narrow"},
	{'ñ', "latin-1"},
	{'Č', "latin ext-A"},
	{'š', "latin ext-A"},
	{'Б', "cyrillic"},
	{'中', "han"},
	{'字', "han"},
	{'日', "han"},
	{'あ', "hiragana"},
	{'한', "hangul"},
	{'│', "box drawing"},
	{'😀', "emoji"},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fontprobe <font-file> [size]")
		os.Exit(2)
	}
	path := os.Args[1]

	size := 18.0
	if len(os.Args) > 2 {
		if v, err := strconv.ParseFloat(os.Args[2], 64); err == nil && v > 0 {
			size = v
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read %s: %v\n", path, err)
		os.Exit(1)
	}

	fmt.Printf("file : %s\n", path)
	fmt.Printf("size : %.1f\n", size)
	fmt.Printf("bytes: %d\n\n", len(data))

	count := collectionCount(data)
	fmt.Printf("subfonts in file (per x/image): %d\n\n", count)

	for i := 0; i < count; i++ {
		fmt.Printf("---------- subfont %d ----------\n", i)
		reportGG(path, i, size)
		reportSfnt(data, i)
		fmt.Println()
	}
}

// collectionCount reports how many fonts the file holds. A plain .ttf answers
// one; only collections answer more. x/image is asked rather than gg because
// gg exposes no collection count, which is itself worth knowing.
func collectionCount(data []byte) int {
	if col, err := opentype.ParseCollection(data); err == nil {
		return col.NumFonts()
	}
	return 1
}

func reportGG(path string, index int, size float64) {
	src, err := text.NewFontSourceFromFile(path, text.WithCollectionIndex(index))
	if err != nil {
		fmt.Printf("gg     : FAILED to open: %v\n", err)
		return
	}
	defer src.Close()

	face := src.Face(size)
	m := face.Metrics()
	fmt.Printf("gg     : name=%q ascent=%.2f descent=%.2f lineGap=%.2f advanceA=%.2f\n",
		src.Name(), m.Ascent, m.Descent, m.LineGap, face.Advance("A"))

	for _, p := range probeRunes {
		has := face.HasGlyph(p.r)
		gid := shapedGID(face, p.r)
		adv := face.Advance(string(p.r))
		fmt.Printf("gg     :   %-12s U+%04X %-2c hasGlyph=%-5v gid=%-5d advance=%.2f\n",
			p.what, p.r, p.r, has, gid, adv)
	}
}

// shapedGID runs the rune through the same shaper the renderer uses, because
// HasGlyph consults the cmap directly while drawing goes through shaping.
// A rune that HasGlyph accepts but shaping maps to 0 is the interesting case:
// that is a .notdef on screen despite full cmap coverage.
func shapedGID(face text.Face, r rune) int {
	gid := -1
	for g := range face.Glyphs(string(r)) {
		gid = int(g.GID)
		break
	}
	return gid
}

func reportSfnt(data []byte, index int) {
	col, err := opentype.ParseCollection(data)
	if err != nil {
		fmt.Printf("x/image: FAILED to parse collection: %v\n", err)
		return
	}
	f, err := col.Font(index)
	if err != nil {
		fmt.Printf("x/image: FAILED to open subfont %d: %v\n", index, err)
		return
	}

	var buf sfnt.Buffer
	name, err := f.Name(&buf, sfnt.NameIDFamily)
	if err != nil {
		name = "<unnamed>"
	}
	fmt.Printf("x/image: name=%q numGlyphs=%d\n", name, f.NumGlyphs())

	for _, p := range probeRunes {
		gid, err := f.GlyphIndex(&buf, p.r)
		if err != nil {
			fmt.Printf("x/image:   %-12s U+%04X %-2c ERROR %v\n", p.what, p.r, p.r, err)
			continue
		}
		fmt.Printf("x/image:   %-12s U+%04X %-2c gid=%d\n", p.what, p.r, p.r, gid)
	}
}
