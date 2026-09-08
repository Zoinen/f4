package main

import (
	"reflect"
	"testing"

	"github.com/unxed/vtui"
)

func TestEditor_MultiCursor_AddCursorSelectsWordFirst(t *testing.T) {
	ev := multiCursorEditor(t, "alpha beta alpha")
	ev.CursorLine = 0
	ev.CursorPos = 2 // inside the first "alpha"

	ev.AddCursorAtNextOccurrence()

	if !ev.selActive {
		t.Fatal("the first press did not select the word under the caret")
	}
	if start, end := ev.getSelectionRange(); start != 0 || end != 5 {
		t.Errorf("selection = [%d %d), want the word [0 5)", start, end)
	}
	if ev.multiCursor() {
		t.Errorf("the first press already added carets: %v", extraCaretOffsets(ev))
	}
}

func TestEditor_MultiCursor_AddCursorWalksTheCopies(t *testing.T) {
	ev := multiCursorEditor(t, "aa bb aa bb aa")
	ev.CursorLine = 0
	ev.CursorPos = 0

	ev.AddCursorAtNextOccurrence() // selects "aa" at 0
	ev.AddCursorAtNextOccurrence() // adds the copy at 6
	ev.AddCursorAtNextOccurrence() // adds the copy at 12

	// The newest copy is the primary caret, so the view follows it.
	if start, end := ev.getSelectionRange(); start != 12 || end != 14 {
		t.Errorf("primary selection = [%d %d), want [12 14)", start, end)
	}
	if got, want := extraCaretSelections(ev), [][2]int{{0, 2}, {6, 8}}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra selections = %v, want %v", got, want)
	}
}

// Once every copy has a caret, pressing again changes nothing rather than
// adding a caret on top of one that is already there.
func TestEditor_MultiCursor_AddCursorStopsWhenAllTaken(t *testing.T) {
	ev := multiCursorEditor(t, "aa bb aa")
	ev.CursorLine = 0
	ev.CursorPos = 0

	ev.AddCursorAtNextOccurrence()
	ev.AddCursorAtNextOccurrence()
	before := extraCaretSelections(ev)

	ev.AddCursorAtNextOccurrence()
	ev.AddCursorAtNextOccurrence()

	if got := extraCaretSelections(ev); !reflect.DeepEqual(got, before) {
		t.Errorf("extra selections = %v, want them unchanged at %v", got, before)
	}
}

// The search wraps: a copy before the selection is found once the end of the
// file has been passed.
func TestEditor_MultiCursor_AddCursorWrapsAround(t *testing.T) {
	ev := multiCursorEditor(t, "aa bb aa")
	ev.CursorLine = 0
	ev.CursorPos = 6 // inside the second "aa"

	ev.AddCursorAtNextOccurrence() // selects the copy at 6
	ev.AddCursorAtNextOccurrence() // the only other copy is behind it

	if start, end := ev.getSelectionRange(); start != 0 || end != 2 {
		t.Errorf("primary selection = [%d %d), want the wrapped copy [0 2)", start, end)
	}
	if got, want := extraCaretSelections(ev), [][2]int{{6, 8}}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra selections = %v, want %v", got, want)
	}
}

func TestEditor_MultiCursor_SelectAllOccurrences(t *testing.T) {
	ev := multiCursorEditor(t, "aa bb\naa cc aa")
	ev.CursorLine = 0
	ev.CursorPos = 0

	ev.SelectAllOccurrences()

	// The caret that was already on a copy stays the primary one, so the
	// view does not jump.
	if start, end := ev.getSelectionRange(); start != 0 || end != 2 {
		t.Errorf("primary selection = [%d %d), want [0 2)", start, end)
	}
	if got, want := extraCaretSelections(ev), [][2]int{{6, 8}, {12, 14}}; !reflect.DeepEqual(got, want) {
		t.Errorf("extra selections = %v, want %v", got, want)
	}
}

// Copies found this way are ordinary carets: typing replaces every one of them
// in a single undoable change.
func TestEditor_MultiCursor_SelectAllThenTypeReplacesEveryCopy(t *testing.T) {
	ev := multiCursorEditor(t, "foo bar\nfoo baz\nfoo")
	ev.CursorLine = 0
	ev.CursorPos = 1

	ev.SelectAllOccurrences()
	typeCharAtCarets(ev, 'X')

	if got, want := ev.pt.String(), "X bar\nX baz\nX"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}

	ev.Undo()
	if got, want := ev.pt.String(), "foo bar\nfoo baz\nfoo"; got != want {
		t.Errorf("buffer after undo = %q, want %q", got, want)
	}
}

// The scan pulls the text out in chunks; a copy lying across a chunk boundary
// still has to be found.
func TestEditor_MultiCursor_SearchCrossesChunkBoundaries(t *testing.T) {
	needle := "needle"
	filler := make([]byte, editorCaretSearchChunk-3)
	for i := range filler {
		filler[i] = '.'
	}
	text := needle + string(filler) + needle
	ev := multiCursorEditor(t, text)
	ev.CursorLine = 0
	ev.CursorPos = 0

	ev.AddCursorAtNextOccurrence() // selects the copy at 0
	ev.AddCursorAtNextOccurrence()

	wantStart := len(needle) + len(filler)
	if start, end := ev.getSelectionRange(); start != wantStart || end != wantStart+len(needle) {
		t.Errorf("primary selection = [%d %d), want [%d %d)", start, end, wantStart, wantStart+len(needle))
	}
}

// Editor actions that know nothing about the caret set put it down first,
// rather than working through the primary caret and leaving the rest painted
// over text they no longer describe.
func TestEditor_MultiCursor_PlainActionsCollapseTheSet(t *testing.T) {
	ev := multiCursorEditor(t, "one\ntwo\nthree")
	ev.toggleCursorAt(4)

	vtui.FrameManager.Push(ev)
	if !RunAction("Editor.DeleteLine") {
		t.Fatal("Editor.DeleteLine did not run")
	}

	if ev.multiCursor() {
		t.Errorf("extra carets survived a single-caret action: %v", extraCaretOffsets(ev))
	}
}

func TestEditor_MultiCursor_OccurrenceActionsAreRegistered(t *testing.T) {
	for name, key := range map[string]string{
		"Editor.AddCursorAtNextOccurrence": "CtrlShiftN",
		"Editor.SelectAllOccurrences":      "CtrlShiftL",
	} {
		action, ok := GetAction(name)
		if !ok {
			t.Errorf("%s is not registered", name)
			continue
		}
		if action.Area != "Editor" || action.MenuPath != "Edit" ||
			len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != key {
			t.Errorf("%s = %+v; want Editor/Edit bound to %s", name, action, key)
		}
	}
}
