//go:build windows

package terminal

import "github.com/unxed/f4/vfs/hostmode"

// RunLocalCommandInline keeps the shell dialect and the process transport
// together. With winescape enabled the command is a host POSIX process; a
// Windows cmd.exe fallback would make the same command line change language
// merely because PTY allocation failed.
func RunLocalCommandInline(dir, command string) error {
	if hostmode.Posix() {
		if handled, err := runNativeLocalCommandInline(dir, command); handled {
			return err
		}
	}
	return ErrLocalCommandUnavailable
}
