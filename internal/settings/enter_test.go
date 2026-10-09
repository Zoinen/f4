package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// enterTestCenter is a page like Terminal & environment: a checkbox above a
// record collection whose Add button creates an unnamed record that the
// provider refuses, as envman refuses a profile without a name.
func enterTestCenter(t *testing.T) (*settingsCenter, *f4settings.Draft, *int) {
	t.Helper()
	flag := f4settings.Scalar("flag", "terminal", "Group", "Flag", "A checkbox.", f4settings.Boolean)
	col := f4settings.Collection{ID: "profiles", Category: "terminal", Group: "Profiles", Label: f4settings.Text{English: "Profiles"}, NameField: "name",
		Fields: []f4settings.Field{f4settings.Scalar("name", "terminal", "Profiles", "Name", "Profile name.", f4settings.String)}}
	d := f4settings.NewDraft(map[string]string{"flag": "false"}, map[string][]f4settings.Record{"profiles": {}})
	commits := 0
	d.ValidateFunc = func(d *f4settings.Draft) map[string]error {
		for _, r := range d.Records["profiles"] {
			if r.Values["name"] == "" {
				return map[string]error{"profiles": errors.New("profile name is empty")}
			}
		}
		return nil
	}
	d.CommitFunc = func(context.Context, *f4settings.Draft) f4settings.Result {
		commits++
		return f4settings.Result{Applied: d.Changed()}
	}
	t.Cleanup(d.Close)
	c := newSettingsCenter([]*settingsSession{{catalog: f4settings.Catalog{ID: "test", Categories: Categories, Fields: []f4settings.Field{flag}, Collections: []f4settings.Collection{col}}, draft: d}})
	c.ResizeConsole(150, 40)
	c.SetPosition(0, 0, 149, 39)
	c.selectCategory("terminal")
	return c, d, &commits
}

func enterTestFocus(t *testing.T, c *settingsCenter, match func(*settingsRow) vtui.UIElement) {
	t.Helper()
	for _, r := range c.page.rows {
		if el := match(r); el != nil {
			c.SetFocusedItem(c.page)
			c.page.SetFocusedItem(r.control)
			if group, ok := r.control.(interface{ SetFocusedItem(vtui.UIElement) }); ok && el != r.control {
				group.SetFocusedItem(el)
			}
			return
		}
	}
	t.Fatal("control not found on the page")
}

func enterKey(c *settingsCenter) {
	c.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, Char: '\r'})
}

// Enter on a control that has no use for it applied, instead of pressing the
// first button further down the page: the reporter of f4 #1154 pressed Enter
// on the first checkbox of Terminal & environment, got an unnamed
// environment profile from its Add button, and Apply then failed.
func TestSettingsEnterOnCheckboxAppliesWithoutPressingPageButtons(t *testing.T) {
	c, d, commits := enterTestCenter(t)
	enterTestFocus(t, c, func(r *settingsRow) vtui.UIElement {
		if r.field.ID == "flag" {
			return r.control
		}
		return nil
	})
	enterKey(c)
	if n := len(d.Records["profiles"]); n != 0 {
		t.Fatalf("Enter on the checkbox added %d record(s)", n)
	}
	if c.status != Phrase("Settings applied.") {
		t.Fatalf("Enter did not apply: status %q", c.status)
	}
	if *commits != 0 {
		t.Fatalf("nothing changed, yet the provider committed %d time(s)", *commits)
	}
}

// A button that is itself focused still takes Enter.
func TestSettingsEnterOnFocusedAddButtonStillAdds(t *testing.T) {
	c, d, _ := enterTestCenter(t)
	enterTestFocus(t, c, func(r *settingsRow) vtui.UIElement {
		if bar, ok := r.control.(*settingsButtonRow); ok && len(bar.buttons) > 0 {
			return bar.buttons[0]
		}
		return nil
	})
	enterKey(c)
	if n := len(d.Records["profiles"]); n != 1 {
		t.Fatalf("Enter on a focused Add button added %d record(s), want 1", n)
	}
}
