package panel

import (
	"strings"
)

// kittyLocalCommandTitleSequence reconstructs the OSC title sequence used by
// KiTTY's internally handled commands.  f4 consumes OSC titles while parsing
// the PTY, so these private titles need to be sent to the outer terminal too.
// All KiTTY local-command extensions use the double-underscore title prefix;
// ordinary shell titles remain internal to f4.
func kittyLocalCommandTitleSequence(title string) []byte {
	if !strings.HasPrefix(title, "__") {
		return nil
	}
	return []byte("\x1b]0;" + title + "\x07")
}
