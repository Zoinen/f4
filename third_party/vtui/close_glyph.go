package vtui

import "sync"

// The close button is drawn "[×]". U+00D7 is not in the OEM code pages the
// raster fonts of the classic Windows console are made from (CP437 has the
// arrows next to it, but no multiplication sign), so there the button showed as
// "[?]" (unxed/f4#1148). EffectiveCloseSymbol gives a plain "x" in that one
// case and the configured symbol everywhere else.

var (
	rasterFontOnce sync.Once
	rasterFont     bool
)

// EffectiveCloseSymbol is the rune the close button is drawn with. It is
// UIStrings.CloseSymbol, except that the default multiplication sign becomes
// "x" when the console font is a raster one that cannot show it. An
// application that sets its own symbol gets it as it is.
func EffectiveCloseSymbol() rune {
	rasterFontOnce.Do(func() { rasterFont = consoleFontIsRaster() })
	return pickCloseSymbol(UIStrings.CloseSymbol, rasterFont)
}

// pickCloseSymbol is the decision behind EffectiveCloseSymbol.
func pickCloseSymbol(configured rune, rasterFont bool) rune {
	if configured == '×' && rasterFont {
		return 'x'
	}
	return configured
}
