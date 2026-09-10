package vtui

// SemanticNode exports rows in display order, matching keyboard selection and actions.
func (t *Table) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := t.GetPosition()
	columns := make([]map[string]any, len(t.Columns))
	for i, c := range t.Columns {
		columns[i] = map[string]any{"title": c.Title, "width": c.Width, "minWidth": c.MinWidth, "alignment": int(c.Alignment)}
	}
	rows := make([]map[string]any, t.ItemCount)
	for pos := range rows {
		idx := t.rowAt(pos)
		cells := make([]string, len(t.Columns))
		for col := range cells {
			if t.cellProvider != nil {
				cells[col] = t.cellProvider.GetCellText(idx, col)
			} else if t.rowProvider != nil {
				row := t.rowProvider.Row(idx)
				if col < len(row) {
					cells[col] = row[col]
				}
			} else if idx >= 0 && idx < len(t.Rows) {
				cells[col] = t.Rows[idx].GetCellText(col)
			}
		}
		rows[pos] = map[string]any{"cells": cells}
	}
	return map[string]any{"id": SemanticID(t), "kind": "table", "x": x1, "y": y1, "w": x2 - x1 + 1, "h": y2 - y1 + 1,
		"visible": t.IsVisible(), "focused": t.IsFocused(), "disabled": t.IsDisabled(), "columns": columns, "rows": rows,
		"cursor": t.SelectPos, "top": t.TopPos, "showHeader": t.ShowHeader, "sortable": t.Sortable, "sortColumn": t.SortColumn,
		"sortAscending": t.SortAscending, "quickSearch": t.QuickSearch, "searchText": t.SearchText()}
}

func (t *Table) HandleSemanticAction(action map[string]any) bool {
	if t.IsDisabled() {
		return false
	}
	switch semanticString(action["action"]) {
	case "focus", "control.focus":
		t.SetFocus(true)
		return true
	case "control.search":
		if t.QuickSearch {
			t.SetSearchText(semanticString(action["text"]))
			return true
		}
	case "control.sort":
		col := semanticInt(action["index"])
		if t.Sortable && col >= 0 && col < len(t.Columns) {
			t.SetSort(col, t.SortColumn != col || !t.SortAscending)
			return true
		}
	case "select", "control.select", "control.activate":
		idx := semanticInt(action["index"])
		if idx < 0 || idx >= t.ItemCount {
			return false
		}
		previous := t.SelectPos
		t.SetSelectPos(idx)
		if previous != t.SelectPos && t.OnSelect != nil {
			t.OnSelect(t.SelectPos)
		}
		if semanticString(action["action"]) == "control.activate" && t.OnAction != nil {
			t.OnAction(t.SelectPos)
		}
		return true
	}
	return false
}
