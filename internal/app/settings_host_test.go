package app

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestApplyGlyphStyleFollowsTheSetting(t *testing.T) {
	old := config.App.GlyphStyle
	oldStyle := vtui.CurrentGlyphStyle()
	t.Cleanup(func() {
		config.App.GlyphStyle = old
		vtui.SetGlyphStyle(oldStyle)
	})
	config.App.GlyphStyle = config.GlyphStyleRounded
	ApplyGlyphStyle()
	if vtui.CurrentGlyphStyle() != vtui.GlyphStyleRounded {
		t.Fatal("the rounded setting should select the rounded glyph set")
	}
	config.App.GlyphStyle = "junk"
	ApplyGlyphStyle()
	if vtui.CurrentGlyphStyle() != vtui.GlyphStyleClassic {
		t.Fatal("an unknown setting is the classic set")
	}
}
