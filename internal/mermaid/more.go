package mermaid

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	pieHeader  = regexp.MustCompile(`^pie(?:\s+showData)?(?:\s+title\s+(.+))?$`)
	pieTitle   = regexp.MustCompile(`^title\s+(.+)$`)
	pieSlice   = regexp.MustCompile(`^"([^"]+)"\s*:\s*(\d+(?:\.\d+)?)$`)
	ganttHead  = regexp.MustCompile(`^gantt$`)
	ganttSkip  = regexp.MustCompile(`^(?:dateFormat|axisFormat|excludes|includes|todayMarker|tickInterval|weekday|weekend|accTitle|accDescr)\b`)
	ganttSect  = regexp.MustCompile(`^section\s+(.+)$`)
	ganttTask  = regexp.MustCompile(`^([^:]+?)\s*:\s*(.+)$`)
	stateHead  = regexp.MustCompile(`^stateDiagram(?:-v2)?$`)
	stateTrans = regexp.MustCompile(`^(\[\*\]|[A-Za-z0-9_]+)\s*-->\s*(\[\*\]|[A-Za-z0-9_]+)\s*(?::\s*(.*))?$`)
	stateAlias = regexp.MustCompile(`^state\s+"([^"]+)"\s+as\s+([A-Za-z0-9_]+)$`)
	stateDesc  = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*:\s*(.+)$`)
	erHead     = regexp.MustCompile(`^erDiagram$`)
	erRel      = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s+(\|\||\|o|\}o|\}\|)(--|\.\.)(\|\||o\||o\{|\|\{)\s+([A-Za-z0-9_-]+)\s*:\s*(?:"([^"]*)"|(\S+))$`)
	erEntity   = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*\{$`)
)

const pieBar = 20

// Pie converts a ```mermaid pie chart: each slice as a bar, its share and its
// value.
func Pie(source string) (text string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) > MaxLines {
		return "", false
	}
	type slice struct {
		label string
		value float64
		raw   string
	}
	var slices []slice
	title, sawHeader := "", false
	total := 0.0
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !sawHeader {
			m := pieHeader.FindStringSubmatch(line)
			if m == nil {
				return "", false
			}
			title, sawHeader = strings.TrimSpace(m[1]), true
			continue
		}
		if m := pieTitle.FindStringSubmatch(line); m != nil {
			title = strings.TrimSpace(m[1])
			continue
		}
		m := pieSlice.FindStringSubmatch(line)
		if m == nil {
			return "", false
		}
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return "", false
		}
		slices = append(slices, slice{m[1], v, m[2]})
		total += v
		if len(slices) > maxEdges {
			return "", false
		}
	}
	if !sawHeader || len(slices) == 0 || total <= 0 {
		return "", false
	}
	width := 0
	for _, s := range slices {
		if n := utf8.RuneCountInString(s.label); n > width {
			width = n
		}
	}
	var out []string
	if title != "" {
		out = append(out, title)
	}
	for _, s := range slices {
		share := s.value / total
		bar := strings.Repeat("█", int(share*pieBar+0.5))
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(s.label))
		out = append(out, fmt.Sprintf("%s%s  %-*s %5.1f%%  (%s)", s.label, pad, pieBar, bar, share*100, s.raw))
	}
	return strings.Join(out, "\n"), true
}

// Gantt converts a ```mermaid gantt chart to a list: the title, each section
// and its tasks with their schedule as written. Dates are not laid out on an
// axis; the layout options are ignored.
func Gantt(source string) (text string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) > MaxLines {
		return "", false
	}
	var out []string
	sawHeader, tasks := false, 0
	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !sawHeader {
			if !ganttHead.MatchString(line) {
				return "", false
			}
			sawHeader = true
			continue
		}
		switch {
		case ganttSkip.MatchString(line):
		case pieTitle.MatchString(line):
			out = append(out, pieTitle.FindStringSubmatch(line)[1])
		case ganttSect.MatchString(line):
			out = append(out, "▸ "+ganttSect.FindStringSubmatch(line)[1])
		case ganttTask.MatchString(line):
			m := ganttTask.FindStringSubmatch(line)
			out = append(out, "  "+m[1]+" — "+m[2])
			tasks++
		default:
			return "", false
		}
		if tasks > maxEdges {
			return "", false
		}
	}
	if !sawHeader || tasks == 0 {
		return "", false
	}
	return strings.Join(out, "\n"), true
}

// State converts a ```mermaid stateDiagram: transitions with [*] as the start
// and end marks, state aliases and descriptions. Composite states, notes,
// forks and concurrency are refused.
func State(source string) (text string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) > MaxLines {
		return "", false
	}
	names := make(map[string]string)
	var out, descriptions []string
	sawHeader := false
	name := func(id string) string {
		if n := names[id]; n != "" {
			return n
		}
		return id
	}
	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !sawHeader {
			if !stateHead.MatchString(line) {
				return "", false
			}
			sawHeader = true
			continue
		}
		switch {
		case classIgnore.MatchString(line):
		case stateAlias.MatchString(line):
			m := stateAlias.FindStringSubmatch(line)
			names[m[2]] = m[1]
		case stateTrans.MatchString(line):
			m := stateTrans.FindStringSubmatch(line)
			from, to := name(m[1]), name(m[2])
			if m[1] == "[*]" {
				from = "●"
			}
			if m[2] == "[*]" {
				to = "◎"
			}
			entry := "(" + from + ") ──▶ (" + to + ")"
			if label := strings.TrimSpace(m[3]); label != "" {
				entry += ": " + label
			}
			out = append(out, entry)
		case stateDesc.MatchString(line):
			m := stateDesc.FindStringSubmatch(line)
			descriptions = append(descriptions, name(m[1])+": "+strings.TrimSpace(m[2]))
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
	return strings.Join(append(out, descriptions...), "\n"), true
}

var erLeft = map[string]string{"||": "1", "|o": "0..1", "}o": "0..*", "}|": "1..*"}
var erRight = map[string]string{"||": "1", "o|": "0..1", "o{": "0..*", "|{": "1..*"}

// ER converts a ```mermaid erDiagram: entities with their attribute lines,
// and each relationship with its cardinalities and label.
func ER(source string) (text string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) > MaxLines {
		return "", false
	}
	var entities, relations []string
	attrs := make(map[string][]string)
	open, sawHeader := "", false
	touch := func(name string) {
		if _, seen := attrs[name]; !seen {
			attrs[name] = nil
			entities = append(entities, name)
		}
	}
	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), ";"))
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !sawHeader {
			if !erHead.MatchString(line) {
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
			attrs[open] = append(attrs[open], line)
			continue
		}
		switch {
		case erEntity.MatchString(line):
			open = erEntity.FindStringSubmatch(line)[1]
			touch(open)
		case erRel.MatchString(line):
			m := erRel.FindStringSubmatch(line)
			touch(m[1])
			touch(m[5])
			glyph := "───"
			if m[3] == ".." {
				glyph = "┄┄┄"
			}
			label := m[6]
			if label == "" {
				label = m[7]
			}
			relations = append(relations, fmt.Sprintf("%s %s %s %s %s: %s", m[1], erLeft[m[2]], glyph, erRight[m[4]], m[5], label))
		default:
			return "", false
		}
		if len(relations) > maxEdges {
			return "", false
		}
	}
	if !sawHeader || open != "" || len(entities) == 0 {
		return "", false
	}
	var out []string
	for _, e := range entities {
		if len(attrs[e]) == 0 {
			continue
		}
		out = append(out, "entity "+e)
		for _, a := range attrs[e] {
			out = append(out, "  "+a)
		}
	}
	out = append(out, relations...)
	return strings.Join(out, "\n"), true
}
