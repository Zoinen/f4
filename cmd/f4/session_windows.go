//go:build windows

package main

import (
	"fmt"
	"os"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func SupportsBackgrounding() bool {
	return false
}

type SessionInfo struct {
	PID      int
	Title    string
	SockPath string
}

func listSessions() []SessionInfo {
	return nil
}

func runSessionPicker(sessions []SessionInfo) *SessionInfo {
	return nil
}

func ManageSessions() {
	stopWindowAppearanceManager := startWindowsConsoleWindowAppearanceManager()
	defer stopWindowAppearanceManager()

	scr := InitCore()
	PreferCompatibleGraphicsProtocol(scr)

	restore, err := vtui.PrepareTerminal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	if restore != nil {
		defer restore()
	}

	// Unlike the Unix build (session_unix.go's runServer, attach time),
	// there's no separate daemon/client split here to defer this past --
	// this *is* the one and only session, already fully up (panels pushed
	// inside InitCore(), terminal just prepared above), so opening right
	// here is the direct equivalent of that hook. openDashEFileIfRequested
	// itself no-ops when -e wasn't given.
	openDashEFileIfRequested()

	// Ask the terminal what it can draw, if the environment did not say.
	// This must happen here: after PrepareTerminal, so VT output is on and
	// the query is asked rather than printed; before InstallConsoleOverlay,
	// which decides on the answer; and before vtinput's reader exists, which
	// would otherwise swallow the reply as keystrokes.
	probeGraphicsIfUnknown(scr)

	// The window over the console, for a console that cannot show a picture
	// itself — which is conhost, where cmd.exe lives. Windows Terminal
	// renders sixel and is left alone. Before the first frame, because
	// every gate on it is asked from inside one.
	InstallConsoleOverlay()

	reader := vtinput.NewReader(os.Stdin, false)
	vtui.FrameManager.Run(reader)

	// Run() returns in good order when the input channel closes, and the
	// input channel closes when the console host has died (#397). Say so,
	// or the log just stops after the session is saved.
	if cause := consoleHostGone(); cause != nil {
		reportConsoleHostGone(cause)
	}
}

func runServer(sockPath string)                {}
func runClient(sockPath string, serverPID int) {}
