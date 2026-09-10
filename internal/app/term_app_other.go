//go:build !windows

package app

import (
	"github.com/unxed/f4/internal/media"
)

// The X11 window over the terminal, for a terminal that cannot show a picture
// itself. See docs on the image viewer's last resort.
func (termApplication) InstallImageOverlay() { media.InstallX11Overlay() }
