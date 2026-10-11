package config

import "testing"

func TestNormalizeGlyphStyle(t *testing.T) {
	for in, want := range map[string]string{
		"":         GlyphStyleClassic,
		"classic":  GlyphStyleClassic,
		"rounded":  GlyphStyleRounded,
		" Rounded": GlyphStyleRounded,
		"ROUNDED ": GlyphStyleRounded,
		"round":    GlyphStyleClassic,
		"nonsense": GlyphStyleClassic,
	} {
		if got := NormalizeGlyphStyle(in); got != want {
			t.Errorf("NormalizeGlyphStyle(%q) = %q, want %q", in, got, want)
		}
	}
	if App.GlyphStyle != GlyphStyleClassic {
		t.Errorf("the default style is %q, want classic", App.GlyphStyle)
	}
}
