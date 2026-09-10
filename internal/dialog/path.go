package dialog

import "github.com/mattn/go-runewidth"

// TruncPathLeft shortens path to at most width display cells by dropping
// characters from the front and marking the cut with an ellipsis. far2l
// truncates the paths it lists in menus the same way round
// (mix/StrCells.cpp, StrCellsTruncateLeft): the tail of a path is the
// part that identifies it.
func TruncPathLeft(path string, width int) string {
	const ellipsis = "…"
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(path) <= width {
		return path
	}
	runes := []rune(path)
	for i := 1; i < len(runes); i++ {
		if runewidth.StringWidth(string(runes[i:]))+1 <= width {
			return ellipsis + string(runes[i:])
		}
	}
	return ellipsis
}
