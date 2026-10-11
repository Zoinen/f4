package terminal

import "fmt"

// ManagedForegroundCommand wraps sqCmd -- a command already shell-quoted for
// eval, e.g. via ShellSingleQuote -- in the OSC 133 C/D markers f4 needs in
// order to know when a foreground command run from its own command line has
// finished. See internal/panel/frame.go's composition of fullWireCmd for the
// caller (the "managed foreground command" branch), and
// managed_exec_test.go for what a job-control stop (Ctrl+Z / SIGTSTP) does
// to the D marker printed here -- it never runs, which is the mechanism
// behind f4 #1603's stuck-forever terminal.
func ManagedForegroundCommand(sqCmd string) string {
	return fmt.Sprintf("{ trap \"printf ''\" INT; printf \"\\033]133;C\\007\"; eval %s ; FARVTRESULT=$?; printf \"\\033]133;D\\007\"; trap - INT; (exit $FARVTRESULT); }", sqCmd)
}
