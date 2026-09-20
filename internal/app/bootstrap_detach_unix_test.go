//go:build !windows

package app

import "testing"

// The redirect exists for the copy checkAndDetach started: no terminal, stdout
// on /dev/null, and a library's dying words worth keeping in the crash log. A
// process that merely inherited the flag has a terminal, and sending its output
// to a file leaves the user staring at a blank screen -- issue #1151, where
// `f4 --version` at the command line of a GUI f4 printed into the outer
// session's crash log.
func TestDetachedStdoutGoesToCrashLog(t *testing.T) {
	cases := []struct {
		name             string
		detached         bool
		stdoutIsTerminal bool
		want             bool
	}{
		{"the detached copy has nowhere else to print", true, false, true},
		{"an inherited flag with a terminal to print on", true, true, false},
		{"an ordinary run in a terminal", false, true, false},
		{"an ordinary run with output in a pipe", false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detachedStdoutGoesToCrashLog(tc.detached, tc.stdoutIsTerminal); got != tc.want {
				t.Errorf("detachedStdoutGoesToCrashLog(%v, %v) = %v, want %v",
					tc.detached, tc.stdoutIsTerminal, got, tc.want)
			}
		})
	}
}
