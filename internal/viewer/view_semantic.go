package viewer

import (
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/sdk/extui"
)

// HandleSemanticAction runs a native GUI action addressed at this viewer.

func (vv *ViewerView) semanticRows() []extui.TextRowModel {
	if vv.Backend == nil {
		return nil
	}
	width := vv.X2 - vv.X1 + 1
	if vv.ScrollBar != nil {
		width--
	}
	contentHeight := vv.Y2 - vv.Y1
	if width <= 0 || contentHeight <= 0 {
		return nil
	}
	if vv.Busy {
		return []extui.TextRowModel{{Index: 0, Text: " [ Loading... ] "}}
	}
	var rows []extui.TextRowModel
	if vv.HexMode {
		currOffset := vv.TopOffset &^ 0xF
		for y := 0; y < contentHeight && currOffset < vv.Backend.Size(); y++ {
			data, err := vv.Backend.ReadAt(currOffset, 16)
			if err != nil && err != piecetable.ErrLoading {
				break
			}
			rows = append(rows, extui.TextRowModel{
				Index:  y,
				Offset: currOffset,
				Text:   semanticHexLine(currOffset, data),
			})
			currOffset += 16
		}
		return rows
	}

	currOffset := vv.TopOffset
	for y := 0; y < contentHeight; y++ {
		if currOffset >= vv.Backend.Size() {
			break
		}
		data, err := vv.Backend.ReadAt(currOffset, width*4)
		if err == piecetable.ErrLoading {
			rows = append(rows, extui.TextRowModel{Index: y, Offset: currOffset, Text: " [ Loading... ] "})
			break
		}
		if err != nil || len(data) == 0 {
			break
		}
		lineLen, textLen, _ := semanticViewerLineLen(data, width, vv.WrapMode)
		rows = append(rows, extui.TextRowModel{Index: y, Offset: currOffset, Text: string(data[:textLen])})
		if lineLen <= 0 {
			break
		}
		currOffset += int64(lineLen)
	}
	return rows
}
