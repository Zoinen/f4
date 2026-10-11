package vtui

import (
	"strings"
	"testing"
)

func TestSemanticInt(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int
	}{
		{"nil", nil, 0},
		{"string", "3", 0},
		{"bool", true, 0},
		{"int", 7, 7},
		{"negative int", -2, -2},
		{"int8", int8(-8), -8},
		{"int16", int16(16), 16},
		{"int32", int32(32), 32},
		{"int64", int64(64), 64},
		{"uint", uint(1), 1},
		{"uint8", uint8(8), 8},
		{"uint16", uint16(16), 16},
		{"uint32", uint32(32), 32},
		{"uint64", uint64(64), 64},
		{"float32 truncates", float32(2.9), 2},
		// JSON numbers arrive as float64.
		{"float64 truncates", float64(3.99), 3},
		{"negative float64", float64(-1), -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := semanticInt(tt.in); got != tt.want {
				t.Fatalf("semanticInt(%#v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestSemanticStringAndHotkey(t *testing.T) {
	strs := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"", ""},
		{"close", "close"},
		{42, ""},
		{[]byte("x"), ""},
	}
	for _, tt := range strs {
		if got := semanticString(tt.in); got != tt.want {
			t.Errorf("semanticString(%#v) = %q, want %q", tt.in, got, tt.want)
		}
	}

	runes := []struct {
		in   rune
		want string
	}{
		{0, ""},
		{'o', "o"},
		{'ж', "ж"},
	}
	for _, tt := range runes {
		if got := stringOrEmpty(tt.in); got != tt.want {
			t.Errorf("stringOrEmpty(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSemanticID(t *testing.T) {
	if got := SemanticID(nil); got != "" {
		t.Fatalf("SemanticID(nil) = %q, want empty", got)
	}

	named := NewButton(0, 0, "Ok")
	named.SetId("okButton")
	if got := SemanticID(named); got != "id:okButton" {
		t.Fatalf("SemanticID(named) = %q, want id:okButton", got)
	}

	anon := NewButton(0, 0, "Ok")
	got := SemanticID(anon)
	if !strings.HasPrefix(got, "*vtui.Button:0x") {
		t.Fatalf("SemanticID(anonymous button) = %q, want *vtui.Button:0x...", got)
	}
	if other := SemanticID(NewButton(0, 0, "Ok")); other == got {
		t.Fatalf("two anonymous buttons share the semantic id %q", got)
	}
	if again := SemanticID(anon); again != got {
		t.Fatalf("SemanticID is not stable: %q then %q", got, again)
	}

	// A value that is not a UIElement still gets a type-and-pointer id.
	s := &struct{ n int }{}
	if got := SemanticID(s); !strings.HasPrefix(got, "*struct") {
		t.Fatalf("SemanticID(non-element) = %q, want a *struct... id", got)
	}
}

func TestSemanticNode_Widgets(t *testing.T) {
	SetDefaultPalette()
	ctx := &SemanticContext{Width: 80, Height: 25}

	tests := []struct {
		name  string
		node  func() map[string]any
		check map[string]any
	}{
		{
			name: "button",
			node: func() map[string]any {
				b := NewButton(2, 3, "&Save")
				b.IsDefault = true
				return b.SemanticNode(ctx)
			},
			check: map[string]any{"kind": "button", "x": 2, "y": 3, "h": 1, "text": "Save", "hotkey": "s", "default": true, "disabled": false},
		},
		{
			name: "three-state checkbox",
			node: func() map[string]any {
				cb := NewCheckbox(1, 1, "&Mixed", true)
				cb.State = 2
				return cb.SemanticNode(ctx)
			},
			check: map[string]any{"kind": "checkbox", "text": "Mixed", "hotkey": "m", "state": 2, "threeState": true, "w": 4 + 5},
		},
		{
			name: "check group",
			node: func() map[string]any {
				cg := NewCheckGroup(0, 0, 2, []string{"A", "B", "C"})
				cg.States[1] = true
				return cg.SemanticNode(ctx)
			},
			check: map[string]any{"kind": "checkGroup", "focusIndex": 0, "columns": 2, "h": 2},
		},
		{
			name: "radio group",
			node: func() map[string]any {
				rg := NewRadioGroup(0, 0, 0, []string{"One", "Two"})
				rg.Selected = 1
				return rg.SemanticNode(ctx)
			},
			// cols < 1 is clamped to a single column: two rows.
			check: map[string]any{"kind": "radioGroup", "selected": 1, "columns": 1, "h": 2},
		},
		{
			name: "combo box",
			node: func() map[string]any {
				cb := NewComboBox(5, 6, 20, []string{"&red", "green"})
				cb.DropdownOnly = true
				cb.Edit.SetText("green")
				return cb.SemanticNode(ctx)
			},
			check: map[string]any{"kind": "comboBox", "x": 5, "y": 6, "w": 20, "h": 1, "text": "green", "dropdownOnly": true, "selected": 0},
		},
		{
			name: "password edit",
			node: func() map[string]any {
				e := NewPasswordEdit(0, 0, 10, "")
				e.ShowHistoryButton = true
				e.InsertString("abc")
				return e.SemanticNode(ctx)
			},
			check: map[string]any{"kind": "edit", "w": 10, "text": "abc", "cursor": 3, "left": 0, "password": true, "history": true, "selectionStart": -1},
		},
		{
			name: "progress bar",
			node: func() map[string]any {
				pb := NewProgressBar(0, 4, 30)
				pb.SetPercent(40)
				return pb.SemanticNode(ctx)
			},
			check: map[string]any{"kind": "progressBar", "y": 4, "w": 30, "percent": 40},
		},
		{
			name: "scroll bar",
			node: func() map[string]any {
				sb := NewScrollBar(9, 0, 5)
				sb.SetParams(3, 0, 10)
				return sb.SemanticNode(ctx)
			},
			check: map[string]any{"kind": "scrollBar", "x": 9, "w": 1, "h": 5, "value": 3, "min": 0, "max": 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := tt.node()
			if id, _ := node["id"].(string); id == "" {
				t.Fatalf("node has no id: %#v", node)
			}
			for k, want := range tt.check {
				if got := node[k]; got != want {
					t.Errorf("node[%q] = %#v, want %#v (node %#v)", k, got, want, node)
				}
			}
		})
	}
}

func TestSemanticNode_GroupItemsAndCombo(t *testing.T) {
	ctx := &SemanticContext{}

	cg := NewCheckGroup(0, 0, 1, []string{"x", "y"})
	cg.States[1] = true
	node := cg.SemanticNode(ctx)
	if items := node["items"].([]string); len(items) != 2 || items[1] != "y" {
		t.Fatalf("check group items = %#v", node["items"])
	}
	if states := node["states"].([]bool); !states[1] || states[0] {
		t.Fatalf("check group states = %#v", node["states"])
	}

	cb := NewComboBox(0, 0, 12, []string{"&red", "a&&b"})
	cb.Menu.Items[1].Shortcut = "F2"
	cb.Menu.Items[1].Command = 99
	items := cb.SemanticNode(ctx)["items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("combo items = %#v", items)
	}
	want := []map[string]any{
		{"index": 0, "text": "red", "rawText": "&red", "hotkey": "r", "shortcut": "", "command": 0, "separator": false},
		{"index": 1, "text": "a&b", "rawText": "a&&b", "hotkey": "", "shortcut": "F2", "command": 99, "separator": false},
	}
	for i := range want {
		for k, v := range want[i] {
			if items[i][k] != v {
				t.Errorf("combo item %d [%q] = %#v, want %#v", i, k, items[i][k], v)
			}
		}
	}
}

func TestSemanticAction_Widgets(t *testing.T) {
	SetDefaultPalette()

	type widget interface {
		UIElement
		SemanticActionHandler
	}
	tests := []struct {
		name   string
		build  func() (widget, func(t *testing.T))
		action map[string]any
		want   bool
	}{
		{
			name: "button focus",
			build: func() (widget, func(*testing.T)) {
				b := NewButton(0, 0, "Ok")
				return b, func(t *testing.T) {
					if !b.IsFocused() {
						t.Error("button not focused")
					}
				}
			},
			action: map[string]any{"action": "control.focus"},
			want:   true,
		},
		{
			name: "button unknown action",
			build: func() (widget, func(*testing.T)) {
				b := NewButton(0, 0, "Ok")
				clicked := false
				b.OnClick = func() { clicked = true }
				return b, func(t *testing.T) {
					if clicked {
						t.Error("unknown action clicked the button")
					}
				}
			},
			action: map[string]any{"action": "toggle"},
			want:   false,
		},
		{
			name: "button control.activate",
			build: func() (widget, func(*testing.T)) {
				b := NewButton(0, 0, "Ok")
				clicked := false
				b.OnClick = func() { clicked = true }
				return b, func(t *testing.T) {
					if !clicked {
						t.Error("button not clicked")
					}
				}
			},
			action: map[string]any{"action": "control.activate"},
			want:   true,
		},
		{
			name: "three-state checkbox cycles",
			build: func() (widget, func(*testing.T)) {
				cb := NewCheckbox(0, 0, "x", true)
				cb.State = 2
				got := -1
				cb.OnChange = func(s int) { got = s }
				return cb, func(t *testing.T) {
					if cb.State != 0 || got != 0 {
						t.Errorf("state %d, OnChange %d; want 0, 0", cb.State, got)
					}
				}
			},
			action: map[string]any{"action": "control.toggle"},
			want:   true,
		},
		{
			name: "checkbox focus",
			build: func() (widget, func(*testing.T)) {
				cb := NewCheckbox(0, 0, "x", false)
				return cb, func(t *testing.T) {
					if !cb.IsFocused() || cb.State != 0 {
						t.Errorf("focused %v, state %d", cb.IsFocused(), cb.State)
					}
				}
			},
			action: map[string]any{"action": "focus"},
			want:   true,
		},
		{
			name: "checkbox unknown action",
			build: func() (widget, func(*testing.T)) {
				cb := NewCheckbox(0, 0, "x", false)
				return cb, func(t *testing.T) {
					if cb.State != 0 {
						t.Error("state changed")
					}
				}
			},
			action: map[string]any{"action": "select", "index": 0},
			want:   false,
		},
		{
			name: "check group select flips the item",
			build: func() (widget, func(*testing.T)) {
				cg := NewCheckGroup(0, 0, 1, []string{"a", "b", "c"})
				cg.States[2] = true
				return cg, func(t *testing.T) {
					if cg.States[2] || cg.focusIdx != 2 {
						t.Errorf("states %v focus %d", cg.States, cg.focusIdx)
					}
				}
			},
			action: map[string]any{"action": "control.select", "index": float64(2)},
			want:   true,
		},
		{
			name: "check group select out of range",
			build: func() (widget, func(*testing.T)) {
				cg := NewCheckGroup(0, 0, 1, []string{"a"})
				return cg, func(t *testing.T) {
					if cg.States[0] {
						t.Error("state changed")
					}
				}
			},
			action: map[string]any{"action": "select", "index": 5},
			want:   false,
		},
		{
			name: "check group negative index",
			build: func() (widget, func(*testing.T)) {
				cg := NewCheckGroup(0, 0, 1, []string{"a"})
				return cg, func(*testing.T) {}
			},
			action: map[string]any{"action": "select", "index": -1},
			want:   false,
		},
		{
			name: "check group focus",
			build: func() (widget, func(*testing.T)) {
				cg := NewCheckGroup(0, 0, 1, []string{"a"})
				return cg, func(t *testing.T) {
					if !cg.IsFocused() {
						t.Error("not focused")
					}
				}
			},
			action: map[string]any{"action": "focus"},
			want:   true,
		},
		{
			name: "radio group select fires OnChange",
			build: func() (widget, func(*testing.T)) {
				rg := NewRadioGroup(0, 0, 1, []string{"a", "b"})
				got := -1
				rg.OnChange = func(i int) { got = i }
				return rg, func(t *testing.T) {
					if rg.Selected != 1 || got != 1 || rg.focusIdx != 1 {
						t.Errorf("selected %d OnChange %d focus %d", rg.Selected, got, rg.focusIdx)
					}
				}
			},
			action: map[string]any{"action": "select", "index": 1},
			want:   true,
		},
		{
			name: "radio group reselect is silent",
			build: func() (widget, func(*testing.T)) {
				rg := NewRadioGroup(0, 0, 1, []string{"a", "b"})
				rg.Selected = 1
				calls := 0
				rg.OnChange = func(int) { calls++ }
				return rg, func(t *testing.T) {
					if calls != 0 || rg.Selected != 1 {
						t.Errorf("OnChange calls %d, selected %d", calls, rg.Selected)
					}
				}
			},
			action: map[string]any{"action": "control.select", "index": 1},
			want:   true,
		},
		{
			name: "radio group out of range",
			build: func() (widget, func(*testing.T)) {
				rg := NewRadioGroup(0, 0, 1, []string{"a", "b"})
				return rg, func(t *testing.T) {
					if rg.Selected != 0 {
						t.Error("selection changed")
					}
				}
			},
			action: map[string]any{"action": "select", "index": 2},
			want:   false,
		},
		{
			name: "radio group focus",
			build: func() (widget, func(*testing.T)) {
				rg := NewRadioGroup(0, 0, 1, []string{"a"})
				return rg, func(t *testing.T) {
					if !rg.IsFocused() {
						t.Error("not focused")
					}
				}
			},
			action: map[string]any{"action": "control.focus"},
			want:   true,
		},
		{
			name: "combo select sets text and fires OnAction",
			build: func() (widget, func(*testing.T)) {
				cb := NewComboBox(0, 0, 10, []string{"red", "green"})
				fired := -1
				cb.Menu.OnAction = func(i int) { fired = i }
				return cb, func(t *testing.T) {
					if cb.Edit.GetText() != "green" || cb.Menu.SelectPos != 1 || fired != 1 {
						t.Errorf("text %q pos %d fired %d", cb.Edit.GetText(), cb.Menu.SelectPos, fired)
					}
				}
			},
			action: map[string]any{"action": "select", "index": 1},
			want:   true,
		},
		{
			name: "combo select out of range",
			build: func() (widget, func(*testing.T)) {
				cb := NewComboBox(0, 0, 10, []string{"red"})
				return cb, func(t *testing.T) {
					if cb.Edit.GetText() != "" {
						t.Errorf("text %q", cb.Edit.GetText())
					}
				}
			},
			action: map[string]any{"action": "control.select", "index": 1},
			want:   false,
		},
		{
			name: "combo focus",
			build: func() (widget, func(*testing.T)) {
				cb := NewComboBox(0, 0, 10, nil)
				return cb, func(t *testing.T) {
					if !cb.IsFocused() {
						t.Error("not focused")
					}
				}
			},
			action: map[string]any{"action": "focus"},
			want:   true,
		},
		{
			name: "edit set_text calls OnTextChange",
			build: func() (widget, func(*testing.T)) {
				e := NewEdit(0, 0, 10, "old")
				got := ""
				e.OnTextChange = func(s string) { got = s }
				return e, func(t *testing.T) {
					if e.GetText() != "new" || got != "new" {
						t.Errorf("text %q, OnTextChange %q", e.GetText(), got)
					}
				}
			},
			action: map[string]any{"action": "control.setText", "text": "new"},
			want:   true,
		},
		{
			name: "edit insert_text at the cursor",
			build: func() (widget, func(*testing.T)) {
				e := NewEdit(0, 0, 10, "")
				e.InsertString("ac")
				e.curPos = 1
				return e, func(t *testing.T) {
					if e.GetText() != "abc" {
						t.Errorf("text %q, want abc", e.GetText())
					}
				}
			},
			action: map[string]any{"action": "insert_text", "text": "b"},
			want:   true,
		},
		{
			name: "edit set_text with non-string text clears",
			build: func() (widget, func(*testing.T)) {
				e := NewEdit(0, 0, 10, "old")
				return e, func(t *testing.T) {
					if e.GetText() != "" {
						t.Errorf("text %q, want empty", e.GetText())
					}
				}
			},
			action: map[string]any{"action": "set_text", "text": 5},
			want:   true,
		},
		{
			name: "edit focus",
			build: func() (widget, func(*testing.T)) {
				e := NewEdit(0, 0, 10, "")
				return e, func(t *testing.T) {
					if !e.IsFocused() {
						t.Error("not focused")
					}
				}
			},
			action: map[string]any{"action": "control.focus"},
			want:   true,
		},
		{
			name: "edit unknown action",
			build: func() (widget, func(*testing.T)) {
				e := NewEdit(0, 0, 10, "keep")
				return e, func(t *testing.T) {
					if e.GetText() != "keep" {
						t.Errorf("text %q", e.GetText())
					}
				}
			},
			action: map[string]any{"action": "toggle"},
			want:   false,
		},
		{
			name: "scroll bar clamps and notifies",
			build: func() (widget, func(*testing.T)) {
				sb := NewScrollBar(0, 0, 5)
				sb.SetParams(2, 0, 10)
				got := -1
				sb.OnScroll = func(v int) { got = v }
				return sb, func(t *testing.T) {
					if sb.Value != 10 || got != 10 {
						t.Errorf("value %d OnScroll %d, want 10", sb.Value, got)
					}
				}
			},
			action: map[string]any{"action": "control.scroll", "value": 99},
			want:   true,
		},
		{
			name: "scroll bar clamps below min",
			build: func() (widget, func(*testing.T)) {
				sb := NewScrollBar(0, 0, 5)
				sb.SetParams(4, 1, 10)
				return sb, func(t *testing.T) {
					if sb.Value != 1 {
						t.Errorf("value %d, want 1", sb.Value)
					}
				}
			},
			action: map[string]any{"action": "scroll", "value": -3},
			want:   true,
		},
		{
			name: "scroll bar unknown action",
			build: func() (widget, func(*testing.T)) {
				sb := NewScrollBar(0, 0, 5)
				return sb, func(*testing.T) {}
			},
			action: map[string]any{"action": "focus"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, check := tt.build()
			if got := w.HandleSemanticAction(tt.action); got != tt.want {
				t.Fatalf("HandleSemanticAction(%v) = %v, want %v", tt.action, got, tt.want)
			}
			check(t)
		})
	}
}

// A disabled control ignores the keyboard and the mouse; the semantic path
// (external GUI, automation) must not be a way around that.
func TestSemanticAction_DisabledControlsRefuse(t *testing.T) {
	SetDefaultPalette()

	type widget interface {
		UIElement
		SemanticActionHandler
	}
	btnClicked := false
	btn := NewButton(0, 0, "Ok")
	btn.OnClick = func() { btnClicked = true }
	chk := NewCheckbox(0, 0, "x", false)
	cg := NewCheckGroup(0, 0, 1, []string{"a", "b"})
	rg := NewRadioGroup(0, 0, 1, []string{"a", "b"})
	combo := NewComboBox(0, 0, 10, []string{"red", "green"})
	edit := NewEdit(0, 0, 10, "keep")

	tests := []struct {
		name    string
		w       widget
		actions []string
	}{
		{"button", btn, []string{"activate", "control.activate", "focus"}},
		{"checkbox", chk, []string{"toggle", "control.toggle", "focus"}},
		{"check group", cg, []string{"select", "control.select", "focus"}},
		{"radio group", rg, []string{"select", "control.select", "focus"}},
		{"combo box", combo, []string{"select", "control.select", "focus"}},
		{"edit", edit, []string{"set_text", "control.setText", "insert_text", "control.insertText", "focus"}},
	}
	for _, tt := range tests {
		tt.w.SetDisabled(true)
		for _, a := range tt.actions {
			action := map[string]any{"action": a, "index": 1, "text": "changed"}
			if tt.w.HandleSemanticAction(action) {
				t.Errorf("%s: disabled control handled %q", tt.name, a)
			}
		}
		if tt.w.IsFocused() {
			t.Errorf("%s: disabled control took focus", tt.name)
		}
	}

	if btnClicked {
		t.Error("disabled button was clicked")
	}
	if chk.State != 0 {
		t.Errorf("disabled checkbox state = %d, want 0", chk.State)
	}
	if cg.States[1] {
		t.Error("disabled check group item was flipped")
	}
	if rg.Selected != 0 {
		t.Errorf("disabled radio group selected = %d, want 0", rg.Selected)
	}
	if combo.Edit.GetText() != "" || combo.Menu.SelectPos != 0 {
		t.Errorf("disabled combo text %q pos %d", combo.Edit.GetText(), combo.Menu.SelectPos)
	}
	if edit.GetText() != "keep" {
		t.Errorf("disabled edit text = %q, want keep", edit.GetText())
	}

	// Enabled again, the same controls react.
	chk.SetDisabled(false)
	if !chk.HandleSemanticAction(map[string]any{"action": "toggle"}) || chk.State != 1 {
		t.Fatalf("re-enabled checkbox did not toggle: state %d", chk.State)
	}
}

func TestSemantic_GroupRoutingAndChildren(t *testing.T) {
	SetDefaultPalette()
	ctx := &SemanticContext{}

	outer := NewGroup(0, 0, 40, 10)
	inner := NewGroup(1, 1, 20, 5)
	deep := NewCheckbox(2, 2, "deep", false)
	inner.AddItem(deep)
	text := NewText(0, 8, "plain", 0)
	outer.AddItem(inner)
	outer.AddItem(text)

	node := outer.SemanticNode(ctx)
	if node["kind"] != "group" || node["w"] != 40 || node["h"] != 10 {
		t.Fatalf("group node = %#v", node)
	}
	children := node["children"].([]map[string]any)
	if len(children) != 2 {
		t.Fatalf("group children = %#v", children)
	}
	if children[0]["kind"] != "group" {
		t.Errorf("inner group exported as %#v", children[0]["kind"])
	}
	// Text has no semantic node of its own: it falls back to a plain widget.
	if children[1]["kind"] != "widget" || children[1]["y"] != 8 || children[1]["id"] != SemanticID(text) {
		t.Errorf("fallback child = %#v", children[1])
	}
	grand := children[0]["children"].([]map[string]any)
	if len(grand) != 1 || grand[0]["kind"] != "checkbox" {
		t.Fatalf("nested children = %#v", grand)
	}

	// nil children are skipped.
	if got := semanticChildren(ctx, []UIElement{nil, text}); len(got) != 1 {
		t.Fatalf("semanticChildren with nil = %#v", got)
	}

	tests := []struct {
		name   string
		action map[string]any
		want   bool
	}{
		{"nested child through Container", map[string]any{"target": SemanticID(deep), "action": "toggle"}, true},
		{"inner group focus", map[string]any{"target": SemanticID(inner), "action": "control.focus"}, true},
		{"outer group focus", map[string]any{"target": SemanticID(outer), "action": "focus"}, true},
		{"outer group unknown action", map[string]any{"target": SemanticID(outer), "action": "toggle"}, false},
		{"child without a handler", map[string]any{"target": SemanticID(text), "action": "toggle"}, false},
		{"unknown target", map[string]any{"target": "id:nope", "action": "toggle"}, false},
		{"no target", map[string]any{"action": "toggle"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := outer.HandleSemanticAction(tt.action); got != tt.want {
				t.Fatalf("HandleSemanticAction(%v) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
	if deep.State != 1 {
		t.Errorf("nested checkbox state = %d, want 1 after one toggle", deep.State)
	}
	if !outer.IsFocused() {
		t.Error("outer group not focused")
	}

	// handleSemanticChildrenAction on its own: found deep in a subtree,
	// nothing for an unknown id.
	if !handleSemanticChildrenAction(outer.GetChildren(), SemanticID(deep), map[string]any{"action": "toggle"}) {
		t.Error("handleSemanticChildrenAction did not reach the nested checkbox")
	}
	if handleSemanticChildrenAction(outer.GetChildren(), "id:nope", map[string]any{"action": "toggle"}) {
		t.Error("handleSemanticChildrenAction handled an unknown target")
	}
}

func TestSemantic_WindowNodeAndClose(t *testing.T) {
	SetDefaultPalette()
	ctx := &SemanticContext{}

	win := NewWindow(0, 0, 29, 9, "  Plain  ")
	node := win.SemanticNode(ctx)
	if node["kind"] != "window" || node["title"] != "Plain" || node["modal"] != false || node["w"] != 30 || node["h"] != 10 {
		t.Fatalf("window node = %#v", node)
	}

	tests := []struct {
		action string
		target func(w *Window) string
		want   bool
		done   bool
	}{
		{"close", func(w *Window) string { return SemanticID(w) }, true, true},
		{"dialog.close", func(w *Window) string { return SemanticID(w) }, true, true},
		{"window.close", func(w *Window) string { return SemanticID(w) }, true, true},
		{"zoom", func(w *Window) string { return SemanticID(w) }, false, false},
		{"close", func(*Window) string { return "id:other" }, false, false},
	}
	for _, tt := range tests {
		w := NewDialog(0, 0, 19, 5, "D")
		got := w.HandleSemanticAction(map[string]any{"action": tt.action, "target": tt.target(w)})
		if got != tt.want || w.IsDone() != tt.done {
			t.Errorf("%s: handled %v done %v, want %v %v", tt.action, got, w.IsDone(), tt.want, tt.done)
		}
	}
}

func TestSemantic_VMenuActions(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	newMenu := func() (*VMenu, *int) {
		m := NewVMenu("M")
		m.AddItem(MenuItem{Text: "one"})
		m.AddSeparator()
		m.AddItem(MenuItem{Text: "two"})
		confirmed := -1
		m.OnAction = func(i int) { confirmed = i }
		return m, &confirmed
	}

	tests := []struct {
		name      string
		action    string
		index     any
		target    bool
		want      bool
		confirmed int
		done      bool
	}{
		{"activate", "menu.activate", 2, true, true, 2, true},
		{"activate legacy name", "menu_activate", float64(0), true, true, 0, true},
		{"activate separator", "menu.activate", 1, true, false, -1, false},
		{"activate out of range", "menu.activate", 3, true, false, -1, false},
		{"activate negative", "menu.activate", -1, true, false, -1, false},
		{"close", "menu.close", nil, true, true, -1, true},
		{"close legacy name", "close", nil, true, true, -1, true},
		{"wrong target", "menu.close", nil, false, false, -1, false},
		{"unknown action", "toggle", nil, true, false, -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, confirmed := newMenu()
			target := "id:other"
			if tt.target {
				target = SemanticID(m)
			}
			got := m.HandleSemanticAction(map[string]any{"target": target, "action": tt.action, "index": tt.index})
			if got != tt.want {
				t.Fatalf("handled = %v, want %v", got, tt.want)
			}
			if *confirmed != tt.confirmed {
				t.Errorf("confirmed = %d, want %d", *confirmed, tt.confirmed)
			}
			if m.IsDone() != tt.done {
				t.Errorf("done = %v, want %v", m.IsDone(), tt.done)
			}
		})
	}
}

func TestSemanticMenuBar(t *testing.T) {
	if semanticMenuBar(nil) != nil {
		t.Fatal("semanticMenuBar(nil) is not nil")
	}

	mb := NewMenuBar([]string{"&File", "Edit"})
	mb.SetPosition(0, 0, 79, 0)
	mb.Items[0].Command = 12
	mb.Items[0].SubItems = []MenuItem{
		{Text: "&Open", Shortcut: "F3", Command: 7},
		{Separator: true},
	}
	mb.SelectPos = 1
	mb.Active = true

	node := semanticMenuBar(mb)
	for k, want := range map[string]any{"kind": "menuBar", "x": 0, "y": 0, "w": 80, "h": 1, "active": true, "selected": 1} {
		if node[k] != want {
			t.Errorf("menu bar [%q] = %#v, want %#v", k, node[k], want)
		}
	}
	items := node["items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("menu bar items = %#v", items)
	}
	wantItems := []map[string]any{
		// "  File  " is 8 columns; the bar content starts at X1+2.
		{"index": 0, "x": 2, "w": 8, "text": "File", "rawText": "&File", "hotkey": "f", "command": 12, "disabled": false},
		{"index": 1, "x": 10, "w": 8, "text": "Edit", "rawText": "Edit", "hotkey": "", "command": 0},
	}
	for i := range wantItems {
		for k, v := range wantItems[i] {
			if items[i][k] != v {
				t.Errorf("item %d [%q] = %#v, want %#v", i, k, items[i][k], v)
			}
		}
	}
	subs := items[0]["items"].([]map[string]any)
	if len(subs) != 2 {
		t.Fatalf("sub items = %#v", subs)
	}
	if subs[0]["text"] != "Open" || subs[0]["hotkey"] != "o" || subs[0]["shortcut"] != "F3" || subs[0]["command"] != 7 || subs[0]["separator"] != false {
		t.Errorf("first sub item = %#v", subs[0])
	}
	if subs[1]["separator"] != true || subs[1]["index"] != 1 {
		t.Errorf("separator sub item = %#v", subs[1])
	}
	if got := items[1]["items"].([]map[string]any); len(got) != 0 {
		t.Errorf("item without submenu exported %#v", got)
	}
}

func TestSemanticKeyBar(t *testing.T) {
	kb := NewKeyBar()
	kb.SetPosition(0, 24, 79, 24)
	kb.Normal[0] = "Help"
	kb.Shift[0] = "Add"
	kb.Ctrl[1] = "Swap"
	kb.Alt[2] = "Find"

	tests := []struct {
		name             string
		shift, ctrl, alt bool
		modifier         string
		slot             int
		label            string
	}{
		{"normal", false, false, false, "normal", 0, "Help"},
		{"shift", true, false, false, "shift", 0, "Add"},
		{"ctrl", false, true, false, "ctrl", 1, "Swap"},
		{"alt", false, false, true, "alt", 2, "Find"},
		// Shift wins over Ctrl and Alt, Ctrl over Alt.
		{"shift+ctrl+alt", true, true, true, "shift", 0, "Add"},
		{"ctrl+alt", false, true, true, "ctrl", 1, "Swap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb.LatchModifiers(tt.shift, tt.ctrl, tt.alt)
			node := semanticKeyBar(kb)
			if node["kind"] != "keyBar" || node["y"] != 24 || node["w"] != 80 || node["h"] != 1 {
				t.Fatalf("key bar node = %#v", node)
			}
			if node["modifier"] != tt.modifier {
				t.Fatalf("modifier = %#v, want %q", node["modifier"], tt.modifier)
			}
			items := node["items"].([]map[string]any)
			if len(items) != 12 {
				t.Fatalf("key bar has %d items, want 12", len(items))
			}
			if items[tt.slot]["text"] != tt.label || items[tt.slot]["index"] != tt.slot {
				t.Errorf("slot %d = %#v, want %q", tt.slot, items[tt.slot], tt.label)
			}
			if items[11]["key"] != "F12" {
				t.Errorf("last key = %#v, want F12", items[11]["key"])
			}
		})
	}
}

func TestFrameManager_ExportSemanticScene_Parts(t *testing.T) {
	SetDefaultPalette()
	var nilFM *frameManager
	if nilFM.ExportSemanticScene() != nil {
		t.Fatal("nil frameManager exported a scene")
	}
	if (&frameManager{}).ExportSemanticScene() != nil {
		t.Fatal("frameManager without a screen exported a scene")
	}

	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm := &frameManager{}
	fm.Init(scr)
	plain := newMockFrame(0, 0, 10, 3, false)
	fm.Push(plain)
	dlg := NewDialog(5, 5, 24, 10, "Dlg")
	dlg.AddItem(NewButton(1, 1, "Ok"))
	fm.Push(dlg)
	fm.MenuBar = NewMenuBar([]string{"Left"})
	fm.KeyBar = NewKeyBar()
	fm.currentToast = &Toast{Message: "saved"}

	orig := AppSceneAdapter
	t.Cleanup(func() { AppSceneAdapter = orig })
	AppSceneAdapter = nil

	scene := fm.ExportSemanticScene()
	for k, want := range map[string]any{"type": "scene", "version": SemanticSceneVersion, "width": 80, "height": 25, "activeScreen": 0} {
		if scene[k] != want {
			t.Errorf("scene[%q] = %#v, want %#v", k, scene[k], want)
		}
	}
	if _, ok := scene["workspaceCount"]; ok {
		t.Error("a single workspace reported workspaceCount")
	}
	if mb, ok := scene["menuBar"].(map[string]any); !ok || mb["kind"] != "menuBar" {
		t.Errorf("menuBar = %#v", scene["menuBar"])
	}
	if kb, ok := scene["keyBar"].(map[string]any); !ok || kb["kind"] != "keyBar" {
		t.Errorf("keyBar = %#v", scene["keyBar"])
	}
	if toast, ok := scene["toast"].(map[string]any); !ok || toast["message"] != "saved" {
		t.Errorf("toast = %#v", scene["toast"])
	}
	frames, ok := scene["frames"].([]map[string]any)
	if !ok || len(frames) != 2 {
		t.Fatalf("frames = %#v", scene["frames"])
	}
	// A frame with no semantic node of its own is exported as a fallback.
	if frames[0]["kind"] != "fallback" || frames[0]["fallback"] != true || frames[0]["w"] != 10 || frames[0]["h"] != 3 {
		t.Errorf("fallback frame = %#v", frames[0])
	}
	if reason, _ := frames[0]["reason"].(string); !strings.Contains(reason, "mockFrame") {
		t.Errorf("fallback reason = %q", reason)
	}
	if frames[1]["kind"] != "dialog" || frames[1]["title"] != "Dlg" {
		t.Errorf("dialog frame = %#v", frames[1])
	}
	screens := scene["screens"].([]map[string]any)
	if len(screens) != 1 || screens[0]["active"] != true {
		t.Errorf("screens = %#v", screens)
	}
	if semanticFrame(nil, nil) != nil {
		t.Error("semanticFrame(nil) is not nil")
	}

	// The application adapter replaces the scene; a nil answer keeps it.
	AppSceneAdapter = func(ctx *SemanticContext, base map[string]any) map[string]any {
		if ctx.Width != 80 || ctx.Height != 25 {
			t.Errorf("adapter context = %#v", ctx)
		}
		return map[string]any{"type": "custom", "base": base["type"]}
	}
	if got := fm.ExportSemanticScene(); got["type"] != "custom" || got["base"] != "scene" {
		t.Errorf("adapted scene = %#v", got)
	}
	AppSceneAdapter = func(*SemanticContext, map[string]any) map[string]any { return nil }
	if got := fm.ExportSemanticScene(); got["type"] != "scene" {
		t.Errorf("scene with a nil adapter answer = %#v", got)
	}
}

func TestFrameManager_HandleSemanticAction_Routing(t *testing.T) {
	SetDefaultPalette()
	var nilFM *frameManager
	if nilFM.HandleSemanticAction(map[string]any{"action": "x"}) {
		t.Fatal("nil frameManager handled an action")
	}

	newFM := func() (*frameManager, *Button, *bool, *int) {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(80, 25)
		fm := &frameManager{}
		fm.Init(scr)
		lastCmd := 0
		base := newMockFrame(0, 0, 80, 25, false)
		base.onHandleCommand = func(cmd int, args any) bool {
			lastCmd = cmd
			return cmd == 4242
		}
		fm.Push(base)
		dlg := NewDialog(5, 5, 24, 10, "Dlg")
		btn := NewButton(1, 1, "Ok")
		clicked := false
		btn.OnClick = func() { clicked = true }
		dlg.AddItem(btn)
		fm.Push(dlg)
		fm.MenuBar = NewMenuBar([]string{"Left", "Right"})
		return fm, btn, &clicked, &lastCmd
	}

	if fm, _, _, _ := newFM(); fm.HandleSemanticAction(nil) {
		t.Fatal("nil action handled")
	}

	t.Run("command kind", func(t *testing.T) {
		fm, _, _, lastCmd := newFM()
		if !fm.HandleSemanticAction(map[string]any{"kind": "command", "command": float64(4242)}) || *lastCmd != 4242 {
			t.Fatalf("command not delivered: last %d", *lastCmd)
		}
		if fm.HandleSemanticAction(map[string]any{"kind": "command", "command": 4343}) {
			t.Fatal("unhandled command reported as handled")
		}
	})

	t.Run("routed to a dialog control", func(t *testing.T) {
		fm, btn, clicked, _ := newFM()
		if !fm.HandleSemanticAction(map[string]any{"target": SemanticID(btn), "action": "activate"}) || !*clicked {
			t.Fatal("button in the top dialog was not activated")
		}
	})

	t.Run("untargeted action nobody takes", func(t *testing.T) {
		fm, _, clicked, _ := newFM()
		if fm.HandleSemanticAction(map[string]any{"action": "activate"}) || *clicked {
			t.Fatal("untargeted activate was handled")
		}
	})

	t.Run("menu bar activate", func(t *testing.T) {
		tests := []struct {
			action string
			index  any
			want   bool
		}{
			{"menuBar.activate", 1, true},
			{"menu_bar_activate", float64(0), true},
			{"menuBar.activate", 2, false},
			{"menuBar.activate", -1, false},
		}
		for _, tt := range tests {
			fm, _, _, _ := newFM()
			got := fm.HandleSemanticAction(map[string]any{"action": tt.action, "index": tt.index})
			if got != tt.want {
				t.Errorf("%s %v: handled %v, want %v", tt.action, tt.index, got, tt.want)
			}
			if tt.want {
				if !fm.MenuBar.Active || fm.MenuBar.SelectPos != semanticInt(tt.index) {
					t.Errorf("%s %v: menu bar active %v pos %d", tt.action, tt.index, fm.MenuBar.Active, fm.MenuBar.SelectPos)
				}
			} else if fm.MenuBar.Active {
				t.Errorf("%s %v: menu bar activated", tt.action, tt.index)
			}
		}
	})

	t.Run("workspace actions with a bad index", func(t *testing.T) {
		fm, _, _, _ := newFM()
		for _, action := range []map[string]any{
			{"action": "workspace.activate", "index": 3},
			{"action": "tab.activate", "target": "workspace-tab-99"},
			{"action": "workspace.close", "index": -1},
			{"action": "close", "target": "workspace-tab-99"},
		} {
			if fm.HandleSemanticAction(action) {
				t.Errorf("%v handled", action)
			}
		}
		if len(fm.Screens) != 1 {
			t.Fatalf("workspaces = %d, want 1", len(fm.Screens))
		}
	})

	t.Run("workspace activate by index", func(t *testing.T) {
		fm, _, _, _ := newFM()
		fm.AddScreen(newMockFrame(0, 0, 10, 3, false))
		if fm.ActiveIdx != 1 {
			t.Fatalf("active workspace = %d, want 1", fm.ActiveIdx)
		}
		if !fm.HandleSemanticAction(map[string]any{"action": "tab.activate", "index": float64(0)}) || fm.ActiveIdx != 0 {
			t.Fatalf("tab.activate by index left workspace %d active", fm.ActiveIdx)
		}
		scene := fm.ExportSemanticScene()
		if scene["workspaceCount"] != 2 {
			t.Errorf("workspaceCount = %#v, want 2", scene["workspaceCount"])
		}
	})
}
