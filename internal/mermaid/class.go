package mermaid

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	classHeader = regexp.MustCompile(`^classDiagram$`)
	classDecl   = regexp.MustCompile(`^class\s+([A-Za-z0-9_]+)\s*(\{)?$`)
	classMember = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*:\s*(.+)$`)
	classRel    = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*(?:"([^"]*)"\s*)?(<\|--|<\|\.\.|\.\.\|>|--\|>|\*--|--\*|o--|--o|<--|-->|\.\.>|<\.\.|--|\.\.)\s*(?:"([^"]*)"\s*)?([A-Za-z0-9_]+)\s*(?::\s*(.*))?$`)
	classIgnore = regexp.MustCompile(`^direction\s+(?:TB|BT|LR|RL)$`)
)

// classArrow says how a relation is drawn and whether its ends are swapped:
// the arrow of "A <|-- B" points from B (the child) to A.
var classArrow = map[string]struct {
	glyph string
	swap  bool
}{
	"<|--": {"──▷", true}, "--|>": {"──▷", false},
	"<|..": {"┄┄▷", true}, "..|>": {"┄┄▷", false},
	"*--": {"◆──", false}, "--*": {"──◆", false},
	"o--": {"◇──", false}, "--o": {"──◇", false},
	"-->": {"──▶", false}, "<--": {"◀──", false},
	"..>": {"┄┄▶", false}, "<..": {"◀┄┄", false},
	"--": {"───", false}, "..": {"┄┄┄", false},
}

// Class converts the body of a ```mermaid classDiagram block: classes with
// their members and annotations, and the relations between them. Generics,
// namespaces, notes, styling and other syntax are refused so the caller keeps
// the source.
func Class(source string) (text string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) > MaxLines {
		return "", false
	}
	members := make(map[string][]string)
	var order []string
	declared := make(map[string]bool)
	touch := func(name string) {
		if _, seen := members[name]; !seen {
			members[name] = nil
			order = append(order, name)
		}
	}
	var relations []string
	sawHeader, open := false, ""
	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !sawHeader {
			if !classHeader.MatchString(line) {
				return "", false
			}
			sawHeader = true
			continue
		}
		if open != "" {
			if line == "}" {
				open = ""
				continue
			}
			members[open] = append(members[open], line)
			continue
		}
		switch {
		case classIgnore.MatchString(line):
		case classDecl.MatchString(line):
			m := classDecl.FindStringSubmatch(line)
			touch(m[1])
			declared[m[1]] = true
			if m[2] != "" {
				open = m[1]
			}
		case classRel.MatchString(line):
			m := classRel.FindStringSubmatch(line)
			touch(m[1])
			touch(m[5])
			arrow := classArrow[m[3]]
			from, fromCard, to, toCard := m[1], m[2], m[5], m[4]
			if arrow.swap {
				from, fromCard, to, toCard = to, toCard, from, fromCard
			}
			var b strings.Builder
			b.WriteString(from)
			if fromCard != "" {
				fmt.Fprintf(&b, " %q", fromCard)
			}
			b.WriteString(" " + arrow.glyph + " ")
			if toCard != "" {
				fmt.Fprintf(&b, "%q ", toCard)
			}
			b.WriteString(to)
			if label := strings.TrimSpace(m[6]); label != "" {
				b.WriteString(": " + label)
			}
			relations = append(relations, b.String())
		case classMember.MatchString(line):
			m := classMember.FindStringSubmatch(line)
			touch(m[1])
			members[m[1]] = append(members[m[1]], strings.TrimSpace(m[2]))
		default:
			return "", false
		}
		if len(relations) > maxEdges {
			return "", false
		}
	}
	if !sawHeader || open != "" || len(order) == 0 {
		return "", false
	}
	var out []string
	for _, name := range order {
		if !declared[name] && len(members[name]) == 0 {
			continue
		}
		out = append(out, "class "+name)
		for _, member := range members[name] {
			out = append(out, "  "+member)
		}
	}
	out = append(out, relations...)
	return strings.Join(out, "\n"), true
}
