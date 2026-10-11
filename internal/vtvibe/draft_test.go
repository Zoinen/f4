package vtvibe

import (
	"strings"
	"testing"
)

func TestSessionDraftSection(t *testing.T) {
	s := NewSession()
	if s.HasDraftSection("rejected-edits") {
		t.Fatal("a fresh draft has no sections")
	}

	// Into an untouched draft: the template goes, the markers stay in
	// draft.md but never reach Draft().
	s.SetDraftSection("rejected-edits", "Rejected:\n- edit 1")
	if got := s.Draft(); got != "Rejected:\n- edit 1" {
		t.Fatalf("Draft() = %q", got)
	}
	raw, _ := s.tree.readFile(draftFile)
	if !strings.Contains(string(raw), "<!-- vtvibe:rejected-edits -->") || strings.Contains(string(raw), "Type a multi-line") {
		t.Fatalf("draft.md = %q, want the markers and no template", raw)
	}
	if !s.HasDraftSection("rejected-edits") {
		t.Fatal("HasDraftSection false after SetDraftSection")
	}

	// The human writes above it; the section is rewritten, their text kept.
	_ = s.tree.writeFile(draftFile, []byte("Please redo it.\r\n\r\n"+strings.ReplaceAll(string(raw), "\n", "\r\n")))
	s.SetDraftSection("rejected-edits", "Rejected:\n- edit 1\n- edit 3")
	if got, want := s.Draft(), "Please redo it.\n\nRejected:\n- edit 1\n- edit 3"; got != want {
		t.Fatalf("Draft() = %q, want %q", got, want)
	}

	// Emptied: the section goes, the human's text stays.
	s.SetDraftSection("rejected-edits", "")
	if got := s.Draft(); got != "Please redo it." || s.HasDraftSection("rejected-edits") {
		t.Fatalf("Draft() = %q after removing the section", got)
	}

	// A draft with nothing but the section goes back to the template.
	s.ClearDraft()
	s.SetDraftSection("rejected-edits", "x")
	s.SetDraftSection("rejected-edits", "")
	if raw, _ := s.tree.readFile(draftFile); string(raw) != draftTemplate {
		t.Fatalf("draft.md = %q, want the template back", raw)
	}

	// Sending clears the section with the rest.
	s.SetDraftSection("rejected-edits", "x")
	s.ClearDraft()
	if s.HasDraftSection("rejected-edits") || s.Draft() != "" {
		t.Fatal("ClearDraft left the section behind")
	}
}

func TestCutDraftSectionNeedsBothMarkers(t *testing.T) {
	text := "a\n<!-- vtvibe:x -->\nb"
	if got, ok := cutDraftSection(text, "x"); ok || got != text {
		t.Fatalf("cut a section without an end marker: %q", got)
	}
	if got, ok := cutDraftSection("a\n<!-- vtvibe:x -->\nb\n<!-- /vtvibe:x -->\nc", "x"); !ok || got != "a\nc" {
		t.Fatalf("cutDraftSection = %q, %v", got, ok)
	}
}
