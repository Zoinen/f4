package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/frameborder"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/vtui"
)

// RenderDialogOuterBorder optionally reserves an empty border outside the
// border of a modal dialog or the user menu, matching the far2l/Far3
// "outer border" look requested in f4#1399 (the reporter finds f4's
// current single-frame look, without that extra ring, too close to Dos
// Navigator's). The ring is blank space: a second box drawn on top of the
// frame's own border would read as a doubled frame, which is the look the
// reporter does not want. It is purely cosmetic and off by default.
//
// It runs from vtui's FrameManager.AfterFrameShow, right after the frame
// painted its own border -- the same extension point dialog.RenderHelpFrame
// already uses to add a hint on the Help window's border (see #378) -- so
// the extra ring stays in that frame's place in the paint stack: a frame
// above still covers it, and nothing outside vtui needs to change.
func RenderDialogOuterBorder(scr *vtui.ScreenBuf, frame vtui.Frame) {
	if !config.App.DialogOuterBorder || !isDialogOuterBorderFrame(frame) {
		return
	}
	x1, y1, x2, y2 := frame.GetPosition()
	if x2 <= x1 || y2 <= y1 {
		return
	}
	// Sample the frame's own border so the empty ring reuses whatever
	// attribute the frame just drew there -- the normal dialog box colour,
	// a themed variant, or (for IsWarning dialogs) the warning box colour
	// -- instead of hard-coding a palette index. frameborder.Attr keeps the
	// probe on a border cell that is actually inside the buffer: a dialog
	// dragged past the left or the top edge has its top-left corner
	// off-screen, and GetCell answers an off-screen coordinate with a zero
	// cell, which would paint the whole ring in the wrong colour.
	attr, ok := frameborder.Attr(scr, x1, y1, x2, y2)
	if !ok {
		return
	}
	p := vtui.NewPainter(scr)
	// Four strips, not one enclosing rectangle: the latter would erase the
	// frame it decorates. The sides are three cells wide, the top and the
	// bottom one cell tall; each strip covers its corners, so together they
	// form an unbroken ring around the frame.
	p.Fill(x1-3, y1-1, x2+3, y1-1, ' ', attr)
	p.Fill(x1-3, y2+1, x2+3, y2+1, ' ', attr)
	p.Fill(x1-3, y1, x1-1, y2, ' ', attr)
	p.Fill(x2+1, y1, x2+3, y2, ' ', attr)

	// The frame manager lays a frame's shadow down before Show, in the very
	// cells this ring has just claimed, so the ring wipes it out. Lay it
	// again just outside the ring, using the shape vtui uses for a frame --
	// a two-cell wide right column and a one-row bottom strip -- pushed out
	// by the ring's own size.
	if frame.HasShadow() {
		scr.ApplyShadow(x1-1, y2+2, x2+5, y2+2)
		scr.ApplyShadow(x2+4, y1, x2+5, y2+1)
	}
}

// isDialogOuterBorderFrame reports whether frame is one of the two kinds
// f4#1399 asked for: a modal dialog window, or the user menu. It deliberately
// excludes ordinary popup/context menus and non-modal windows (editor,
// viewer, panels, ...), which never get the extra ring.
//
// GetType and IsModal are vtui.Frame interface methods, so they resolve
// correctly through any number of embedding layers (several f4 dialogs wrap
// *vtui.Window in their own struct, e.g. dialog.FileDialog) without needing
// a concrete type assertion for every wrapper. The user menu is the one
// exception: it is a *vtui.VMenu under the hood, whose GetType() reports
// TypeMenu the same as any other menu, so it needs its own concrete check
// against panel.UserMenuFrame, the wrapper f4 pushes it as.
func isDialogOuterBorderFrame(frame vtui.Frame) bool {
	if frame.GetType() == vtui.TypeDialog {
		return true
	}
	_, isUserMenu := frame.(*panel.UserMenuFrame)
	return isUserMenu
}
