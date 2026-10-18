package vtui

import (
	"errors"
	"strings"
	"testing"
)

type failingVuiReader struct{}

func (failingVuiReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func loaderElementByID(root UIElement, id string) UIElement {
	var found UIElement
	walk(root, func(el UIElement) bool {
		if el.GetId() == id {
			found = el
			return false
		}
		return true
	})
	return found
}

func TestLoadDialogReportsInputErrors(t *testing.T) {
	if _, err := LoadDialog(failingVuiReader{}); err == nil || err.Error() != "read failed" {
		t.Fatalf("LoadDialog(reader error) = %v, want read failed", err)
	}
	if _, err := LoadDialog(strings.NewReader("{")); err == nil {
		t.Fatal("LoadDialog(invalid JSON) succeeded")
	}
	if _, err := LoadDialog(strings.NewReader(`{"vuiVersion":1}`)); err == nil || !strings.Contains(err.Error(), "empty vui document") {
		t.Fatalf("LoadDialog(without root) = %v, want empty-document error", err)
	}
}

func TestLoadDialogFileOnceReportsOpenError(t *testing.T) {
	if _, err := loadDialogFileOnce(t.TempDir() + "/missing.vui"); err == nil {
		t.Fatal("loadDialogFileOnce(missing file) succeeded")
	}
}

func TestLoadVuiDocumentRejectsInvalidRootsAndWidgets(t *testing.T) {
	for name, doc := range map[string]*VuiDocument{
		"nil document": nil,
		"nil root":     {},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadVuiDocument(doc); err == nil {
				t.Fatal("LoadVuiDocument succeeded")
			}
		})
	}

	if _, err := LoadVuiDocument(&VuiDocument{Root: &VuiNode{Type: "Label"}}); err == nil || !strings.Contains(err.Error(), "root node must be Dialog or Window") {
		t.Fatalf("non-window root error = %v", err)
	}
	if _, err := LoadVuiDocument(&VuiDocument{Root: &VuiNode{
		Type:     "Dialog",
		Children: []*VuiNode{{Type: "not-a-widget"}},
	}}); err == nil {
		t.Fatal("unknown child widget succeeded")
	}
}

func TestLoadVuiDocumentAppliesLayoutAndConnections(t *testing.T) {
	SetDefaultPalette()
	doc := &VuiDocument{
		Root: &VuiNode{
			Type: "Dialog",
			ID:   "dialog",
			Props: map[string]any{
				"title":    " Test ",
				"autoSize": true,
				"center":   false,
				"width":    float64(40),
				"height":   float64(12),
			},
			Layout: &VuiLayoutDef{Type: "VBox", Spacing: float64(2), Margins: []int{1, 3, 2, 4}, Align: "center"},
			Children: []*VuiNode{
				{Type: "Label", Props: map[string]any{"text": "&Name", "buddy": "name"}},
				{Type: "Edit", ID: "name", Props: map[string]any{"text": "guest"}},
				{Type: "Group", Layout: &VuiLayoutDef{Type: "HBox", Spacing: float64(3), Margins: []int{2, 4, 6, 8}}, Children: []*VuiNode{
					{Type: "Button", ID: "ok", Props: map[string]any{"text": "&OK"}},
				}},
			},
		},
		Connections: []VuiConnection{
			{From: "missing", Command: "CmQuit"},
			{From: "ok", Command: "CmOK"},
			{From: "name", Command: "not-a-command"},
		},
	}
	win, err := LoadVuiDocument(doc)
	if err != nil {
		t.Fatalf("LoadVuiDocument: %v", err)
	}
	if x1, y1, _, _ := win.GetPosition(); x1 != 0 || y1 != 0 {
		t.Fatalf("uncentered dialog starts at (%d,%d), want (0,0)", x1, y1)
	}
	if win.MinW <= 0 || win.MinH <= 0 {
		t.Fatalf("auto-size minimum = %dx%d, want positive dimensions", win.MinW, win.MinH)
	}
	button, ok := loaderElementByID(win, "ok").(*Button)
	if !ok || button.Command != CmOK {
		t.Fatalf("connected button = %#v, want command CmOK", button)
	}
	label, ok := win.GetChildren()[0].(*Text)
	if !ok {
		t.Fatal("first child is not a label")
	}
	if label.FocusLink != loaderElementByID(win, "name") {
		t.Fatalf("label buddy = %#v, want name edit", label.FocusLink)
	}
	if _, ok := loaderElementByID(win, "name").(*Edit); !ok {
		t.Fatal("name node was not built as an Edit")
	}
}

func TestLoadVuiDocumentUsesExplicitSizeAndDefaultCentering(t *testing.T) {
	oldFrameManager := FrameManager
	FrameManager = nil
	t.Cleanup(func() { FrameManager = oldFrameManager })

	win, err := LoadVuiDocument(&VuiDocument{Root: &VuiNode{
		Type: "Dialog",
		Props: map[string]any{
			"w": float64(30),
			"h": float64(8),
		},
		Children: []*VuiNode{{Type: "Group", Layout: &VuiLayoutDef{Type: "VBox"}}},
	}})
	if err != nil {
		t.Fatalf("LoadVuiDocument: %v", err)
	}
	if x1, y1, x2, y2 := win.GetPosition(); x1 != 25 || y1 != 8 || x2 != 54 || y2 != 15 {
		t.Fatalf("centered dialog bounds = (%d,%d)-(%d,%d), want (25,8)-(54,15)", x1, y1, x2, y2)
	}
}

func TestVuiValueConversions(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want PropValue
	}{
		{"string", "text", PropValString("text")},
		{"bool", true, PropValBool(true)},
		{"float", float64(7), PropValInt(7)},
		{"int", 8, PropValInt(8)},
		{"list", []any{"a", 2, false}, PropValStringList([]string{"a", "2", "false"})},
		{"fallback", map[string]int{"x": 1}, PropValString("map[x:1]")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toPropValue(tt.in); got.String() != tt.want.String() {
				t.Fatalf("toPropValue(%#v) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}

	commands := []struct {
		in   any
		want int
	}{
		{nil, 0},
		{17, 17},
		{float64(18), 18},
		{"CmOk", CmOK},
		{"CmOK", CmOK},
		{"CmCancel", CmCancel},
		{"CmQuit", CmQuit},
		{"CmDefault", CmDefault},
		{"CmClose", CmClose},
		{"CmZoom", CmZoom},
		{"CmResize", CmResize},
		{"CmNext", CmNext},
		{"CmPrev", CmPrev},
		{"CmHelp", CmHelp},
		{"19", 19},
		{"not-a-command", 0},
		{true, 0},
	}
	for _, tt := range commands {
		if got := resolveCommandID(tt.in); got != tt.want {
			t.Errorf("resolveCommandID(%#v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
