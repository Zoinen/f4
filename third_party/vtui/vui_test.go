package vtui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDistribute1D_ExactMath(t *testing.T) {
	// 3 expanding items in 30 cells with margin 2 and spacing 1
	items := []SizeSpec{
		{Hint: 5, Min: 2, Policy: PolicyExpanding, Stretch: 1},
		{Hint: 5, Min: 2, Policy: PolicyExpanding, Stretch: 1},
		{Hint: 5, Min: 2, Policy: PolicyExpanding, Stretch: 1},
	}
	// length 30, margin 2+2=4, spacing 1*2=2 -> usable 24.
	// 24 / 3 = 8 cells each.
	sizes, pos := Distribute1D(30, items, 1, 2, 2)
	if len(sizes) != 3 {
		t.Fatalf("expected 3 sizes, got %d", len(sizes))
	}
	for i, s := range sizes {
		if s != 8 {
			t.Errorf("item %d: expected size 8, got %d", i, s)
		}
	}
	expectedPos := []int{2, 11, 20}
	for i, p := range pos {
		if p != expectedPos[i] {
			t.Errorf("item %d: expected pos %d, got %d", i, expectedPos[i], p)
		}
	}
}

func TestVuiLoader_HelloDialog_Golden(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)
	defer fm.Shutdown()

	oldFM := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFM }()

	dlg, err := LoadDialogFile("testdata/hello.vui")
	if err != nil {
		t.Fatalf("Failed to load testdata/hello.vui: %v", err)
	}
	fm.Push(dlg)

	x1, y1, x2, y2 := dlg.GetPosition()
	w := x2 - x1 + 1
	h := y2 - y1 + 1

	goldenData, err := os.ReadFile("testdata/hello.golden.json")
	if err != nil {
		t.Fatalf("Failed to read golden file: %v", err)
	}

	var golden struct {
		Dialog    map[string]int `json:"dialog"`
		NameEdit  map[string]int `json:"nameEdit"`
		OkBtn     map[string]int `json:"okBtn"`
		CancelBtn map[string]int `json:"cancelBtn"`
	}
	if err := json.Unmarshal(goldenData, &golden); err != nil {
		t.Fatalf("Failed to parse golden json: %v", err)
	}

	if w != golden.Dialog["w"] || h != golden.Dialog["h"] {
		t.Errorf("Dialog size mismatch: got %dx%d, want %dx%d", w, h, golden.Dialog["w"], golden.Dialog["h"])
	}
	if x1 != golden.Dialog["x1"] || y1 != golden.Dialog["y1"] {
		t.Errorf("Dialog position mismatch: got (%d,%d), want (%d,%d)", x1, y1, golden.Dialog["x1"], golden.Dialog["y1"])
	}

	// Verify button and edit positions
	edit, ok := fm.Lookup(dlg.ID(), "nameEdit")
	if !ok {
		t.Fatal("nameEdit element not found via Lookup")
	}
	ex1, ey1, ex2, ey2 := edit.GetPosition()
	if ex1 != golden.NameEdit["x1"] || ey1 != golden.NameEdit["y1"] || ex2 != golden.NameEdit["x2"] || ey2 != golden.NameEdit["y2"] {
		t.Errorf("nameEdit position: got (%d,%d)-(%d,%d), want (%d,%d)-(%d,%d)", ex1, ey1, ex2, ey2, golden.NameEdit["x1"], golden.NameEdit["y1"], golden.NameEdit["x2"], golden.NameEdit["y2"])
	}

	okBtn, ok := fm.Lookup(dlg.ID(), "okBtn")
	if !ok {
		t.Fatal("okBtn element not found via Lookup")
	}
	bx1, by1, bx2, by2 := okBtn.GetPosition()
	if bx1 != golden.OkBtn["x1"] || by1 != golden.OkBtn["y1"] || bx2 != golden.OkBtn["x2"] || by2 != golden.OkBtn["y2"] {
		t.Errorf("okBtn position: got (%d,%d)-(%d,%d), want (%d,%d)-(%d,%d)", bx1, by1, bx2, by2, golden.OkBtn["x1"], golden.OkBtn["y1"], golden.OkBtn["x2"], golden.OkBtn["y2"])
	}
	cancelBtn, ok := fm.Lookup(dlg.ID(), "cancelBtn")
	if !ok {
		t.Fatal("cancelBtn element not found via Lookup")
	}
	cbx1, cby1, cbx2, cby2 := cancelBtn.GetPosition()
	if cbx1 != golden.CancelBtn["x1"] || cby1 != golden.CancelBtn["y1"] || cbx2 != golden.CancelBtn["x2"] || cby2 != golden.CancelBtn["y2"] {
		t.Errorf("cancelBtn position: got (%d,%d)-(%d,%d), want (%d,%d)-(%d,%d)", cbx1, cby1, cbx2, cby2, golden.CancelBtn["x1"], golden.CancelBtn["y1"], golden.CancelBtn["x2"], golden.CancelBtn["y2"])
	}

	// Verify layout rules validity
	AssertLayout(t, dlg)
}

func TestVuiLoader_ConnectionsAndBuddy(t *testing.T) {
	SetDefaultPalette()
	vuiContent := `{
		"vuiVersion": 1,
		"root": {
			"type": "Dialog",
			"id": "loginDlg",
			"props": { "title": " Login " },
			"children": [
				{ "type": "Label", "props": { "text": "&User:", "buddy": "userEdit" } },
				{ "type": "Edit", "id": "userEdit", "props": { "text": "guest" } },
				{ "type": "Button", "id": "submitBtn", "props": { "text": "&Submit" } }
			]
		},
		"connections": [
			{ "from": "submitBtn", "signal": "clicked", "command": "CmOk" }
		]
	}`

	dlg, err := LoadDialog(strings.NewReader(vuiContent))
	if err != nil {
		t.Fatalf("LoadDialog failed: %v", err)
	}

	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()
	oldFM := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFM }()
	fm.Push(dlg)

	btn, ok := fm.Lookup(dlg.ID(), "submitBtn")
	if !ok {
		t.Fatal("submitBtn not found")
	}
	button := btn.(*Button)
	if button.Command != CmOK {
		t.Errorf("Expected button command CmOK (%d), got %d", CmOK, button.Command)
	}

	edit, ok := fm.Lookup(dlg.ID(), "userEdit")
	if !ok {
		t.Fatal("userEdit not found")
	}
	e := edit.(*Edit)
	if e.GetText() != "guest" {
		t.Errorf("Expected edit text 'guest', got %q", e.GetText())
	}
}
