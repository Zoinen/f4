package main

import (
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

// semanticStyledViewerWindowRows renders the complete bounded semantic
// window, rather than only the terminal-sized viewport in its middle.  The
// viewer renderers are kept as the single source of truth for tabs, wide
// characters and the differently coloured offset/hex/ascii regions.
//
// renderText and renderHex normally update navigation state as a side effect.
// A semantic snapshot must not do that: mouse momentum may ask for a new
// window while terminal input is using lineOffsets/eofVisible.  Save and
// restore those fields around the bounded off-screen render.
func semanticStyledViewerWindowRows(vv *ViewerView, window semanticSurfaceWindow, width int) []extui.TextRowModel {
	if vv == nil || width <= 0 || len(window.rows) == 0 {
		return window.rows
	}

	topOffset := vv.TopOffset
	lineOffsets := cloneInt64Slice(vv.lineOffsets)
	eofVisible := vv.eofVisible
	lastKnownSize := vv.lastKnownSize
	defer func() {
		vv.TopOffset = topOffset
		vv.lineOffsets = lineOffsets
		vv.eofVisible = eofVisible
		vv.lastKnownSize = lastKnownSize
	}()

	vv.TopOffset = window.start
	rowCount := len(window.rows)
	rendered := semanticRenderSurface(vv.X1, vv.Y1+1,
		vv.X1+width-1, vv.Y1+rowCount, func(scr *vtui.ScreenBuf) {
			background := vtui.Palette[ColViewerText]
			scr.FillRect(vv.X1, vv.Y1+1, vv.X1+width-1,
				vv.Y1+rowCount, ' ', background)
			if vv.Busy {
				scr.Write(vv.X1, vv.Y1+1,
					vtui.StringToCharInfo(" [ Loading... ] ", background))
				return
			}
			if vv.HexMode {
				vv.renderHex(scr, width, rowCount)
				return
			}
			vv.renderText(scr, width, rowCount)
		})

	return semanticRowsWithRenderedRunsAt(window.rows, rendered.Rows, 0)
}

// semanticStyledEditorWindowRows reuses EditorView.DisplayObject for an
// arbitrary bounded visual-row range.  This retains every detail of the
// terminal renderer (syntax, selections, crosshair, whitespace/tab display,
// horizontal scrolling and line backgrounds) without maintaining a second
// styling implementation for QML.
//
// DisplayObject observes ScrollTopRow and the frame height, so both are
// changed only for the duration of the off-screen render.  Its scrollbar is
// also restored exactly.  Syntax and wrap-engine caches may be warmed by the
// read; those caches are intentionally retained, while user-visible editor
// state is not changed.
func semanticStyledEditorWindowRows(ev *EditorView, window semanticSurfaceWindow, width int) []extui.TextRowModel {
	if ev == nil || width <= 0 || len(window.rows) == 0 {
		return window.rows
	}

	scrollTopRow := ev.ScrollTopRow
	y2 := ev.Y2
	visible := ev.IsVisible()
	scrollBar := semanticCaptureScrollBar(ev.scrollBar)
	defer func() {
		ev.ScrollTopRow = scrollTopRow
		ev.Y2 = y2
		ev.SetVisible(visible)
		semanticRestoreScrollBar(ev.scrollBar, scrollBar)
	}()

	ev.ScrollTopRow = int(window.start)
	ev.Y2 = ev.Y1 + len(window.rows)
	// SemanticNode is normally requested only for visible frames, but making
	// the off-screen render independent of that flag keeps the helper total
	// and does not leak visibility back into the live frame.
	ev.SetVisible(true)
	rendered := semanticRenderSurface(ev.X1, ev.Y1+1,
		ev.X1+width-1, ev.Y1+len(window.rows), ev.DisplayObject)
	return semanticRowsWithRenderedRunsAt(window.rows, rendered.Rows, 0)
}

func cloneInt64Slice(values []int64) []int64 {
	if values == nil {
		return nil
	}
	return append([]int64(nil), values...)
}

type semanticScrollBarSnapshot struct {
	present bool
	visible bool
	value   int
	min     int
	max     int
	pgStep  int
}

func semanticCaptureScrollBar(scrollBar *vtui.ScrollBar) semanticScrollBarSnapshot {
	if scrollBar == nil {
		return semanticScrollBarSnapshot{}
	}
	return semanticScrollBarSnapshot{
		present: true,
		visible: scrollBar.IsVisible(),
		value:   scrollBar.Value,
		min:     scrollBar.Min,
		max:     scrollBar.Max,
		pgStep:  scrollBar.PgStep,
	}
}

func semanticRestoreScrollBar(scrollBar *vtui.ScrollBar, state semanticScrollBarSnapshot) {
	if scrollBar == nil || !state.present {
		return
	}
	scrollBar.Value = state.value
	scrollBar.Min = state.min
	scrollBar.Max = state.max
	scrollBar.PgStep = state.pgStep
	scrollBar.SetVisible(state.visible)
}
