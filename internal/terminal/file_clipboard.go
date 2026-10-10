package terminal

import (
	"context"
	"errors"
	"time"

	"github.com/unxed/vtui"
)

var errFileClipboardUnavailable = errors.New("file clipboard is unavailable")

// The native file-list clipboard (CF_HDROP on Windows, the file managers'
// URI-list targets elsewhere) is reached through these two variables so that
// a test run can switch it off. It is a clipboard shared by the whole desktop
// session, not by the process: a test that copies files wrote them to the
// runner's real clipboard, and a paste test of another test binary running
// at the same time then read those files back instead of the image or text
// it had stubbed in (the flaky TestPanelPasteHotkeysAndActionDispatch on the
// Windows runners).
var (
	readNativeFileClipboard = readFileClipboard
	setNativeFileClipboard  = setFileClipboard
)

// DisableSystemFileClipboard cuts the file-list clipboard off from the
// system: setting it does nothing and reading it finds no files. Test
// binaries call it from their TestMain; it is not meant for the program.
func DisableSystemFileClipboard() {
	readNativeFileClipboard = func(context.Context) ([]string, bool, error) {
		return nil, false, errFileClipboardUnavailable
	}
	setNativeFileClipboard = func(context.Context, string, []string, bool) error {
		return errFileClipboardUnavailable
	}
}

// SetF4FileClipboard publishes both the existing text representation and the
// native file representation. The text copy remains the fallback for
// terminals and clipboard providers which do not understand files.
func SetF4FileClipboard(text string, paths []string, cut bool) {
	SetF4Clipboard(text)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := setNativeFileClipboard(ctx, text, paths, cut); err != nil {
		vtui.DebugLog("clipboard files: %v", err)
	}
}
