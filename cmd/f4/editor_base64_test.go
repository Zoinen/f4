package main

import (
	"errors"
	"testing"

	"github.com/unxed/f4/piecetable"
)

func selectEditorBytes(ev *EditorView, end int) {
	ev.selActive = true
	ev.selAnchorOffset = 0
	ev.CursorLine = ev.li.GetLineAtOffset(end)
	ev.CursorPos = end - ev.li.GetLineOffset(ev.CursorLine)
}

func TestEditorBase64EncodeAndDecodeSelection(t *testing.T) {
	ev := NewEditorView(piecetable.New([]byte("hello world")), nil, "test.txt")
	defer ev.Close()

	selectEditorBytes(ev, len("hello world"))
	if err := ev.transformBase64Selection(true); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := ev.GetText(); got != "aGVsbG8gd29ybGQ=" {
		t.Fatalf("encoded text = %q", got)
	}

	selectEditorBytes(ev, len("aGVsbG8gd29ybGQ="))
	if err := ev.transformBase64Selection(false); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := ev.GetText(); got != "hello world" {
		t.Fatalf("decoded text = %q", got)
	}
}

func TestEditorBase64DecodeAcceptsWhitespaceAndRejectsInvalidInput(t *testing.T) {
	ev := NewEditorView(piecetable.New([]byte("aGVs\n bG8=")), nil, "test.txt")
	defer ev.Close()

	selectEditorBytes(ev, len("aGVs\n bG8="))
	if err := ev.transformBase64Selection(false); err != nil {
		t.Fatalf("decode wrapped Base64: %v", err)
	}
	if got := ev.GetText(); got != "hello" {
		t.Fatalf("wrapped decode = %q", got)
	}

	ev.SetText("not Base64!")
	selectEditorBytes(ev, len("not Base64!"))
	before := ev.GetText()
	if err := ev.transformBase64Selection(false); err == nil {
		t.Fatal("invalid Base64 was accepted")
	}
	if got := ev.GetText(); got != before {
		t.Fatalf("invalid decode changed text to %q", got)
	}
}

func TestEditorBase64DecodeSelectionWithTrailingNewline(t *testing.T) {
	const input = "aGVsbG8gd29ybGQ=\n"
	ev := NewEditorView(piecetable.New([]byte(input)), nil, "test.txt")
	defer ev.Close()

	selectEditorBytes(ev, len(input))
	if err := ev.transformBase64Selection(false); err != nil {
		t.Fatalf("decode trailing-newline selection: %v", err)
	}
	if got := ev.GetText(); got != "hello world" {
		t.Fatalf("trailing-newline decode = %q", got)
	}
}

func TestEditorBase64RejectsRectangularSelection(t *testing.T) {
	ev := NewEditorView(piecetable.New([]byte("hello")), nil, "test.txt")
	defer ev.Close()
	ev.rectSelActive = true

	err := ev.transformBase64Selection(true)
	if err == nil || errors.Is(err, errBase64NoSelection) {
		t.Fatalf("rectangular selection error = %v", err)
	}
}

func TestEditorBase64ActionsExposeF11AndEditCommands(t *testing.T) {
	menu, ok := GetAction("Editor.Base64Menu")
	if !ok || menu.Area != "Editor" || len(menu.DefaultKeys) != 1 || menu.DefaultKeys[0] != "F11" {
		t.Fatalf("Base64 menu action = %#v, present=%t", menu, ok)
	}
	for _, name := range []string{"Editor.Base64Encode", "Editor.Base64Decode"} {
		action, ok := GetAction(name)
		if !ok || action.Area != "Editor" || action.MenuPath != "Edit" || action.Handler == nil {
			t.Fatalf("Base64 action %s = %#v, present=%t", name, action, ok)
		}
	}
}
