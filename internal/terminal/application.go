package terminal

import (
	"github.com/unxed/vtui"
)

// Application is what the terminal needs from the application above it.
//
// Almost all of it is the Unix session server's doing: a daemon that a client
// attaches to has to rebuild the interface for the new terminal, and the
// interface is not the terminal's to build. The rest is two lookups the
// terminal cannot make on its own — decoding an image it received inline, and
// naming the version it reports to a probe.
type Application interface {
	// InitCore builds the screen buffer the session serves and returns it.
	InitCore() *vtui.ScreenBuf

	// InstallImageOverlay puts the picture window over a terminal that cannot
	// show one itself — an X11 window on Unix, a console overlay on Windows.
	// It runs before the first frame, because every gate on it is asked from
	// inside one.
	InstallImageOverlay()

	// OpenEditFile opens the file named by -e, or does nothing when none was
	// given. The Unix session folds this into ClientAttached, which has an
	// attach to defer it past; the Windows session is already fully up by the
	// time it can be called.
	OpenEditFile()

	// ClientAttached runs the application's part of a client attach, in order:
	// give the host console back to a workspace that had its panels hidden,
	// move that workspace to the directories the client started in, and open
	// the file named by -e. Empty strings skip their step.
	ClientAttached(startLeft, startRight, editPath string)

	// ClientDetached leaves every active host console before the terminal is
	// restored, across all workspaces rather than only the visible one.
	ClientDetached()

	// DecodeImage decodes an image the terminal received inline. The decoders
	// live with the image viewer, which is above this package.
	DecodeImage(data []byte) (*vtui.ImageSurface, error)

	// VersionInfo is the version string the Wine probe reports.
	VersionInfo() string

	// EditFilePath is the file named by -e, or "".
	EditFilePath() string

	// StartupDirs are the directories the process started in.
	StartupDirs() (left, right string)
}

// App is the live application. The composition root sets it once at startup.
var App Application
