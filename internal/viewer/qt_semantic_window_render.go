package viewer

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func semanticStyledViewerWindowRows(vv *ViewerView, window semantic.SemanticSurfaceWindow, width int) []extui.TextRowModel {
	if vv == nil || width <= 0 || len(window.Rows) == 0 {
		return window.Rows
	}

	topOffset := vv.TopOffset
	lineOffsets := semantic.CloneInt64Slice(vv.LineOffsets)
	urlRows := append([][]UrlCellRange(nil), vv.visibleURLRows...)
	eofVisible := vv.EofVisible
	lastKnownSize := vv.lastKnownSize
	defer func() {
		vv.TopOffset = topOffset
		vv.LineOffsets = lineOffsets
		vv.visibleURLRows = urlRows
		vv.EofVisible = eofVisible
		vv.lastKnownSize = lastKnownSize
	}()

	vv.TopOffset = window.Start
	rowCount := len(window.Rows)
	rendered := semantic.SemanticRenderSurface(vv.X1, vv.Y1+1,
		vv.X1+width-1, vv.Y1+rowCount, func(scr *vtui.ScreenBuf) {
			background := vtui.Palette[theme.ColViewerText]
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
			vv.renderTextRows(scr, width, rowCount, false)
		})

	return semantic.SemanticRowsWithContentKeys(
		semantic.SemanticRowsWithRenderedRunsAt(window.Rows, rendered.Rows, 0))
}
