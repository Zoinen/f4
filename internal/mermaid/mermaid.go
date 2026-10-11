// Package mermaid turns a Mermaid flowchart into plain Unicode text for the
// terminal Markdown viewer (f4#1664). A terminal cannot draw the diagram, so
// the text form lists it: one line per connection, nodes in their own
// brackets, arrows drawn with box characters, edge labels inline. Only the
// flowchart subset that this reader fully understands is converted; anything
// else (subgraphs, other diagram types, unknown syntax, oversized input) is
// refused so the caller keeps the diagram's source, which is exact.
package mermaid

import (
	"regexp"
	"strings"
)

// Limits on what one diagram may hold.
const (
	MaxLines         = 400
	maxEdges         = 600
	maxSubgraphDepth = 8
)

type node struct {
	open, close, text string
}

type edge struct {
	from, to string
	label    string
	style    byte // '-', '=' or '.'
	arrow    bool
	both     bool
}

// shapes lists node delimiters, longest opening first.
var shapes = []struct{ open, close string }{
	{"((", "))"}, {"([", "])"}, {"[[", "]]"}, {"[(", ")]"}, {"{{", "}}"},
	{"[", "]"}, {"(", ")"}, {"{", "}"}, {">", "]"},
}

var (
	header       = regexp.MustCompile(`^(?:flowchart|graph)(?:\s+(TD|TB|BT|LR|RL))?$`)
	nodeID       = regexp.MustCompile(`^[A-Za-z0-9_]+`)
	labelled     = regexp.MustCompile(`^\s*(--|==|-\.)\s+([^-=.|<>][^|]*?)\s+(-->|---|==>|===|\.->|-\.-)`)
	plainOp      = regexp.MustCompile(`^\s*(<-->|-{2,}>|-{3,}|={2,}>|={3,}|-\.+->|-\.+-)(?:\s*\|([^|]*)\|)?`)
	subgraphLine = regexp.MustCompile(`^subgraph\s+(?:([A-Za-z0-9_]+)\s*\[(.*)\]|(.+))$`)
	ignored      = regexp.MustCompile(`^(?:style|classDef|class|linkStyle|click|direction)\b`)
	brTag        = regexp.MustCompile(`(?i)<br\s*/?>`)
)

// Flowchart converts the body of a ```mermaid block. ok is false when the
// diagram is not a flowchart this package fully understands.
func Flowchart(source string) (text string, ok bool) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	if len(lines) > MaxLines {
		return "", false
	}
	nodes := make(map[string]*node)
	var order []string
	var edges []edge
	var events []event
	depth := 0
	sawHeader := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		line = strings.TrimSuffix(line, ";")
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !sawHeader {
			if !header.MatchString(line) {
				return "", false
			}
			sawHeader = true
			continue
		}
		if ignored.MatchString(line) {
			continue
		}
		if m := subgraphLine.FindStringSubmatch(line); m != nil {
			title := m[2]
			if title == "" {
				title = m[3]
			}
			title = strings.Trim(strings.TrimSpace(title), `"`)
			events = append(events, event{kind: 'o', text: title, depth: depth})
			depth++
			if depth > maxSubgraphDepth {
				return "", false
			}
			continue
		}
		if line == "end" {
			if depth == 0 {
				return "", false
			}
			depth--
			events = append(events, event{kind: 'c', depth: depth})
			continue
		}
		before := len(edges)
		first, ok := parseStatement(line, nodes, &order, &edges)
		if !ok || len(edges) > maxEdges {
			return "", false
		}
		if len(edges) == before {
			for _, id := range first {
				events = append(events, event{kind: 'n', id: id, depth: depth})
			}
		}
		for i := before; i < len(edges); i++ {
			events = append(events, event{kind: 'e', edge: i, depth: depth})
		}
	}
	if !sawHeader || len(order) == 0 || depth != 0 {
		return "", false
	}
	used := make(map[string]bool)
	for _, e := range edges {
		used[e.from], used[e.to] = true, true
	}
	var out, tail []string
	pre := func(d int) string { return strings.Repeat("│ ", d) }
	for _, ev := range events {
		switch ev.kind {
		case 'o':
			out = append(out, pre(ev.depth)+"┌ "+ev.text)
		case 'c':
			out = append(out, pre(ev.depth)+"└")
		case 'e':
			e := edges[ev.edge]
			out = append(out, pre(ev.depth)+display(nodes[e.from], e.from)+" "+arrowText(e)+" "+display(nodes[e.to], e.to))
		case 'n':
			if used[ev.id] {
				continue
			}
			line := pre(ev.depth) + display(nodes[ev.id], ev.id)
			// Loose nodes outside any group keep going last, as they always did.
			if ev.depth == 0 {
				tail = append(tail, line)
			} else {
				out = append(out, line)
			}
		}
	}
	return strings.Join(append(out, tail...), "\n"), true
}

// event is one printed thing, in source order: a subgraph opening or closing,
// an edge, or a node that stands alone.
type event struct {
	kind  byte
	text  string
	id    string
	edge  int
	depth int
}

func display(n *node, id string) string {
	if n == nil || n.open == "" {
		return "[" + id + "]"
	}
	return n.open + n.text + n.close
}

func arrowText(e edge) string {
	line, head := "─", "▶"
	switch e.style {
	case '=':
		line = "━"
	case '.':
		line = "┄"
	}
	tail := line
	if e.arrow {
		tail = head
	}
	prefix := ""
	if e.both {
		prefix = "◀"
	}
	if e.label == "" {
		return prefix + line + line + tail
	}
	return prefix + line + line + " " + e.label + " " + line + tail
}

// parseStatement reads NODE (EDGE NODE)* from line.
// classShorthand is the ":::name" a node may carry to pick a style class; it
// changes only colours, so it is skipped.
var classShorthand = regexp.MustCompile(`^:::[A-Za-z0-9_-]+`)

func skipClass(s string) string { return classShorthand.ReplaceAllString(s, "") }

// parseGroup reads NODE (& NODE)*.
func parseGroup(s string, nodes map[string]*node, order *[]string) (ids []string, rest string, ok bool) {
	id, rest, ok := parseNode(s, nodes, order)
	if !ok {
		return nil, "", false
	}
	ids = []string{id}
	for {
		t := strings.TrimSpace(rest)
		if !strings.HasPrefix(t, "&") {
			return ids, rest, true
		}
		id, rest, ok = parseNode(t[1:], nodes, order)
		if !ok {
			return nil, "", false
		}
		ids = append(ids, id)
	}
}

// parseStatement reads GROUP (EDGE GROUP)* from line, where a group of nodes
// joined by & is connected to every node of the next group. It returns the
// nodes of the first group, which stand alone when the line has no edge.
func parseStatement(line string, nodes map[string]*node, order *[]string, edges *[]edge) ([]string, bool) {
	prev, rest, ok := parseGroup(line, nodes, order)
	if !ok {
		return nil, false
	}
	first := prev
	for strings.TrimSpace(rest) != "" {
		e, after, ok := parseEdge(rest)
		if !ok {
			return nil, false
		}
		var next []string
		next, rest, ok = parseGroup(after, nodes, order)
		if !ok {
			return nil, false
		}
		for _, from := range prev {
			for _, to := range next {
				link := e
				link.from, link.to = from, to
				*edges = append(*edges, link)
			}
		}
		if len(*edges) > maxEdges {
			return nil, false
		}
		prev = next
	}
	return first, true
}

func parseNode(s string, nodes map[string]*node, order *[]string) (id, rest string, ok bool) {
	s = strings.TrimLeft(s, " \t")
	id = nodeID.FindString(s)
	if id == "" {
		return "", "", false
	}
	rest = s[len(id):]
	n, known := nodes[id]
	if !known {
		n = &node{}
		nodes[id] = n
		*order = append(*order, id)
	}
	for _, sh := range shapes {
		if !strings.HasPrefix(rest, sh.open) {
			continue
		}
		body := rest[len(sh.open):]
		var text, tail string
		if strings.HasPrefix(body, `"`) {
			end := strings.Index(body[1:], `"`)
			if end < 0 || !strings.HasPrefix(body[end+2:], sh.close) {
				return "", "", false
			}
			text, tail = body[1:end+1], body[end+2+len(sh.close):]
		} else {
			end := strings.Index(body, sh.close)
			if end < 0 {
				return "", "", false
			}
			text, tail = body[:end], body[end+len(sh.close):]
		}
		text = strings.TrimSpace(brTag.ReplaceAllString(text, " "))
		if strings.ContainsAny(text, "\n\r") {
			return "", "", false
		}
		if n.open == "" || text != "" {
			n.open, n.close, n.text = sh.open, sh.close, text
		}
		return id, skipClass(tail), true
	}
	rest = skipClass(rest)
	if strings.HasPrefix(rest, "@") {
		return "", "", false
	}
	return id, rest, true
}

func parseEdge(s string) (edge, string, bool) {
	if m := labelled.FindStringSubmatch(s); m != nil {
		e := edge{label: strings.TrimSpace(m[2]), style: m[1][0]}
		if m[1] == "-." {
			e.style = '.'
		}
		e.arrow = strings.HasSuffix(m[3], ">")
		return e, s[len(m[0]):], true
	}
	if m := plainOp.FindStringSubmatch(s); m != nil {
		op := m[1]
		e := edge{label: strings.TrimSpace(m[2]), style: op[0], arrow: strings.HasSuffix(op, ">"), both: op == "<-->"}
		switch {
		case strings.Contains(op, "."):
			e.style = '.'
		case op[0] == '<':
			e.style = '-'
		}
		return e, s[len(m[0]):], true
	}
	return edge{}, "", false
}
