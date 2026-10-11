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

func TestPropKind_String(t *testing.T) {
	cases := []struct {
		name string
		kind PropKind
		want string
	}{
		{"string", PropString, "string"},
		{"int", PropInt, "int"},
		{"bool", PropBool, "bool"},
		{"color", PropColor, "color"},
		{"stringList", PropStringList, "stringList"},
		{"rect", PropRect, "rect"},
		{"unknown", PropKind(99), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.kind.String(); got != tc.want {
				t.Errorf("PropKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

func TestPropValue_String(t *testing.T) {
	cases := []struct {
		name string
		v    PropValue
		want string
	}{
		{"string", PropValString("hi"), `PropValue(string: "hi")`},
		{"int", PropValInt(42), "PropValue(int: 42)"},
		{"bool", PropValBool(true), "PropValue(bool: true)"},
		{"color", PropValColor(0xff0000), "PropValue(color: 0xff0000)"},
		{"stringList", PropValStringList([]string{"a", "b"}), "PropValue(stringList: [a b])"},
		{"rect", PropValRect(Rect{X1: 1, Y1: 2, X2: 3, Y2: 4}), "PropValue(rect: 1,2-3,4)"},
		{"invalid", PropValue{Kind: PropKind(99)}, "PropValue(invalid)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.v.String(); got != tc.want {
				t.Errorf("PropValue.String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPropValueConstructors(t *testing.T) {
	if v := PropValColor(0x123456); v.Kind != PropColor || v.C != 0x123456 {
		t.Errorf("PropValColor mismatch: %+v", v)
	}
	r := Rect{X1: 5, Y1: 6, X2: 7, Y2: 8}
	if v := PropValRect(r); v.Kind != PropRect || v.R != r {
		t.Errorf("PropValRect mismatch: %+v", v)
	}
}

// TestScreenObjectProperties_TableDriven exercises the ScreenObject.SetProperty
// / GetProperty pairs that TestPropertyAccess_ScreenObject does not already
// cover: help, grow, align, stretch and the min/max size bounds, plus their
// type-mismatch errors.
func TestScreenObjectProperties_TableDriven(t *testing.T) {
	cases := []struct {
		name    string
		prop    string
		set     PropValue
		wantErr error
		get     func(PropValue) bool // only used when wantErr is nil
	}{
		{name: "id wrong type", prop: "id", set: PropValInt(1), wantErr: ErrPropertyType},
		{name: "enabled wrong type", prop: "enabled", set: PropValInt(1), wantErr: ErrPropertyType},

		{name: "help", prop: "help", set: PropValString("topic.help"),
			get: func(v PropValue) bool { return v.S == "topic.help" }},
		{name: "help wrong type", prop: "help", set: PropValInt(1), wantErr: ErrPropertyType},

		{name: "grow", prop: "grow", set: PropValInt(int(GrowAll)),
			get: func(v PropValue) bool { return v.I == int(GrowAll) }},
		{name: "grow wrong type", prop: "grow", set: PropValString("x"), wantErr: ErrPropertyType},

		{name: "align", prop: "align", set: PropValString("left"),
			get: func(v PropValue) bool { return v.S == "left" }},
		{name: "align wrong type", prop: "align", set: PropValInt(1), wantErr: ErrPropertyType},

		{name: "stretch", prop: "stretch", set: PropValInt(3),
			get: func(v PropValue) bool { return v.I == 3 }},
		{name: "stretch wrong type", prop: "stretch", set: PropValString("x"), wantErr: ErrPropertyType},

		{name: "minWidth", prop: "minWidth", set: PropValInt(10),
			get: func(v PropValue) bool { return v.I == 10 }},
		{name: "minWidth wrong type", prop: "minWidth", set: PropValString("x"), wantErr: ErrPropertyType},

		{name: "minHeight", prop: "minHeight", set: PropValInt(11),
			get: func(v PropValue) bool { return v.I == 11 }},
		{name: "minHeight wrong type", prop: "minHeight", set: PropValString("x"), wantErr: ErrPropertyType},

		{name: "maxWidth", prop: "maxWidth", set: PropValInt(12),
			get: func(v PropValue) bool { return v.I == 12 }},
		{name: "maxWidth wrong type", prop: "maxWidth", set: PropValString("x"), wantErr: ErrPropertyType},

		{name: "maxHeight", prop: "maxHeight", set: PropValInt(13),
			get: func(v PropValue) bool { return v.I == 13 }},
		{name: "maxHeight wrong type", prop: "maxHeight", set: PropValString("x"), wantErr: ErrPropertyType},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			so := &ScreenObject{}
			err := so.SetProperty(tc.prop, tc.set)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("SetProperty(%q) error = %v, want %v", tc.prop, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetProperty(%q) unexpected error: %v", tc.prop, err)
			}
			got, ok := so.GetProperty(tc.prop)
			if !ok {
				t.Fatalf("GetProperty(%q) ok = false", tc.prop)
			}
			if !tc.get(got) {
				t.Errorf("GetProperty(%q) = %+v, did not match expectation", tc.prop, got)
			}
		})
	}
}

// TestScreenObjectProperties_Defaults covers the fallback branches in
// GetProperty for "align" (defaults to "fill" when unset) and "stretch"
// (defaults to 1 when unset).
func TestScreenObjectProperties_Defaults(t *testing.T) {
	so := &ScreenObject{}

	if v, ok := so.GetProperty("align"); !ok || v.S != "fill" {
		t.Errorf("GetProperty(align) on zero value = %+v, ok=%v, want fill", v, ok)
	}
	if v, ok := so.GetProperty("stretch"); !ok || v.I != 1 {
		t.Errorf("GetProperty(stretch) on zero value = %+v, ok=%v, want 1", v, ok)
	}

	if err := so.SetProperty("align", PropValString("")); err != nil {
		t.Fatalf("SetProperty(align, \"\") unexpected error: %v", err)
	}
	if v, ok := so.GetProperty("align"); !ok || v.S != "fill" {
		t.Errorf("GetProperty(align) after setting empty = %+v, ok=%v, want fill", v, ok)
	}

	if err := so.SetProperty("stretch", PropValInt(0)); err != nil {
		t.Fatalf("SetProperty(stretch, 0) unexpected error: %v", err)
	}
	if v, ok := so.GetProperty("stretch"); !ok || v.I != 1 {
		t.Errorf("GetProperty(stretch) after setting 0 = %+v, ok=%v, want 1", v, ok)
	}
}
