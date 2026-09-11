package vtui

// SemanticNode exports the logical document, never console wrapping or viewport
// clipping. Cursor and selection offsets count Unicode runes, including newlines.
func (m *MultiLineEdit) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := m.GetPosition()
	cursor := m.semanticOffset(m.curRow, m.curCol)
	anchor := cursor
	if m.selActive {
		anchor = m.semanticOffset(m.selStartRow, m.selStartCol)
	}
	return map[string]any{
		"id": SemanticID(m), "kind": "multiLineEdit",
		"x": x1, "y": y1, "w": x2 - x1 + 1, "h": y2 - y1 + 1,
		"visible": m.IsVisible(), "focused": m.IsFocused(), "disabled": m.IsDisabled(),
		"text": m.GetText(), "cursor": cursor,
		"selectionActive": m.IsFocused() && anchor != cursor,
		"selectionStart":  anchor, "selectionEnd": cursor,
	}
}

func (m *MultiLineEdit) semanticOffset(row, col int) int {
	for _, line := range m.lines[:row] {
		col += len(line) + 1
	}
	return col
}

func (m *MultiLineEdit) semanticPosition(offset int) (int, int) {
	offset = max(0, offset)
	for row, line := range m.lines {
		if offset <= len(line) || row == len(m.lines)-1 {
			return row, min(offset, len(line))
		}
		offset -= len(line) + 1
	}
	return 0, 0
}

func (m *MultiLineEdit) HandleSemanticAction(action map[string]any) bool {
	if m.IsDisabled() {
		return false
	}
	switch semanticString(action["action"]) {
	case "focus", "control.focus":
		m.SetFocus(true)
	case "select", "control.select":
		m.selStartRow, m.selStartCol = m.semanticPosition(semanticInt(action["anchor"]))
		m.curRow, m.curCol = m.semanticPosition(semanticInt(action["cursor"]))
		m.selActive = m.selStartRow != m.curRow || m.selStartCol != m.curCol
		m.ensureVisible()
	case "set_text", "control.setText":
		m.SetText(semanticString(action["text"]))
	case "insert_text", "control.insertText":
		m.insertString(semanticString(action["text"]))
	default:
		return false
	}
	return true
}
