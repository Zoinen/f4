package main

import (
	"time"

	"github.com/unxed/vtui"
)

// syntaxFadeDuration is how long the colours take to arrive. Long enough to
// read as a transition rather than a flicker, short enough that nobody waits
// for it.
const syntaxFadeDuration = 400 * time.Millisecond

// fadeSyntax eases highlighting in instead of snapping to it.
//
// Colorer parses in the background, so the first highlighted frame replaces a
// screenful of plain text all at once, and the eye reads that as the editor
// blinking. Blending each attribute from the plain colour towards its final
// one over a few frames turns the same wait into something calm.
//
// Indexed colours are left alone: there is nothing meaningful between palette
// slot 3 and slot 7, so those simply appear.
func (ev *EditorView) fadeSyntax(syntax []uint64, base uint64) []uint64 {
	if len(syntax) == 0 || !AppConfig.EditorSyntaxAnimation {
		return syntax
	}
	if vtui.FrameManager != nil {
		if screen := vtui.FrameManager.Screen(); screen != nil && screen.Renderer != nil {
			if renderer, ok := screen.Renderer.(interface {
				WantsEditorSyntaxFade() bool
			}); ok && !renderer.WantsEditorSyntaxFade() {
				return syntax
			}
		}
	}
	if ev.syntaxFadeStart.IsZero() {
		ev.syntaxFadeStart = time.Now()
		ev.ensureFade()
	}

	elapsed := time.Since(ev.syntaxFadeStart)
	if elapsed >= syntaxFadeDuration {
		return syntax
	}
	f := float64(elapsed) / float64(syntaxFadeDuration)

	ev.fadeBuf = append(ev.fadeBuf[:0], syntax...)
	for i, a := range ev.fadeBuf {
		if a == 0 {
			continue
		}
		if a&vtui.IsFgRGB != 0 && base&vtui.IsFgRGB != 0 {
			a = vtui.SetRGBFore(a, mixRGB(vtui.GetRGBFore(base), vtui.GetRGBFore(a), f))
		}
		if a&vtui.IsBgRGB != 0 && base&vtui.IsBgRGB != 0 {
			a = vtui.SetRGBBack(a, mixRGB(vtui.GetRGBBack(base), vtui.GetRGBBack(a), f))
		}
		ev.fadeBuf[i] = a
	}
	return ev.fadeBuf
}

// ensureFade keeps the vtui heartbeat redrawing while the fade runs; the
// callback removes itself once the colours have arrived.
func (ev *EditorView) ensureFade() {
	if ev.fadeReg {
		return
	}
	ev.fadeReg = true
	vtui.FrameManager.AddAnimation(ev.fadeTick)
}

// fadeTick is the heartbeat callback: idle once the fade is over.
func (ev *EditorView) fadeTick(float64) bool {
	if time.Since(ev.syntaxFadeStart) >= syntaxFadeDuration {
		ev.fadeReg = false
		return true
	}
	return false
}

// mixRGB walks each channel from one packed colour to another.
func mixRGB(from, to uint32, f float64) uint32 {
	r := uint32(float64((from>>16)&0xFF)+(float64((to>>16)&0xFF)-float64((from>>16)&0xFF))*f) & 0xFF
	g := uint32(float64((from>>8)&0xFF)+(float64((to>>8)&0xFF)-float64((from>>8)&0xFF))*f) & 0xFF
	b := uint32(float64(from&0xFF)+(float64(to&0xFF)-float64(from&0xFF))*f) & 0xFF
	return r<<16 | g<<8 | b
}
