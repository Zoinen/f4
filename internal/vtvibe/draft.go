package vtvibe

import "strings"

// Draft sections: parts of draft.md that f4 itself writes and keeps up to
// date, next to whatever the human typed there. The first one is the list
// of edits turned down on the patch review screen (F8, docs/VTVIBE.md §7.3
// and appendix B.2): every rejection rewrites the whole list, so editing a
// reason or taking a rejection back never leaves a stale line behind.
//
// A section is fenced by two marker lines, "<!-- vtvibe:NAME -->" and
// "<!-- /vtvibe:NAME -->". They stay in draft.md so the section can be found
// again after the human edited the text around it, and Draft drops them, so
// the model only ever sees the section's text.

func draftSectionMarkers(name string) (begin, end string) {
	return "<!-- vtvibe:" + name + " -->", "<!-- /vtvibe:" + name + " -->"
}

func isDraftMarker(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "<!-- vtvibe:") || strings.HasPrefix(line, "<!-- /vtvibe:")
}

// cutDraftSection returns text without the named section (markers
// included) and whether the section was there. A section whose end marker
// was deleted is not a section: nothing is cut, so no text the human wrote
// below a lone marker is lost.
func cutDraftSection(text, name string) (string, bool) {
	begin, end := draftSectionMarkers(name)
	lines := strings.Split(text, "\n")
	from, to := -1, -1
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case begin:
			if from < 0 {
				from = i
			}
		case end:
			if from >= 0 && to < 0 {
				to = i
			}
		}
	}
	if from < 0 || to < 0 {
		return text, false
	}
	out := append(append([]string{}, lines[:from]...), lines[to+1:]...)
	return strings.Join(out, "\n"), true
}

// draftText is draft.md as the human means it: "" while it still holds the
// template.
func draftText(raw string) string {
	text := strings.TrimSpace(raw)
	if text == strings.TrimSpace(draftTemplate) {
		return ""
	}
	return text
}

// SetDraftSection puts text into draft.md as the section called name,
// replacing that section if it is already there, or appending it after
// the human's own text. An empty text removes the section; a draft left
// with nothing else in it goes back to the template.
func (s *Session) SetDraftSection(name, text string) {
	s.treeMu.RLock()
	defer s.treeMu.RUnlock()
	data, _ := s.tree.readFile(draftFile)
	rest, _ := cutDraftSection(strings.ReplaceAll(string(data), "\r\n", "\n"), name)
	rest = draftText(rest)
	text = strings.TrimSpace(text)
	if text == "" {
		if rest == "" {
			_ = s.tree.writeFile(draftFile, []byte(draftTemplate))
			return
		}
		_ = s.tree.writeFile(draftFile, []byte(rest+"\n"))
		return
	}
	begin, end := draftSectionMarkers(name)
	var b strings.Builder
	if rest != "" {
		b.WriteString(rest)
		b.WriteString("\n\n")
	}
	b.WriteString(begin + "\n" + text + "\n" + end + "\n")
	_ = s.tree.writeFile(draftFile, []byte(b.String()))
}

// HasDraftSection reports whether draft.md still holds the section called
// name - it does not once the draft was sent (ClearDraft) or the human cut
// the section out.
func (s *Session) HasDraftSection(name string) bool {
	s.treeMu.RLock()
	defer s.treeMu.RUnlock()
	data, _ := s.tree.readFile(draftFile)
	_, ok := cutDraftSection(strings.ReplaceAll(string(data), "\r\n", "\n"), name)
	return ok
}

// stripDraftMarkers drops the section marker lines, which are for f4, not
// for the model.
func stripDraftMarkers(text string) string {
	if !strings.Contains(text, "<!-- ") {
		return text
	}
	lines := strings.Split(text, "\n")
	out := lines[:0]
	for _, l := range lines {
		if !isDraftMarker(l) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
