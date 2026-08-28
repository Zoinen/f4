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
