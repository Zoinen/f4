package vtui

import (
	"log"
	"os"
	"strings"
)

// Help uses the dialog stream with an owner-declared layout. Only visible rows
// cross the wire; the engine retains topic/history/link and scroll ownership.
func (hv *HelpView) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := hv.GetPosition()
	node := map[string]any{
		"id": SemanticID(hv), "kind": "dialog", "layout": "help",
		"title": strings.TrimSpace(hv.GetTitle()), "modal": true, "showClose": true,
		"x": x1, "y": y1, "w": x2 - x1 + 1, "h": y2 - y1 + 1,
		"canGoBack": len(hv.history) > 0, "children": []map[string]any{},
		"helpLines": []map[string]any{},
	}
	if hv.current == nil {
		return node
	}
	sticky := min(hv.current.StickyRows, len(hv.current.Lines))
	page := hv.scrollableRows()
	total := len(hv.current.Lines) - sticky
	hv.scrollTop = max(0, min(hv.scrollTop, total-page))
	rows := make([]map[string]any, 0, sticky+page)
	for i := range sticky {
		rows = append(rows, hv.semanticHelpLine(i))
	}
	for i := sticky + hv.scrollTop; i < min(len(hv.current.Lines), sticky+hv.scrollTop+page); i++ {
		rows = append(rows, hv.semanticHelpLine(i))
	}
	node["helpLines"] = rows
	node["topic"] = hv.current.Name
	node["scrollTop"] = hv.scrollTop
	node["pageRows"] = page
	node["totalRows"] = total
	node["stickyRows"] = sticky
	return node
}

func (hv *HelpView) scrollableRows() int {
	rows := hv.Y2 - hv.Y1 - 1
	if hv.nativeRows > 0 {
		rows = hv.nativeRows
	}
	if hv.current != nil {
		rows -= hv.current.StickyRows
	}
	return max(1, rows)
}

// ScrollToHelpLine centers a search result using the active renderer's viewport.
func (hv *HelpView) ScrollToHelpLine(line int) int {
	if hv.current == nil {
		return hv.scrollTop
	}
	desired := max(0, line-hv.current.StickyRows-hv.scrollableRows()/2)
	hv.scrollBy(desired - hv.scrollTop)
	return hv.scrollTop
}

func (hv *HelpView) semanticHelpLine(index int) map[string]any {
	line := hv.current.Lines[index]
	spans := []map[string]any{}
	for _, span := range hv.lineSpans(line, index) {
		spans = append(spans, map[string]any{
			"text": span.text, "bold": span.bold, "link": span.link,
			"selected": span.link >= 0 && span.link == hv.selectedIdx,
		})
	}
	return map[string]any{"index": index, "centered": strings.HasPrefix(line, "^"), "spans": spans}
}

func (hv *HelpView) HandleSemanticAction(action map[string]any) bool {
	if semanticString(action["target"]) != SemanticID(hv) {
		return false
	}
	name := semanticString(action["action"])
	switch name {
	case "help.activate":
		index := semanticInt(action["index"])
		if hv.current == nil || index < 0 || index >= len(hv.current.Links) {
			return false
		}
		// Ignore queued clicks from a topic that has already been replaced.
		if topic := semanticString(action["topic"]); topic != "" && topic != hv.current.Name {
			return false
		}
		hv.selectedIdx = index
		hv.SwitchTopic(hv.current.Links[index].Target)
	case "help.back":
		hv.PopTopic()
	case "help.scroll":
		if position, ok := action["position"]; ok {
			hv.scrollBy(semanticInt(position) - hv.scrollTop)
		} else {
			hv.scrollBy(semanticInt(action["delta"]))
		}
	case "help.viewport":
		hv.nativeRows = max(1, min(1000, semanticInt(action["rows"])))
		hv.scrollBy(0)
	case "dialog.close", "window.close", "close":
		hv.Close()
	default:
		return false
	}
	if os.Getenv("F4_HELP_TRACE") != "" {
		log.Printf("[FIX:help] action=%s scrollTop=%d viewportRows=%d", name, hv.scrollTop, hv.nativeRows)
	}
	return true
}

type helpSpan struct {
	text string
	bold bool
	link int
}

// lineSpans is shared by console and native rendering: never expose HLF control
// syntax or parse it a second time in a frontend.
func (hv *HelpView) lineSpans(line string, lineIndex int) []helpSpan {
	line = strings.TrimPrefix(line, "^")
	links := []int{}
	for i, link := range hv.current.Links {
		if link.Line == lineIndex {
			links = append(links, i)
		}
	}
	spans := []helpSpan{}
	var text strings.Builder
	bold := false
	linkIndex := -1
	nextLink := 0
	flush := func() {
		if text.Len() > 0 {
			spans = append(spans, helpSpan{text: text.String(), bold: bold, link: linkIndex})
			text.Reset()
		}
	}
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '#':
			flush()
			bold = !bold
		case '~':
			if linkIndex < 0 && nextLink >= len(links) {
				text.WriteRune(runes[i])
				continue
			}
			flush()
			if linkIndex >= 0 {
				for i+1 < len(runes) && runes[i] != '@' {
					i++
				}
				linkIndex = -1
			} else {
				linkIndex = links[nextLink]
				nextLink++
			}
		default:
			text.WriteRune(runes[i])
		}
	}
	flush()
	return spans
}
