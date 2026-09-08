package main

import (
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"
)

// occurrenceEditor builds an editor whose text area starts at screen row 1
// (row 0 is the top bar) and paints it once.
func occurrenceEditor(t *testing.T, text string) (*EditorView, *vtui.ScreenBuf) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "test.txt")
	t.Cleanup(ev.Close)
	ev.SetPosition(0, 0, 80, 12)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 13)
	return ev, scr
}

// selectRange turns on a plain selection covering [from, to).
func selectRange(ev *EditorView, from, to int) {
	ev.rectSelActive = false
	ev.selActive = true
	ev.selAnchorOffset = from
	ev.CursorLine = ev.li.GetLineAtOffset(to)
	ev.CursorPos = to - ev.li.GetLineOffset(ev.CursorLine)
}

func TestEditor_OccurrenceHighlight_MarksOtherMatches(t *testing.T) {
	old := AppConfig.EditorMarkOccurrences
	AppConfig.EditorMarkOccurrences = true
	defer func() { AppConfig.EditorMarkOccurrences = old }()

	// "foo" selected on the first line; the copy on the third line is the
	// one that has to light up, and so does the "foo" inside "food": the
	// request is for occurrences of the text, not for whole words.
	ev, scr := occurrenceEditor(t, "foo bar\nbaz\nfoo food\n")
	selectRange(ev, 0, 3)
	ev.Show(scr)

	occAttr := vtui.Palette[ColEditorOccurrence]
	occBG := vtui.GetRGBBack(occAttr)

	for x := 0; x < 3; x++ {
		if got := vtui.GetRGBBack(scr.GetCell(x, 3).Attributes); got != occBG {
			t.Errorf("cell (%d,3) background = %06X, want the occurrence colour %06X", x, got, occBG)
		}
	}
	for x := 4; x < 7; x++ {
		if got := vtui.GetRGBBack(scr.GetCell(x, 3).Attributes); got != occBG {
			t.Errorf("cell (%d,3) inside \"food\" = %06X, want the occurrence colour %06X", x, got, occBG)
		}
	}
	// The mark stops where the match does.
	if got := vtui.GetRGBBack(scr.GetCell(7, 3).Attributes); got == occBG {
		t.Error("the highlight ran past the end of the match")
	}
	// The line between them carries no match at all.
	if got := vtui.GetRGBBack(scr.GetCell(0, 2).Attributes); got == occBG {
		t.Error("an unrelated line was marked as an occurrence")
	}
	// The selection itself keeps the selection colour rather than the
	// weaker occurrence one.
	selBG := vtui.GetRGBBack(vtui.Palette[vtui.ColDialogEditSelected])
	if got := vtui.GetRGBBack(scr.GetCell(0, 1).Attributes); got != selBG {
		t.Errorf("selection cell background = %06X, want the selection colour %06X", got, selBG)
	}
}

func TestEditor_OccurrenceHighlight_ClearsWithSelection(t *testing.T) {
	old := AppConfig.EditorMarkOccurrences
	AppConfig.EditorMarkOccurrences = true
	defer func() { AppConfig.EditorMarkOccurrences = old }()

	ev, scr := occurrenceEditor(t, "foo\nfoo\n")
	selectRange(ev, 0, 3)
	ev.Show(scr)

	occBG := vtui.GetRGBBack(vtui.Palette[ColEditorOccurrence])
	if got := vtui.GetRGBBack(scr.GetCell(0, 2).Attributes); got != occBG {
		t.Fatalf("second line was not marked while selected: %06X", got)
	}

	ev.selActive = false
	ev.Show(scr)
	if got := vtui.GetRGBBack(scr.GetCell(0, 2).Attributes); got == occBG {
		t.Error("occurrence highlight stayed on screen after the selection was dropped")
	}
}

func TestEditor_OccurrenceHighlight_RespectsSetting(t *testing.T) {
	old := AppConfig.EditorMarkOccurrences
	AppConfig.EditorMarkOccurrences = false
	defer func() { AppConfig.EditorMarkOccurrences = old }()

	ev, scr := occurrenceEditor(t, "foo\nfoo\n")
	selectRange(ev, 0, 3)
	ev.Show(scr)

	occBG := vtui.GetRGBBack(vtui.Palette[ColEditorOccurrence])
	if got := vtui.GetRGBBack(scr.GetCell(0, 2).Attributes); got == occBG {
		t.Error("occurrences were marked while the setting was off")
	}
}

func TestEditor_OccurrenceNeedle_Eligibility(t *testing.T) {
	old := AppConfig.EditorMarkOccurrences
	AppConfig.EditorMarkOccurrences = true
	defer func() { AppConfig.EditorMarkOccurrences = old }()

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ev := NewEditorView(piecetable.New([]byte("word\t  \nword word\n")), nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 12)

	if got := ev.occurrenceNeedle(); got != nil {
		t.Errorf("needle without a selection = %q, want none", got)
	}

	selectRange(ev, 0, 4)
	if got := string(ev.occurrenceNeedle()); got != "word" {
		t.Errorf("needle = %q, want \"word\"", got)
	}

	// A single character says nothing and lights up half the screen.
	selectRange(ev, 0, 1)
	if got := ev.occurrenceNeedle(); got != nil {
		t.Errorf("single-character needle = %q, want none", got)
	}

	// Indentation is not a term.
	selectRange(ev, 4, 7)
	if got := ev.occurrenceNeedle(); got != nil {
		t.Errorf("whitespace-only needle = %q, want none", got)
	}

	// Neither is a block of lines.
	selectRange(ev, 0, 12)
	if got := ev.occurrenceNeedle(); got != nil {
		t.Errorf("multi-line needle = %q, want none", got)
	}

	// A rectangular selection has no single text to look for.
	selectRange(ev, 0, 4)
	ev.rectSelActive = true
	if got := ev.occurrenceNeedle(); got != nil {
		t.Errorf("needle for a rectangular selection = %q, want none", got)
	}
}

// Matches are searched per painted fragment, so one split across a wrapped row
// has to be found from both halves.
func TestEditor_OccurrenceSpans_AcrossWrappedRows(t *testing.T) {
	old := AppConfig.EditorMarkOccurrences
	AppConfig.EditorMarkOccurrences = true
	defer func() { AppConfig.EditorMarkOccurrences = old }()

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	text := "abcd abcd"
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "test.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 12)
	selectRange(ev, 0, 4)

	needle := ev.occurrenceNeedle()
	if string(needle) != "abcd" {
		t.Fatalf("needle = %q, want \"abcd\"", needle)
	}

	// A fragment ending in the middle of the second "abcd" still reports it.
	spans := ev.appendOccurrenceSpans(nil, needle, 0, 7, 0, len(text))
	found := false
	for _, span := range spans {
		if span.Off == 5 && span.Len == 4 {
			found = true
		}
	}
	if !found {
		t.Errorf("spans = %+v, want the match at offset 5 to be reported", spans)
	}

	// And so does one starting after it began.
	spans = ev.appendOccurrenceSpans(spans, needle, 7, len(text), 0, len(text))
	found = false
	for _, span := range spans {
		if span.Off == 5 && span.Len == 4 {
			found = true
		}
	}
	if !found {
		t.Errorf("spans = %+v, want the match at offset 5 to be reported", spans)
	}

	// The padding never reaches outside the logical line.
	spans = ev.appendOccurrenceSpans(spans, needle, 0, 4, 0, 4)
	if len(spans) != 1 || spans[0].Off != 0 {
		t.Errorf("spans clipped to the line = %+v, want just the match at 0", spans)
	}
}

func TestEditor_OccurrenceHighlight_HasDedicatedPaletteSlot(t *testing.T) {
	SetDefaultF4Palette()
	if vtui.Palette[ColEditorOccurrence] == vtui.Palette[ColEditorText] {
		t.Error("occurrence colour is indistinguishable from ordinary editor text")
	}
	if vtui.Palette[ColEditorOccurrence] == vtui.Palette[vtui.ColDialogEditSelected] {
		t.Error("occurrence colour is indistinguishable from the selection")
	}
	found := false
	for _, slot := range ColorSlots {
		if slot.Index == ColEditorOccurrence {
			found = true
			if slot.Canonical != "Editor.Occurrence" {
				t.Errorf("occurrence colour slot is named %q", slot.Canonical)
			}
		}
	}
	if !found {
		t.Error("occurrence colour has no farcolors.ini slot")
	}
}
