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
	if len(syntax) == 0 {
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
		// The frame heartbeat is too slow to carry a fade on its own.
		go func() {
			tick := time.NewTicker(25 * time.Millisecond)
			defer tick.Stop()
			deadline := time.After(syntaxFadeDuration)
			for {
				select {
				case <-tick.C:
					vtui.FrameManager.Redraw()
				case <-deadline:
					vtui.FrameManager.Redraw()
					return
				}
			}
		}()
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

// mixRGB walks each channel from one packed colour to another.
func mixRGB(from, to uint32, f float64) uint32 {
	ch := func(shift uint) uint32 {
		a := float64((from >> shift) & 0xFF)
		b := float64((to >> shift) & 0xFF)
		return uint32(a+(b-a)*f) & 0xFF
	}
	return ch(16)<<16 | ch(8)<<8 | ch(0)
}
