package panel

import (
	"errors"
	"fmt"

	"github.com/unxed/f4/vfs"
)

// isNotListableDirectory reports whether a SetPath error means the target
// directory exists but the host refuses to list it (#814). Such a change is
// refused before the panel moves, so the caller reports it and stays put
// rather than trying another reading of the path.
func isNotListableDirectory(err error) bool {
	var notListable *vfs.NotListableError
	return errors.As(err, &notListable)
}

// noteNotListableDirectory keeps the first unlistable-directory refusal seen
// while several routes to one path are tried.
func noteNotListableDirectory(refused *error, err error) {
	if refused != nil && *refused == nil && isNotListableDirectory(err) {
		*refused = err
	}
}

// reportNotListableDirectory shows the refusal with the message a failed
// directory read shows, without moving the panel.
func (fp *FileSystemPanel) reportNotListableDirectory(err error) {
	fp.showDirectoryError(" Error ", fmt.Sprintf("Cannot access folder:\n%v", err))
}
