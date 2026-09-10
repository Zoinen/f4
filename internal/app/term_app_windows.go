//go:build windows

package app

import (
	"github.com/unxed/f4/internal/media"
)

// The window over the console, for a console that cannot show a picture
// itself — which is conhost, where cmd.exe lives. Windows Terminal renders
// sixel and is left alone.
func (termApplication) InstallImageOverlay() { media.InstallConsoleOverlay() }
