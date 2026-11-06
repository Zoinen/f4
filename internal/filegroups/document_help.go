package filegroups

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed highlight.help.ini
var documentHelp string

const (
	documentHelpStart = "# >>> f4 file groups help >>>"
	documentHelpEnd   = "# <<< f4 file groups help <<<"
)

// refreshDocumentHelp changes only a marked, generated block before the first
// section. Unmarked legacy help and personal comments are deliberately retained.
// Called only by Commit, it shares validation, conflict checks and atomic save
// with the edited groups; loading or cancelling never writes a profile.
func refreshDocumentHelp(data []byte) ([]byte, error) {
	text := string(data)
	bom := ""
	if strings.HasPrefix(text, "\ufeff") {
		bom, text = "\ufeff", strings.TrimPrefix(text, "\ufeff")
	}
	start, end, offset := -1, -1, 0
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			break
		}
		switch trimmed {
		case documentHelpStart:
			if start >= 0 || end >= 0 {
				return nil, fmt.Errorf("duplicate generated file-group help block; preserve the draft and repair its markers")
			}
			start = offset
		case documentHelpEnd:
			if start < 0 || end >= 0 {
				return nil, fmt.Errorf("unmatched generated file-group help marker; preserve the draft and repair its markers")
			}
			end = offset + len(line)
		}
		offset += len(line)
	}
	if start >= 0 && end < 0 {
		return nil, fmt.Errorf("unterminated generated file-group help block; preserve the draft and repair its markers")
	}
	if start < 0 {
		return []byte(bom + documentHelp + "\n" + text), nil
	}
	return []byte(bom + text[:start] + documentHelp + text[end:]), nil
}
