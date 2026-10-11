package dialog

import (
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

func TestParseGotoOffset(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		hexadecimal bool
		want        int64
		wantErr     bool
	}{
		{"empty string", "", false, 0, true},
		{"blank string", "   ", false, 0, true},
		{"plain decimal", "123", false, 123, false},
		{"decimal with surrounding spaces", "  42  ", false, 42, false},
		{"zero decimal", "0", false, 0, false},
		{"0x prefix", "0x1A", false, 26, false},
		{"0X prefix uppercase", "0X1a", false, 26, false},
		{"0x prefix wins even in decimal mode", "0x10", false, 16, false},
		{"$ prefix", "$1A", false, 26, false},
		{"h suffix", "1Ah", false, 26, false},
		{"H suffix uppercase", "1AH", false, 26, false},
		{"d suffix in decimal mode is stripped", "42d", false, 42, false},
		{"d suffix in hex mode stays a hex digit", "1d", true, 0x1d, false},
		{"hexadecimal checkbox sets the default base", "1A", true, 26, false},
		{"bare 0x prefix has nothing left to parse", "0x", false, 0, true},
		{"bare $ prefix has nothing left to parse", "$", false, 0, true},
		{"bare h suffix has nothing left to parse", "h", false, 0, true},
		{"negative decimal is rejected", "-5", false, 0, true},
		{"negative hexadecimal is rejected", "-5", true, 0, true},
		{"non-numeric decimal text", "abc", false, 0, true},
		{"non-numeric hexadecimal text", "zz", true, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGotoOffset(tt.text, tt.hexadecimal)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseGotoOffset(%q, %v) = %d, <nil>; want an error", tt.text, tt.hexadecimal, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGotoOffset(%q, %v) returned unexpected error: %v", tt.text, tt.hexadecimal, err)
			}
			if got != tt.want {
				t.Errorf("ParseGotoOffset(%q, %v) = %d, want %d", tt.text, tt.hexadecimal, got, tt.want)
			}
		})
	}
}

func TestGotoText(t *testing.T) {
	if got, want := GotoText("Goto.Hexadecimal", "fallback"), "Hexadecimal"; got != want {
		t.Errorf("GotoText for a known key = %q, want %q", got, want)
	}
	if got, want := GotoText("Goto.NoSuchKeyForTesting", "fallback text"), "fallback text"; got != want {
		t.Errorf("GotoText for a missing key = %q, want the fallback %q", got, want)
	}
}

// gotoDialogCheckbox returns the only *vtui.Checkbox in the dialog (the
// hexadecimal toggle), the way firstDialogEdit (file_test.go) already finds
// the only *vtui.Edit.
func gotoDialogCheckbox(dlg vtui.Container) *vtui.Checkbox {
	for _, item := range dlg.GetChildren() {
		if cb, ok := item.(*vtui.Checkbox); ok {
			return cb
		}
	}
	return nil
}

func TestShowGotoOffset(t *testing.T) {
	fm := vtui.FrameManager

	t.Run("valid decimal input reports the offset and closes", func(t *testing.T) {
		fm.Init(vtui.NewSilentScreenBuf())
		got := int64(-1)
		ShowGotoOffset(nil, "Go to offset", "Byte offset:", 0, func(offset int64) { got = offset })

		dlg := fm.GetTopFrame().(vtui.Container)
		edit := firstDialogEdit(dlg)
		if edit == nil {
			t.Fatal("could not find the offset Edit field")
		}
		edit.SetText("123")
		testutil.ClickDialogButton(t, dlg, "Ok")

		if got != 123 {
			t.Errorf("onOK offset = %d, want 123", got)
		}
		if !dlg.(vtui.Frame).IsDone() {
			t.Error("the dialog did not close on valid input")
		}
	})

	t.Run("valid hexadecimal input reports the offset", func(t *testing.T) {
		fm.Init(vtui.NewSilentScreenBuf())
		got := int64(-1)
		ShowGotoOffset(nil, "Go to offset", "Byte offset:", 0, func(offset int64) { got = offset })

		dlg := fm.GetTopFrame().(vtui.Container)
		edit := firstDialogEdit(dlg)
		checkbox := gotoDialogCheckbox(dlg)
		if edit == nil || checkbox == nil {
			t.Fatal("could not find the offset Edit field or the Hexadecimal checkbox")
		}
		edit.SetText("1A")
		checkbox.State = 1
		testutil.ClickDialogButton(t, dlg, "Ok")

		if got != 26 {
			t.Errorf("onOK offset = %d, want 26 (0x1A)", got)
		}
	})

	t.Run("invalid input reports an error and keeps the dialog open", func(t *testing.T) {
		fm.Init(vtui.NewSilentScreenBuf())
		called := false
		ShowGotoOffset(nil, "Go to offset", "Byte offset:", 0, func(offset int64) { called = true })

		dlg := fm.GetTopFrame().(vtui.Container)
		edit := firstDialogEdit(dlg)
		if edit == nil {
			t.Fatal("could not find the offset Edit field")
		}
		edit.SetText("not a number")
		testutil.ClickDialogButton(t, dlg, "Ok")

		if called {
			t.Error("onOK must not fire on an invalid offset")
		}
		if dlg.(vtui.Frame).IsDone() {
			t.Error("the dialog closed despite invalid input")
		}
		if top := fm.GetTopFrame(); any(top) == any(dlg) {
			t.Error("no error dialog appeared over the goto dialog")
		}
	})

	t.Run("cancel closes the dialog without calling onOK", func(t *testing.T) {
		fm.Init(vtui.NewSilentScreenBuf())
		called := false
		ShowGotoOffset(nil, "Go to offset", "Byte offset:", 0, func(offset int64) { called = true })

		dlg := fm.GetTopFrame().(vtui.Container)
		testutil.ClickDialogButton(t, dlg, "Cancel")

		if called {
			t.Error("onOK must not fire on cancel")
		}
		if !dlg.(vtui.Frame).IsDone() {
			t.Error("the dialog did not close on cancel")
		}
	})
}

// gotoDialogEdits returns every *vtui.Edit in the dialog in the order they
// were added: for ShowEditorPosition that is [line, position].
func gotoDialogEdits(dlg vtui.Container) []*vtui.Edit {
	var edits []*vtui.Edit
	for _, item := range dlg.GetChildren() {
		if e, ok := item.(*vtui.Edit); ok {
			edits = append(edits, e)
		}
	}
	return edits
}

func TestShowEditorPosition(t *testing.T) {
	fm := vtui.FrameManager

	t.Run("valid input reports line and position and closes", func(t *testing.T) {
		fm.Init(vtui.NewSilentScreenBuf())
		gotLine, gotPosition := -1, -1
		ShowEditorPosition(nil, 1, 1, func(line, position int) { gotLine, gotPosition = line, position })

		dlg := fm.GetTopFrame().(vtui.Container)
		edits := gotoDialogEdits(dlg)
		if len(edits) != 2 {
			t.Fatalf("dialog has %d Edit fields, want 2 (line, position)", len(edits))
		}
		edits[0].SetText("42")
		edits[1].SetText("7")
		testutil.ClickDialogButton(t, dlg, "Ok")

		if gotLine != 42 || gotPosition != 7 {
			t.Errorf("onOK(line, position) = (%d, %d), want (42, 7)", gotLine, gotPosition)
		}
		if !dlg.(vtui.Frame).IsDone() {
			t.Error("the dialog did not close on valid input")
		}
	})

	t.Run("non-positive or non-numeric input is rejected", func(t *testing.T) {
		cases := []struct {
			name, line, position string
		}{
			{"zero line", "0", "1"},
			{"zero position", "1", "0"},
			{"negative line", "-1", "1"},
			{"negative position", "1", "-1"},
			{"non-numeric line", "x", "1"},
			{"non-numeric position", "1", "x"},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				fm.Init(vtui.NewSilentScreenBuf())
				called := false
				ShowEditorPosition(nil, 1, 1, func(line, position int) { called = true })

				dlg := fm.GetTopFrame().(vtui.Container)
				edits := gotoDialogEdits(dlg)
				if len(edits) != 2 {
					t.Fatalf("dialog has %d Edit fields, want 2 (line, position)", len(edits))
				}
				edits[0].SetText(tt.line)
				edits[1].SetText(tt.position)
				testutil.ClickDialogButton(t, dlg, "Ok")

				if called {
					t.Errorf("onOK must not fire for line=%q position=%q", tt.line, tt.position)
				}
				if dlg.(vtui.Frame).IsDone() {
					t.Errorf("the dialog closed despite invalid input line=%q position=%q", tt.line, tt.position)
				}
			})
		}
	})

	t.Run("cancel closes the dialog without calling onOK", func(t *testing.T) {
		fm.Init(vtui.NewSilentScreenBuf())
		called := false
		ShowEditorPosition(nil, 1, 1, func(line, position int) { called = true })

		dlg := fm.GetTopFrame().(vtui.Container)
		testutil.ClickDialogButton(t, dlg, "Cancel")

		if called {
			t.Error("onOK must not fire on cancel")
		}
		if !dlg.(vtui.Frame).IsDone() {
			t.Error("the dialog did not close on cancel")
		}
	})
}
