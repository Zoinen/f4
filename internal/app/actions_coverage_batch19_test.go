package app

import "testing"

func TestEditorHeaderIsBinaryAcceptsText(t *testing.T) {
	if editorHeaderIsBinary([]byte("plain text"), 65001) {
		t.Fatal("text header was classified as binary")
	}
}

func TestEditorHeaderIsBinaryFindsNUL(t *testing.T) {
	if !editorHeaderIsBinary([]byte{'a', 0, 'b'}, 65001) {
		t.Fatal("header containing NUL was not classified as binary")
	}
}

func TestEditorHeaderIsBinaryChecksDecodedHeader(t *testing.T) {
	if !editorHeaderIsBinary([]byte{0xc0, 0}, 1251) {
		t.Fatal("decoded header containing NUL was not classified as binary")
	}
}

func TestChoiceTextSelectsFirstChoice(t *testing.T) {
	if got := choiceText([]string{"first", "second"}, 0); got != "first" {
		t.Fatalf("choiceText first = %q", got)
	}
}

func TestChoiceTextSelectsLastChoice(t *testing.T) {
	if got := choiceText([]string{"first", "second"}, 1); got != "second" {
		t.Fatalf("choiceText last = %q", got)
	}
}

func TestChoiceTextRejectsNegativeIndex(t *testing.T) {
	if got := choiceText([]string{"first"}, -1); got != "" {
		t.Fatalf("choiceText negative index = %q", got)
	}
}

func TestChoiceTextRejectsPastEndIndex(t *testing.T) {
	if got := choiceText([]string{"first"}, 1); got != "" {
		t.Fatalf("choiceText past-end index = %q", got)
	}
}

func TestChoiceTextHandlesEmptyChoices(t *testing.T) {
	if got := choiceText(nil, 0); got != "" {
		t.Fatalf("choiceText empty choices = %q", got)
	}
}

func TestBoolToCheckboxStateForTrue(t *testing.T) {
	if got := boolToCheckboxState(true); got != 1 {
		t.Fatalf("boolToCheckboxState(true) = %d", got)
	}
}

func TestBoolToCheckboxStateForFalse(t *testing.T) {
	if got := boolToCheckboxState(false); got != 0 {
		t.Fatalf("boolToCheckboxState(false) = %d", got)
	}
}
