# Consistent standalone menu title alignment

The standalone popup title (including Drive) and separator captions should have
consistent alignment. The user prefers left alignment for both. Preserve the
symmetric title insets, elision, font,
pixel-alignment transform and list geometry.

The existing standalone-title regression asserts left alignment, and separator
coverage checks the same alignment. Retain fractional-DPR title coverage when
changing title layout: aligning a Text item alone does not prove glyph sharpness.
