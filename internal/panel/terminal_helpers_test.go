package panel

import (
	semantic "github.com/unxed/f4/internal/semantic"
	strings "strings"
)

func terminalModelText(rows []map[string]any) string {
	var out strings.Builder
	for _, row := range rows {
		for _, run := range semantic.AppMapSlice(row["runs"]) {
			out.WriteString(semantic.String(run["text"]))
		}
		out.WriteByte('\n')
	}
	return out.String()
}
