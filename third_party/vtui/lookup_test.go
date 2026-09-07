package vtui

import (
	"testing"
)

func TestLookup_ReadmeDialog(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	FrameManager.Init(scr)
	defer FrameManager.Shutdown()

	dlg := NewDialog(0, 0, 40, 10, " Hello vtui ")
	dlg.SetID("mainDlg")

	edit := NewEdit(dlg.X1+2, dlg.Y1+3, 36, "Type here...")
	edit.SetID("nameEdit")
	lbl := NewLabel(dlg.X1+2, dlg.Y1+2, "&Name:", edit)
	lbl.SetID("nameLabel")

	btn := NewButton(dlg.X1+16, dlg.Y1+7, "&Ok")
	btn.SetID("okBtn")

	dlg.AddItem(lbl)
	dlg.AddItem(edit)
	dlg.AddItem(btn)

	FrameManager.Push(dlg)

	// 1. Lookup Dialog by ID
	foundDlg, ok := FrameManager.Lookup("mainDlg", "")
	if !ok || foundDlg != dlg {
		t.Errorf("Lookup(mainDlg, \"\") failed: got %v, ok=%v", foundDlg, ok)
	}

	// 2. Lookup child elements within dialog
	foundEdit, ok := FrameManager.Lookup("mainDlg", "nameEdit")
	if !ok || foundEdit != edit {
		t.Errorf("Lookup(mainDlg, nameEdit) failed: got %v, ok=%v", foundEdit, ok)
	}

	foundBtn, ok := FrameManager.Lookup("mainDlg", "okBtn")
	if !ok || foundBtn != btn {
		t.Errorf("Lookup(mainDlg, okBtn) failed: got %v, ok=%v", foundBtn, ok)
	}

	foundLbl, ok := FrameManager.Lookup("mainDlg", "nameLabel")
	if !ok || foundLbl != lbl {
		t.Errorf("Lookup(mainDlg, nameLabel) failed: got %v, ok=%v", foundLbl, ok)
	}

	// 3. Lookup on active frame (empty frameID)
	foundOnActive, ok := FrameManager.Lookup("", "okBtn")
	if !ok || foundOnActive != btn {
		t.Errorf("Lookup(\"\", okBtn) on active frame failed: got %v, ok=%v", foundOnActive, ok)
	}

	// 4. Lookup non-existent
	if _, ok := FrameManager.Lookup("mainDlg", "non_existent"); ok {
		t.Error("Lookup should return false for non-existent element ID")
	}
	if _, ok := FrameManager.Lookup("non_existent_frame", "okBtn"); ok {
		t.Error("Lookup should return false for non-existent frame ID")
	}
}

func TestAutoID_Generation(t *testing.T) {
	g := NewGroup(0, 0, 40, 20)

	b1 := NewButton(0, 0, "B1")
	b2 := NewButton(0, 0, "B2")
	e1 := NewEdit(0, 0, 10, "")

	g.AddItem(b1)
	g.AddItem(b2)
	g.AddItem(e1)

	if b1.ID() != "auto:Button:1" {
		t.Errorf("Expected b1 ID 'auto:Button:1', got %q", b1.ID())
	}
	if b2.ID() != "auto:Button:2" {
		t.Errorf("Expected b2 ID 'auto:Button:2', got %q", b2.ID())
	}
	if e1.ID() != "auto:Edit:1" {
		t.Errorf("Expected e1 ID 'auto:Edit:1', got %q", e1.ID())
	}
}

func TestAutoID_DuplicateExplicitPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic on adding duplicate explicit element ID, got none")
		}
	}()

	g := NewGroup(0, 0, 40, 20)
	b1 := NewButton(0, 0, "B1")
	b1.SetID("same_id")
	b2 := NewButton(0, 0, "B2")
	b2.SetID("same_id")

	g.AddItem(b1)
	g.AddItem(b2)
}
