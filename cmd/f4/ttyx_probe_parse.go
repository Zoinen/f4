package main

import (
	"strconv"
	"strings"
)

// parseXTWinOps decodes "<prefix>height;width t". The parser is shared by
// Unix probing and its platform-independent regression tests; Windows uses
// no ioctl/query implementation but still builds those tests.
func parseXTWinOps(s, prefix string) (int, int, bool) {
	i := strings.Index(s, prefix)
	if i < 0 {
		return 0, 0, false
	}
	rest := s[i+len(prefix):]
	if j := strings.IndexByte(rest, 't'); j >= 0 {
		rest = rest[:j]
	}
	parts := strings.Split(rest, ";")
	if len(parts) < 2 {
		return 0, 0, false
	}
	h, errH := strconv.Atoi(strings.TrimSpace(parts[0]))
	w, errW := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errH != nil || errW != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func answerComplete(s, prefix string) bool {
	return strings.HasSuffix(s, "t") && strings.Contains(s, prefix)
}
