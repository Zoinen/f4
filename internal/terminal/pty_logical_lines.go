package terminal

// LogicalLinePreserver is implemented by a PTY backend whose stream carries
// line boundaries the way a Unix tty does: a line longer than the window
// arrives as one run of text, and only the terminal's own autowrap breaks it.
// Only then is a row's wrap flag a record of what the application wrote, and
// only then may the view reflow the rows it wrapped.
type LogicalLinePreserver interface {
	PreservesLogicalLines() bool
}

// PreservesLogicalLines reports whether p is such a backend. A backend that
// does not say so is assumed not to be.
func PreservesLogicalLines(p PtyBackend) bool {
	if lp, ok := p.(LogicalLinePreserver); ok {
		return lp.PreservesLogicalLines()
	}
	return false
}
