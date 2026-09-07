package vreactive

import "testing"

type mockWidget struct {
	disabled bool
	visible  bool
}

func (m *mockWidget) SetDisabled(d bool) { m.disabled = d }
func (m *mockWidget) SetVisible(v bool)  { m.visible = v }

func TestEffect(t *testing.T) {
	p1 := NewProperty(1)
	p2 := NewProperty(10)

	sum := 0
	runs := 0

	unsub := Effect(func() {
		sum = p1.Get() + p2.Get()
		runs++
	}, p1, p2)

	if runs != 1 || sum != 11 {
		t.Errorf("initial effect failed: runs=%d sum=%d", runs, sum)
	}

	p1.Set(2)
	if runs != 2 || sum != 12 {
		t.Errorf("p1 change effect failed: runs=%d sum=%d", runs, sum)
	}

	p2.Set(20)
	if runs != 3 || sum != 22 {
		t.Errorf("p2 change effect failed: runs=%d sum=%d", runs, sum)
	}

	unsub()
	p1.Set(100)
	if runs != 3 {
		t.Errorf("effect should not run after unsubscribe, runs=%d", runs)
	}
}

func TestTwoWayBind(t *testing.T) {
	prop := NewProperty("initial")
	extVal := ""
	var extListener func(string)

	cleanup := TwoWayBind(
		prop,
		func() string { return extVal },
		func(v string) { extVal = v },
		func(onChange func(string)) func() {
			extListener = onChange
			return func() { extListener = nil }
		},
	)

	if extVal != "initial" {
		t.Errorf("initial two-way sync failed, got %q", extVal)
	}

	// Prop changes -> extVal updates
	prop.Set("from_prop")
	if extVal != "from_prop" {
		t.Errorf("prop -> ext sync failed, got %q", extVal)
	}

	// Ext changes -> prop updates
	extVal = "from_ext"
	extListener("from_ext")
	if prop.Get() != "from_ext" {
		t.Errorf("ext -> prop sync failed, got %q", prop.Get())
	}

	cleanup()
	prop.Set("detached")
	if extVal == "detached" {
		t.Error("sync should not happen after cleanup")
	}
}

func TestBindEnabledAndVisible(t *testing.T) {
	w := &mockWidget{}
	active := NewProperty(true)

	unsub1 := BindEnabled(active, w)
	unsub2 := BindVisible(active, w)

	if w.disabled || !w.visible {
		t.Errorf("expected enabled & visible: disabled=%v visible=%v", w.disabled, w.visible)
	}

	active.Set(false)
	if !w.disabled || w.visible {
		t.Errorf("expected disabled & hidden: disabled=%v visible=%v", w.disabled, w.visible)
	}

	unsub1()
	unsub2()
}
