package mermaid

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	seqHeader      = regexp.MustCompile(`^sequenceDiagram$`)
	seqParticipant = regexp.MustCompile(`^(?:participant|actor)\s+([A-Za-z0-9_]+)(?:\s+as\s+(.+))?$`)
	seqMessage     = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*(-->>|->>|-->|->|--x|-x|--\)|-\))\s*[+-]?\s*([A-Za-z0-9_]+)\s*:\s*(.*)$`)
	seqNote        = regexp.MustCompile(`(?i)^note\s+(?:over\s+([A-Za-z0-9_]+(?:\s*,\s*[A-Za-z0-9_]+)?)|(?:left|right)\s+of\s+([A-Za-z0-9_]+))\s*:\s*(.*)$`)
	seqActivation  = regexp.MustCompile(`^(?:activate|deactivate)\s+[A-Za-z0-9_]+$`)
)

// arrows maps a sequence arrow to its drawing.
var arrows = map[string]string{
	"->>": "──▶", "-->>": "┄┄▶", "->": "───", "-->": "┄┄┄",
	"-x": "──✕", "--x": "┄┄✕", "-)": "──▷", "--)": "┄┄▷",
}

// Sequence converts the body of a ```mermaid sequenceDiagram block: messages
// (with their arrow kinds), participants and their aliases, notes,
// autonumber, activation lines. Blocks (loop, alt, opt, par, ...), boxes and
// other syntax are refused so the caller keeps the source.
func Sequence(source string) (text string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) > MaxLines {
		return "", false
	}
	names := make(map[string]string)
	name := func(id string) string {
		if n := names[id]; n != "" {
			return n
		}
		return id
	}
	var out []string
	sawHeader, numbered, count := false, false, 0
	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !sawHeader {
			if !seqHeader.MatchString(line) {
				return "", false
			}
			sawHeader = true
			continue
		}
		switch {
		case line == "autonumber":
			numbered = true
		case seqActivation.MatchString(line):
		case seqParticipant.MatchString(line):
			m := seqParticipant.FindStringSubmatch(line)
			if _, known := names[m[1]]; !known {
				names[m[1]] = strings.TrimSpace(m[2])
			}
		case seqNote.MatchString(line):
			m := seqNote.FindStringSubmatch(line)
			who := m[1]
			if who == "" {
				who = m[2]
			}
			parts := strings.Split(who, ",")
			for i, p := range parts {
				parts[i] = name(strings.TrimSpace(p))
			}
			out = append(out, fmt.Sprintf("✎ %s: %s", strings.Join(parts, ", "), strings.TrimSpace(m[3])))
		case seqMessage.MatchString(line):
			m := seqMessage.FindStringSubmatch(line)
			count++
			prefix := ""
			if numbered {
				prefix = fmt.Sprintf("%d. ", count)
			}
			out = append(out, fmt.Sprintf("%s%s %s %s: %s", prefix, name(m[1]), arrows[m[2]], name(m[3]), strings.TrimSpace(m[4])))
		default:
			return "", false
		}
		if len(out) > maxEdges {
			return "", false
		}
	}
	if !sawHeader || len(out) == 0 {
		return "", false
	}
	return strings.Join(out, "\n"), true
}

// Convert converts a ```mermaid block body by the diagram type on its first
// line; ok is false for a type or syntax this package does not fully handle.
func Convert(source string) (text string, ok bool) {
	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if seqHeader.MatchString(strings.TrimSuffix(line, ";")) {
			return Sequence(source)
		}
		if classHeader.MatchString(strings.TrimSuffix(line, ";")) {
			return Class(source)
		}
		head := strings.TrimSuffix(line, ";")
		switch {
		case pieHeader.MatchString(head):
			return Pie(source)
		case ganttHead.MatchString(head):
			return Gantt(source)
		case stateHead.MatchString(head):
			return State(source)
		case erHead.MatchString(head):
			return ER(source)
		}
		return Flowchart(source)
	}
	return "", false
}
