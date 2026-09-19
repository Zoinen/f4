package terminal

// Callers hold mu; alternate screens have no scrollback.
func (tv *TerminalView) selectionHistoryRows() int {
	if tv.UseAltScreen {
		return 0
	}
	return tv.semanticPieceRowsUnsafe() + len(tv.GridHistory)
}

func (tv *TerminalView) selectionDocumentY(y int) int {
	return tv.selectionHistoryRows() + y - tv.Y1 - tv.showOffset
}

func (tv *TerminalView) selectionScreenY(row int) int {
	return row - tv.selectionHistoryRows() + tv.Y1 + tv.showOffset
}

// A region scroll discards rows rather than appending them to the document.
// Clear a discarded selection instead of highlighting unrelated replacement text.
func (tv *TerminalView) scrollSelection(top, bottom, delta int) {
	if !tv.SelActive {
		return
	}
	origin := tv.selectionHistoryRows()
	for _, row := range []*int{&tv.selStartY, &tv.selEndY} {
		local := *row - origin
		if local < top || local > bottom {
			continue
		}
		local += delta
		if local < top || local > bottom {
			tv.SelActive = false
			return
		}
		*row += delta
	}
}
