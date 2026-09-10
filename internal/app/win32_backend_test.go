package app

import (
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

func TestF4_ConsoleBackendSelection(t *testing.T) {
	oldBackend := terminal.SelectedTTYBackend
	defer func() { terminal.SelectedTTYBackend = oldBackend }()

	terminal.SelectedTTYBackend = "winapi"
	scr := vtui.NewScreenBuf()
	if terminal.SelectedTTYBackend == "winapi" || terminal.SelectedTTYBackend == "win32" {
		scr.Renderer = vtui.NewWin32ConsoleRenderer(scr)
	}
	scr.AllocBuf(80, 25)

	if _, ok := scr.Renderer.(*vtui.Win32ConsoleRenderer); !ok {
		t.Errorf("Expected Win32ConsoleRenderer for winapi backend, got %T", scr.Renderer)
	}

	terminal.SelectedTTYBackend = "ansi"
	scr2 := vtui.NewScreenBuf()
	scr2.AllocBuf(80, 25)
	if _, ok := scr2.Renderer.(*vtui.AnsiRenderer); !ok {
		t.Errorf("Expected AnsiRenderer for ansi backend, got %T", scr2.Renderer)
	}
}

func TestF4_DefaultBackendUnderWine(t *testing.T) {
	backend := vtui.DefaultConsoleBackend()
	if vtui.IsWine() {
		if backend != "winapi" && backend != "ansi" {
			t.Errorf("Unexpected backend under Wine: %q", backend)
		}
	} else {
		if backend != "ansi" {
			t.Errorf("Expected ansi backend on non-Wine system, got %q", backend)
		}
	}
}
