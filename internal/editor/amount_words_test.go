package editor

import (
	"errors"
	"testing"

	"github.com/unxed/f4/internal/piecetable"
)

func TestEditorAmountInWordsSelectionReplacesWithWords(t *testing.T) {
	const text = "21 руб 50 коп"
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "test.txt")
	defer ev.Close()

	selectEditorBytes(ev, len(text))
	got, err := ev.AmountInWordsSelection()
	if err != nil {
		t.Fatalf("AmountInWordsSelection: %v", err)
	}
	const want = "двадцать один рубль"
	if got != want || ev.GetText() != want {
		t.Fatalf("result %q, text %q; want %q", got, ev.GetText(), want)
	}
}

func TestEditorAmountInWordsSelectionErrors(t *testing.T) {
	ev := NewEditorView(piecetable.New([]byte("no digits here")), nil, "test.txt")
	defer ev.Close()

	if _, err := ev.AmountInWordsSelection(); !errors.Is(err, ErrCalculatorNoSelection) {
		t.Fatalf("no selection: error = %v", err)
	}
	selectEditorBytes(ev, len("no digits here"))
	if _, err := ev.AmountInWordsSelection(); err == nil {
		t.Fatal("a selection with no number was accepted")
	}
	if got := ev.GetText(); got != "no digits here" {
		t.Fatalf("text changed after the error: %q", got)
	}
	ev.RectSelActive = true
	if _, err := ev.AmountInWordsSelection(); err == nil {
		t.Fatal("a rectangular selection was accepted")
	}
}
