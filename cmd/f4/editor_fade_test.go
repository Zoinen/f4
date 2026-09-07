package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestEditorSyntaxFadeDoesNotStartTickerForNativeQtSurface(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	screen.Renderer = &ExtUiRenderer{nativeSemanticSurfaceEnabled: true}
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })

	editor := &EditorView{}
	syntax := []uint64{vtui.IsFgRGB | 0x00112233}
	if got := editor.fadeSyntax(syntax, vtui.IsFgRGB|0x00445566); len(got) != 1 || got[0] != syntax[0] {
		t.Fatalf("native editor syntax was unexpectedly blended: %#v", got)
	}
	if !editor.syntaxFadeStart.IsZero() {
		t.Fatal("native editor syntax started the 25 ms redraw ticker")
	}
}

func TestFadeSyntax_DisabledByDefault(t *testing.T) {
	old := AppConfig.EditorSyntaxAnimation
	AppConfig.EditorSyntaxAnimation = false
	t.Cleanup(func() { AppConfig.EditorSyntaxAnimation = old })

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
