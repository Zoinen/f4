package editor

import (
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestFadeSyntax_DisabledByDefault(t *testing.T) {
	old := config.App.EditorSyntaxAnimation
	config.App.EditorSyntaxAnimation = false
	t.Cleanup(func() { config.App.EditorSyntaxAnimation = old })

	ev := &EditorView{}
	syntax := []uint64{0x123, 0x456, 0x789}
	got := ev.fadeSyntax(syntax, 0xabc)

	if len(got) != len(syntax) {
		t.Fatalf("disabled fade changed attribute length: got %d, want %d", len(got), len(syntax))
	}
	for i := range syntax {
		if got[i] != syntax[i] {
			t.Fatalf("disabled fade changed attribute %d: got %#x, want %#x", i, got[i], syntax[i])
		}
	}
	if &got[0] != &syntax[0] {
		t.Fatal("disabled fade allocated a replacement attribute buffer")
	}
	if !ev.syntaxFadeStart.IsZero() {
		t.Fatal("disabled fade started a timer")
	}
	if ev.fadeReg {
		t.Fatal("disabled fade registered a heartbeat animation")
	}
}

func TestFadeSyntaxEmptyAndComplete(t *testing.T) {
	old := config.App.EditorSyntaxAnimation
	config.App.EditorSyntaxAnimation = true
	t.Cleanup(func() { config.App.EditorSyntaxAnimation = old })

	ev := &EditorView{}
	if got := ev.fadeSyntax(nil, 0); got != nil {
		t.Fatalf("empty syntax = %#v, want nil", got)
	}

	syntax := []uint64{vtui.SetRGBBoth(0, 0xABCDEF, 0x123456)}
	ev.syntaxFadeStart = time.Now().Add(-syntaxFadeDuration)
	if got := ev.fadeSyntax(syntax, vtui.SetRGBBoth(0, 0x000000, 0xFFFFFF)); &got[0] != &syntax[0] {
		t.Fatal("completed fade allocated a replacement buffer")
	}
	if ev.fadeTick(0) != true {
		t.Fatal("completed fade heartbeat remained active")
	}
	if ev.fadeReg {
		t.Fatal("completed fade did not clear heartbeat registration")
	}
}

func TestMixRGBInterpolatesChannels(t *testing.T) {
	if got := mixRGB(0x000000, 0xFFFFFF, 0); got != 0x000000 {
		t.Fatalf("mixRGB at start = %#x", got)
	}
	if got := mixRGB(0x000000, 0xFFFFFF, 1); got != 0xFFFFFF {
		t.Fatalf("mixRGB at end = %#x", got)
	}
	if got := mixRGB(0x000000, 0xFFFFFF, 0.5); got != 0x7F7F7F {
		t.Fatalf("mixRGB at midpoint = %#x, want 0x7f7f7f", got)
	}
}
