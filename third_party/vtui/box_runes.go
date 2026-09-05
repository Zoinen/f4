package vtui

// isBoxDrawRune pre-filters runes for drawBoxGlyph/drawCustomChar so the
// shape switches do not run for every letter on screen. Generous on
// purpose: a false positive only costs a switch, a miss would silently
// fall back to the font.
//
// No build tag: gogpu_renderer.go compiles on a wider platform set than
// gui_boxdraw.go (which holds the shape rasterizers).
func isBoxDrawRune(r rune) bool {
	return (r >= 0x2500 && r <= 0x259F) || (r >= 0x2190 && r <= 0x2195) ||
		r == 0x25B2 || r == 0x25BC
}
