package svcmgr

import (
	"errors"
	"runtime"
	"strconv"
)

// service is one Win32 service as the panel shows it.
type service struct {
	Name    string
	Display string
	State   uint32 // SERVICE_* current state, see stateName
	PID     uint32 // process id while running, 0 otherwise
	// StartType is the SERVICE_*_START value, startUnknown when it could not
	// be read (no right to query the service).
	StartType uint32
}

// The SERVICE_* current-state values of winsvc.h. They are plain numbers, so
// the mapping below builds and is tested on every OS.
const (
	stateStopped         = 1
	stateStartPending    = 2
	stateStopPending     = 3
	stateRunning         = 4
	stateContinuePending = 5
	statePausePending    = 6
	statePaused          = 7
)

// stateName is the state as text, for the state column: the English words
// FAR's SvcMgr shows. An unknown number is shown as itself.
func stateName(state uint32) string {
	switch state {
	case stateStopped:
		return "Stopped"
	case stateStartPending:
		return "Starting"
	case stateStopPending:
		return "Stopping"
	case stateRunning:
		return "Running"
	case stateContinuePending:
		return "Continuing"
	case statePausePending:
		return "Pausing"
	case statePaused:
		return "Paused"
	}
	return strconv.FormatUint(uint64(state), 10)
}

// errUnsupported is what the collector reports off Windows.
var errUnsupported = errors.New("the service manager is only available on Windows")

// supported reports whether this OS has a Service Control Manager. It is a
// variable so tests can pretend either way.
var supported = runtime.GOOS == "windows"

// Supported reports whether the plugin can list services on this OS.
func Supported() bool { return supported }
