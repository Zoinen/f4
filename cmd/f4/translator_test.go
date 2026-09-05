package main

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestIsTranslatorMouseEvent(t *testing.T) {
	base := &vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		ButtonState:     vtinput.RightmostButtonPressed,
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.LeftAltPressed,
		KeyDown:         true,
		MouseEventFlags: 0,
	}
	if !isTranslatorMouseEvent(base) {
		t.Fatal("Ctrl+Alt right-button press was not recognized")
	}

	for name, event := range map[string]*vtinput.InputEvent{
		"release": func() *vtinput.InputEvent {
			e := *base
			e.KeyDown = false
			return &e
		}(),
		"motion": func() *vtinput.InputEvent {
			e := *base
			e.MouseEventFlags = vtinput.MouseMoved
			return &e
		}(),
		"missing alt": func() *vtinput.InputEvent {
			e := *base
			e.ControlKeyState = vtinput.LeftCtrlPressed
			return &e
		}(),
		"left button": func() *vtinput.InputEvent {
			e := *base
			e.ButtonState = vtinput.FromLeft1stButtonPressed
			return &e
		}(),
	} {
		if isTranslatorMouseEvent(event) {
			t.Errorf("%s event was incorrectly recognized", name)
		}
	}
}

func TestTranslatorVMenuTarget(t *testing.T) {
	menu := vtui.NewVMenu("&File")
	menu.SetPosition(2, 2, 30, 6)
	menu.SetVisible(true)
	menu.SetHelp("Menu.File")
	menu.AddItem(vtui.MenuItem{Text: "&Open"})

	target := translatorFrameTarget(menu, 5, 3)
	if target == nil {
		t.Fatal("translator did not find the visible menu row")
	}
	gotText, ok := target.(interface{ GetText() string })
	if !ok || gotText.GetText() != "&Open" {
		got := "<missing>"
		if ok {
			got = gotText.GetText()
		}
		t.Fatalf("menu target text = %q, want %q", got, "&Open")
	}
	report := formatTranslatorReport(target)
	if !strings.Contains(report, "Text: &Open") || !strings.Contains(report, "Help Context: Menu.File") {
		t.Fatalf("translator report = %q", report)
	}
}

func TestTranslatorPanelsFrameCommandLineTarget(t *testing.T) {
	prompt := NewCommandLine("> ")
	prompt.SetPosition(0, 4, 30, 4)
	prompt.Edit.SetText("dir")

	frame := &PanelsFrame{}
	frame.SetHelp("Panels")
	frame.cmdLine = prompt

	target := frame.translatorElementAt(8, 4)
	if target == nil {
		t.Fatal("translator did not find the command line")
	}
	gotText, ok := target.(interface{ GetText() string })
	if !ok || gotText.GetText() != "dir" {
		got := "<missing>"
		if ok {
			got = gotText.GetText()
		}
		t.Fatalf("command-line target text = %q, want %q", got, "dir")
	}
	if got := formatTranslatorReport(target); !strings.Contains(got, "Help Context: Panels") || strings.Contains(got, "Panels -> Panels") {
		t.Fatalf("command-line report = %q", got)
	}
}

func TestTranslatorMenuBarTarget(t *testing.T) {
	menu := vtui.NewMenuBar([]string{"&File", "&View"})
	menu.SetPosition(0, 0, 40, 0)
	menu.SetVisible(true)

	target := translatorMenuBarTarget(menu, 4, 0)
	if target == nil {
		t.Fatal("translator did not find the menu-bar item")
	}
	gotText, ok := target.(interface{ GetText() string })
	if !ok || gotText.GetText() != "&File" {
		got := "<missing>"
		if ok {
			got = gotText.GetText()
		}
		t.Fatalf("menu-bar target text = %q, want %q", got, "&File")
	}
}
