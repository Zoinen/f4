package dialog

import (
	"maps"
	"sort"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

// ProjectHelpSearch adds the console-owned search to the native visible rows.
// It does not retain a second search state or mutate the owner's snapshot.
func ProjectHelpSearch(node map[string]any) map[string]any {
	if node["layout"] != "help" {
		return node
	}
	out := maps.Clone(node)
	out["searchHint"] = i18n.Msg("Help.SearchHint")
	state := CurrentHelpSearch
	if state == nil || state.Frame == nil {
		return out
	}
	if node["id"] != vtui.SemanticID(state.Frame) || node["topic"] != state.TopicName {
		return out
	}
	if len(state.Query) == 0 {
		return out
	}
	out["title"] = semantic.String(node["title"]) + " [" + string(state.Query) + "]"
	rows := semantic.AppMapSlice(node["helpLines"])
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		copy := maps.Clone(row)
		copy["spans"] = helpSearchSpans(row, state)
		result = append(result, copy)
	}
	out["helpLines"] = result
	return out
}

func helpSearchSpans(row map[string]any, state *HelpSearchState) []map[string]any {
	spans := semantic.AppMapSlice(row["spans"])
	length := 0
	for _, span := range spans {
		length += len([]rune(semantic.String(span["text"])))
	}
	marks := make([]int, length)
	line := semantic.Int(row["index"])
	first := sort.Search(len(state.Matches), func(i int) bool { return state.Matches[i].line >= line })
	for i := first; i < len(state.Matches) && state.Matches[i].line == line; i++ {
		match := state.Matches[i]
		mark := 1
		if i == state.Selected {
			mark = 2
		}
		for offset := max(0, match.start); offset < min(length, match.end); offset++ {
			marks[offset] = max(marks[offset], mark)
		}
	}
	result := make([]map[string]any, 0, len(spans))
	offset := 0
	for _, span := range spans {
		runes := []rune(semantic.String(span["text"]))
		for start := 0; start < len(runes); {
			end := start + 1
			for end < len(runes) && marks[offset+end] == marks[offset+start] {
				end++
			}
			part := maps.Clone(span)
			part["text"] = string(runes[start:end])
			part["searchMatch"] = marks[offset+start] > 0
			part["searchSelected"] = marks[offset+start] == 2
			result = append(result, part)
			start = end
		}
		offset += len(runes)
	}
	return result
}

func traceHelpSearch(state *HelpSearchState) {
	// Do not log the query itself; help can include user-generated content.
	vtui.DebugLog("[FIX:help-search] topic=%s queryRunes=%d matches=%d selected=%d",
		strings.TrimSpace(state.TopicName), len(state.Query), len(state.Matches), state.Selected)
}
