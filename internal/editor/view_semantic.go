package editor

import (
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/sdk/extui"
)

// The editor's side of the GUI semantic protocol: what an external UI is told
// the editor contains, and what it is allowed to ask for. Go requires these
// with EditorView; the frame-level half of the protocol is internal/app's.

// GetText возвращает текущий текст редактора из PieceTable

// HandleSemanticAction обрабатывает нативные GUI-действия для EditorView

func (ev *EditorView) semanticRows() []extui.TextRowModel {
	if ev.Pt == nil || ev.Li == nil || ev.Engine == nil {
		return nil
	}
	ev.EnsureEngineWidth()
	height := ev.Y2 - ev.Y1
	if height <= 0 {
		return nil
	}
	startLogLine, startFragIdx := ev.Engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	var rows []extui.TextRowModel
	for logIdx := startLogLine; logIdx < ev.Li.LineCount() && len(rows) < height; logIdx++ {
		frags := ev.Engine.GetFragments(logIdx)
		baseVRow := ev.Engine.GetRowOffset(logIdx)
		for fIdx, frag := range frags {
			if logIdx == startLogLine && fIdx < startFragIdx {
				continue
			}
			data, err := ev.Pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
			text := string(data)
			if err == piecetable.ErrLoading {
				text = " [ Loading... ] "
			} else if err != nil {
				text = ""
			}
			rows = append(rows, extui.TextRowModel{
				Index:       len(rows),
				VisualRow:   baseVRow + fIdx,
				LogicalLine: logIdx,
				Offset:      int64(frag.ByteOffsetStart),
				Text:        text,
			})
			if len(rows) >= height {
				break
			}
		}
	}
	return rows
}
