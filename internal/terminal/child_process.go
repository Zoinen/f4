package terminal

// ChildProcess is what the session needs to know about a child of the shell.
type ChildProcess struct {
	Name string // image file name, e.g. "PING.EXE"
	GUI  bool   // built for the Windows GUI subsystem: cmd does not wait for it
}
