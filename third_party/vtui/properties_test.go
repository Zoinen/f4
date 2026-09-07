package vtui

import (
	"errors"
	"reflect"
	"testing"
)

func TestPropertyAccess_ScreenObject(t *testing.T) {
	so := &ScreenObject{}

	// 1. ID
	if err := so.SetProperty("id", PropValString("my_id")); err != nil {
		t.Fatalf("SetProperty id failed: %v", err)
	}
	if v, ok := so.GetProperty("id"); !ok || v.S != "my_id" {
		t.Errorf("GetProperty id mismatch: %v, %v", v, ok)
	}

	// 2. Visible
	if err := so.SetProperty("visible", PropValBool(false)); err != nil {
		t.Fatalf("SetProperty visible failed: %v", err)
	}
	if v, ok := so.GetProperty("visible"); !ok || v.B != false {
		t.Errorf("GetProperty visible mismatch: %v, %v", v, ok)
	}

	// 3. Enabled
	if err := so.SetProperty("enabled", PropValBool(false)); err != nil {
		t.Fatalf("SetProperty enabled failed: %v", err)
	}
	if v, ok := so.GetProperty("enabled"); !ok || v.B != false {
		t.Errorf("GetProperty enabled mismatch: %v, %v", v, ok)
	}
	if !so.IsDisabled() {
		t.Error("SetProperty enabled=false did not disable ScreenObject")
	}

	// 4. Type mismatch error
	if err := so.SetProperty("visible", PropValInt(123)); !errors.Is(err, ErrPropertyType) {
		t.Errorf("Expected ErrPropertyType, got: %v", err)
	}

	// 5. Unknown property error
	if err := so.SetProperty("non_existent", PropValString("val")); !errors.Is(err, ErrUnknownProperty) {
		t.Errorf("Expected ErrUnknownProperty, got: %v", err)
	}
	if _, ok := so.GetProperty("non_existent"); ok {
		t.Error("Expected GetProperty on unknown property to return false")
	}
}

func TestPropertyAccess_Widgets(t *testing.T) {
	SetDefaultPalette()

	t.Run("Button", func(t *testing.T) {
		b := NewButton(0, 0, "&Save")
		if err := b.SetProperty("text", PropValString("&Submit")); err != nil {
			t.Fatal(err)
		}
		if v, ok := b.GetProperty("text"); !ok || v.S != "Submit" {
			t.Errorf("Button text mismatch: got %v, ok=%v", v, ok)
		}

		if err := b.SetProperty("default", PropValBool(true)); err != nil {
			t.Fatal(err)
		}
		if v, ok := b.GetProperty("default"); !ok || v.B != true {
			t.Errorf("Button default mismatch: got %v, ok=%v", v, ok)
		}

		if err := b.SetProperty("command", PropValInt(1001)); err != nil {
			t.Fatal(err)
		}
		if v, ok := b.GetProperty("command"); !ok || v.I != 1001 {
			t.Errorf("Button command mismatch: got %v, ok=%v", v, ok)
		}
	})

	t.Run("Checkbox", func(t *testing.T) {
		cb := NewCheckbox(0, 0, "Check", false)
		if err := cb.SetProperty("state", PropValInt(1)); err != nil {
			t.Fatal(err)
		}
		if v, ok := cb.GetProperty("state"); !ok || v.I != 1 {
			t.Errorf("Checkbox state mismatch: got %v, ok=%v", v, ok)
		}

		if err := cb.SetProperty("threeState", PropValBool(true)); err != nil {
			t.Fatal(err)
		}
		if v, ok := cb.GetProperty("threeState"); !ok || v.B != true {
			t.Errorf("Checkbox threeState mismatch: got %v, ok=%v", v, ok)
		}
	})

	t.Run("Edit", func(t *testing.T) {
		e := NewEdit(0, 0, 10, "")
		if err := e.SetProperty("text", PropValString("Hello")); err != nil {
			t.Fatal(err)
		}
		if v, ok := e.GetProperty("text"); !ok || v.S != "Hello" {
			t.Errorf("Edit text mismatch: got %v, ok=%v", v, ok)
		}

		if err := e.SetProperty("password", PropValBool(true)); err != nil {
			t.Fatal(err)
		}
		if v, ok := e.GetProperty("password"); !ok || v.B != true {
			t.Errorf("Edit password mismatch: got %v, ok=%v", v, ok)
		}
	})

	t.Run("RadioGroup", func(t *testing.T) {
		rg := NewRadioGroup(0, 0, 1, []string{"A", "B"})
		if err := rg.SetProperty("items", PropValStringList([]string{"X", "Y", "Z"})); err != nil {
			t.Fatal(err)
		}
		if v, ok := rg.GetProperty("items"); !ok || !reflect.DeepEqual(v.L, []string{"X", "Y", "Z"}) {
			t.Errorf("RadioGroup items mismatch: got %v", v)
		}

		if err := rg.SetProperty("selected", PropValInt(2)); err != nil {
			t.Fatal(err)
		}
		if v, ok := rg.GetProperty("selected"); !ok || v.I != 2 {
			t.Errorf("RadioGroup selected mismatch: got %v", v)
		}
	})

	t.Run("ListBox", func(t *testing.T) {
		lb := NewListBox(0, 0, 10, 5, nil)
		if err := lb.SetProperty("items", PropValStringList([]string{"One", "Two"})); err != nil {
			t.Fatal(err)
		}
		if v, ok := lb.GetProperty("items"); !ok || len(v.L) != 2 {
			t.Errorf("ListBox items mismatch: got %v", v)
		}

		if err := lb.SetProperty("selected", PropValInt(1)); err != nil {
			t.Fatal(err)
		}
		if v, ok := lb.GetProperty("selected"); !ok || v.I != 1 {
			t.Errorf("ListBox selected mismatch: got %v", v)
		}
	})

	t.Run("Dialog", func(t *testing.T) {
		d := NewDialog(0, 0, 40, 10, "Title")
		if err := d.SetProperty("title", PropValString("New Title")); err != nil {
			t.Fatal(err)
		}
		if v, ok := d.GetProperty("title"); !ok || v.S != "New Title" {
			t.Errorf("Dialog title mismatch: got %v", v)
		}

		if err := d.SetProperty("isWarning", PropValBool(true)); err != nil {
			t.Fatal(err)
		}
		if v, ok := d.GetProperty("isWarning"); !ok || v.B != true {
			t.Errorf("Dialog isWarning mismatch: got %v", v)
		}
	})
}
