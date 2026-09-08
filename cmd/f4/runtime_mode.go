package main

// runningGUI records the renderer family selected for the process that owns
// the current PanelsFrame. It is deliberately independent of vtui's active
// backend: a TTY session can coexist with a display server, and a Unix f4
// client can attach to a daemon that was started in a different process.
var runningGUI bool

// withGUIRuntime keeps the mode explicit for the whole lifetime of a GUI
// host, including setup and any editor action handled by that host.
func withGUIRuntime(run func() error) (err error) {
	previous := runningGUI
	runningGUI = true
	defer func() { runningGUI = previous }()
	return run()
}
