package gui

// Running records the renderer family selected for the process that owns the
// current panels frame. It is deliberately independent of vtui's active
// backend: a TTY session can coexist with a display server, and a Unix f4
// client can attach to a daemon that was started in a different process.
var Running bool

// WithRuntime keeps the mode explicit for the whole lifetime of a GUI host,
// including setup and any editor action handled by that host.
func WithRuntime(run func() error) (err error) {
	previous := Running
	Running = true
	defer func() { Running = previous }()
	return run()
}
