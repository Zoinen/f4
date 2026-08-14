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

	InitCore()

	restore, err := vtui.PrepareTerminal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}
	if restore != nil {
		defer restore()
	}

	reader := vtinput.NewReader(os.Stdin, false)
	vtui.FrameManager.Run(reader)
}

func runServer(sockPath string)                {}
func runClient(sockPath string, serverPID int) {}
