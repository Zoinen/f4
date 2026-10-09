package terminal

// InteractiveShellArgs configures the persistent local shell, not commands run
// through shell -c. macOS GUI launches do not read login profiles; let the shell
// initialize its own environment, just as it does in Terminal.app.
// The PTY already makes the shell interactive without forcing -i.
func InteractiveShellArgs() []string {
	return []string{"-l"}
}
