package terminal

import (
	"context"
	"os"
	"os/exec"
)

// setImageClipboard hands the file to AppleScript, which publishes it as
// «class PNGf» — the pasteboard type Preview, Telegram and the browsers read.
func setImageClipboard(ctx context.Context, pngData []byte) error {
	if _, err := exec.LookPath("osascript"); err != nil {
		return ErrImageClipboardUnavailable
	}
	tmp, err := os.CreateTemp("", "f4-clipboard-*.png")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(pngData); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// The path travels as an argument, never inside the script text.
	cmd := exec.CommandContext(ctx, "osascript", // #nosec G204 -- fixed script; the temp path is passed as argv, not interpolated.
		"-e", "on run argv",
		"-e", "set the clipboard to (read (POSIX file (item 1 of argv)) as «class PNGf»)",
		"-e", "end run",
		tmp.Name())
	return cmd.Run()
}
