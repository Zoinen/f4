package vtui

import (
	"testing"

	"github.com/unxed/vtui/vreactive"
)

func TestEditBindTextFollowsAndUpdatesTheProperty(t *testing.T) {
	SetDefaultPalette()
	prop := vreactive.NewProperty("start")
	e := NewEdit(0, 0, 20, "")
	var previousSaw []string
	e.OnTextChange = func(s string) { previousSaw = append(previousSaw, s) }

	unbind := e.BindText(prop)
	if e.GetText() != "start" {
		t.Fatalf("the property's value is applied at once, got %q", e.GetText())
	}
	prop.Set("from property")
	if e.GetText() != "from property" {
		t.Fatalf("edit after a property change = %q", e.GetText())
	}
	// Typing updates the property and still reaches the handler that was there.
	e.OnTextChange("typed")
	if prop.Get() != "typed" {
		t.Fatalf("property after typing = %q", prop.Get())
	}
	if len(previousSaw) == 0 || previousSaw[len(previousSaw)-1] != "typed" {
		t.Fatalf("the previous OnTextChange handler was not called: %v", previousSaw)
	}

	unbind()
	prop.Set("after unbind")
	if e.GetText() == "after unbind" {
		t.Fatal("the edit still follows the property after unbind")
	}
	before := len(previousSaw)
	e.OnTextChange("plain")
	if len(previousSaw) != before+1 || prop.Get() != "after unbind" {
		t.Fatal("unbind must restore the previous handler and stop feeding the property")
	}
}

func TestBindWithNothingIsHarmless(t *testing.T) {
	var e *Edit
	e.BindText(vreactive.NewProperty(""))()
	NewEdit(0, 0, 5, "x").BindText(nil)()
	var cb *Checkbox
	cb.BindChecked(vreactive.NewProperty(false))()
	NewCheckbox(0, 0, "c", false).BindChecked(nil)()
	var tx *Text
	tx.BindText(vreactive.NewProperty(""))()
	NewText(0, 0, "x", 0).BindText(nil)()
}

func TestCheckboxBindCheckedFollowsAndUpdatesTheProperty(t *testing.T) {
	SetDefaultPalette()
	prop := vreactive.NewProperty(true)
	cb := NewCheckbox(0, 0, "Option", true)
	cb.State = 2 // the undefined state of a three-state box
	var previous []int
	cb.OnChange = func(state int) { previous = append(previous, state) }

	unbind := cb.BindChecked(prop)
	if cb.State != 1 {
		t.Fatalf("the property (true) is applied at once, State = %d", cb.State)
	}
	prop.Set(false)
	if cb.State != 0 {
		t.Fatalf("State after the property went false = %d", cb.State)
	}
	// Clicking updates the property; the undefined state reads as false.
	cb.OnChange(1)
	if !prop.Get() {
		t.Fatal("checking the box did not update the property")
	}
	cb.OnChange(2)
	if prop.Get() {
		t.Fatal("the undefined state must read as false")
	}
	if len(previous) == 0 {
		t.Fatal("the previous OnChange handler was not called")
	}

	unbind()
	prop.Set(true)
	if cb.State == 1 {
		t.Fatal("the checkbox still follows the property after unbind")
	}
}

func TestTextBindTextFollowsTheProperty(t *testing.T) {
	SetDefaultPalette()
	prop := vreactive.NewProperty("one")
	tx := NewText(0, 0, "", 0)
	unbind := tx.BindText(prop)
	if tx.cleanText != "one" {
		t.Fatalf("text = %q, want the property's value at once", tx.cleanText)
	}
	prop.Set("two")
	if tx.cleanText != "two" {
		t.Fatalf("text after a change = %q", tx.cleanText)
	}
	unbind()
	prop.Set("three")
	if tx.cleanText == "three" {
		t.Fatal("the text still follows the property after unbind")
	}
}
