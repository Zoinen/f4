package dialog

import (
	"strings"
)

// A vtui caption reads a single & as the hotkey marker, so a name that
// contains one — a file, a bookmark, a user-menu entry — has to double it
// before it is shown. Every surface that puts user text in a menu needs
// this, which is why it is here rather than beside any one of them.

// escapeAmpersand doubles literal '&' so vtui doesn't treat them as
// hotkey markers in the label portion.
func EscapeAmpersand(s string) string {
	return strings.ReplaceAll(s, "&", "&&")
}
