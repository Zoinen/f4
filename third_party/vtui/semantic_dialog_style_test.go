package vtui

import "testing"

func TestSemanticWarningDialogKeepsCenteredLayout(t *testing.T) {
	dlg := NewCenteredDialog(50, 12, "Delete")
	dlg.IsWarning = true
	text := NewText(0, 0, "This action cannot be undone.", 0)
	combo := NewComboBox(0, 0, 32, []string{"Queue"})
	dlg.AddItem(text)
	dlg.AddItem(combo)
	layout := NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 46, 8)
	layout.Add(text, Margins{}, AlignCenter)
	layout.Add(combo, Margins{Top: 1}, AlignCenter)
	layout.Apply()
	node := dlg.SemanticNode(&SemanticContext{})
	if node["isWarning"] != true {
		t.Fatalf("warning palette lost: %v", node["isWarning"])
	}
	for _, child := range node["children"].([]map[string]any) {
		if child["horizontalAlignment"] != "center" {
			t.Fatalf("%v lost centering: %v", child["kind"], child)
		}
	}
	dlg.IsWarning = false
	if dlg.SemanticNode(&SemanticContext{})["isWarning"] != false {
		t.Fatal("warning state did not clear")
	}
}
