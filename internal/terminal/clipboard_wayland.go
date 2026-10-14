//go:build linux || freebsd || openbsd || netbsd || dragonfly

package terminal

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/unxed/goclip"
)

// goclip writes text on Wayland by piping it to a bare wl-copy, and wl-copy
// with no --type guesses the type from the content. "def test():" comes out
// as text/x-python: f4 itself still reads it back, but browsers, LibreOffice
// and XWayland (xsel, xclip) only take text types, so for them the clipboard
// is empty (nz256, f4#1890). This driver takes the "wayland" slot of goclip's
// registry and says what the text is; reading and clearing stay goclip's.
type waylandTextClipboard struct {
	*goclip.WaylandDriver
}

// waylandTextType is what wl-copy offers. A text/* type makes it offer the
// usual aliases too (text/plain, TEXT, STRING, UTF8_STRING).
const waylandTextType = "text/plain;charset=utf-8"

func (d waylandTextClipboard) WriteText(text string) error {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if path, err := exec.LookPath("wl-copy"); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, path, "--type", waylandTextType) // #nosec G204 -- fixed arguments, the text goes through stdin.
			cmd.Stdin = strings.NewReader(text)
			if cmd.Run() == nil {
				return nil
			}
		}
	}
	return d.WaylandDriver.WriteText(text)
}

func init() {
	goclip.RegisterDriver(waylandTextClipboard{goclip.NewWaylandDriver()})
}
