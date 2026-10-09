//go:build !windows && !darwin

package terminal

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

// setImageClipboard goes through the same command-line tools the file
// clipboard uses (file_clipboard_unix.go), offering the bytes as image/png.
func setImageClipboard(ctx context.Context, pngData []byte) error {
	var name string
	var args []string
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		name, args = "wl-copy", []string{"--type", "image/png"}
	case os.Getenv("DISPLAY") != "":
		name, args = "xclip", []string{"-selection", "clipboard", "-t", "image/png", "-i"}
	default:
		return ErrImageClipboardUnavailable
	}
	if _, err := exec.LookPath(name); err != nil {
		return ErrImageClipboardUnavailable
	}
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- name and arguments come from the fixed table above, never from input.
	cmd.Stdin = bytes.NewReader(pngData)
	return cmd.Run()
}
