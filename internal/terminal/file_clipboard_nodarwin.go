//go:build !darwin && !windows

package terminal

import "context"

// readDarwinFileClipboard exists so that the shared reader compiles on every
// Unix; the real one is in file_clipboard_darwin.go.
func readDarwinFileClipboard(context.Context) ([]string, bool, error) {
	return nil, false, errFileClipboardUnavailable
}
