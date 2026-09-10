package vtui

import "testing"

type semanticTableTestRow []string

func (r semanticTableTestRow) GetCellText(col int) string { return r[col] }
func TestSemanticTableDisplayOrderAndActions(t *testing.T) {
	table := NewTable(0, 0, 60, 12, []TableColumn{{Title: "Command", Width: 20}, {Title: "Key"}})
	table.SetRows([]TableRow{semanticTableTestRow{"Beta", "F2"}, semanticTableTestRow{"Alpha", "F1"}})
	table.Sortable = true
	table.QuickSearch = true
	if !table.HandleSemanticAction(map[string]any{"action": "control.sort", "index": 0}) {
		t.Fatal("sort rejected")
	}
	node := table.SemanticNode(nil)
	rows := node["rows"].([]map[string]any)
	if rows[0]["cells"].([]string)[0] != "Alpha" {
		t.Fatal(rows)
	}
	activated := -1
	table.OnAction = func(pos int) { activated = table.rowAt(pos) }
	table.HandleSemanticAction(map[string]any{"action": "control.activate", "index": 0})
	if activated != 1 {
		t.Fatalf("activated source row %d", activated)
	}
	table.HandleSemanticAction(map[string]any{"action": "control.search", "text": "Beta"})
	if table.ItemCount != 1 {
		t.Fatal(table.ItemCount)
	}
	if table.HandleSemanticAction(map[string]any{"action": "control.activate", "index": 2}) {
		t.Fatal("invalid row accepted")
	}
	table.HandleSemanticAction(map[string]any{"action": "control.activate", "index": 0})
	if activated != 0 {
		t.Fatal(activated)
	}
}
