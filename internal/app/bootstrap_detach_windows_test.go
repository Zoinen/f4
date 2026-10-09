//go:build windows

package app

import "testing"

// Under Wine the detached GUI copy still had the launching terminal as its
// Unix stderr, and Wine's own messages filled a terminal nobody read until
// the window hung (issue #474). Only that copy loses the descriptor: a
// process that merely inherited F4_DETACHED has a terminal of its own, where
// Wine's messages belong.
func TestDetachedWineStderrGoesToNull(t *testing.T) {
	cases := []struct {
		name             string
		detached         bool
		stdoutIsTerminal bool
		want             bool
	}{
		{"the detached copy, stdout on /dev/null", true, false, true},
		{"an inherited flag with a terminal to print on", true, true, false},
		{"an ordinary run in a terminal", false, true, false},
		{"an ordinary run with output in a pipe", false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detachedWineStderrGoesToNull(tc.detached, tc.stdoutIsTerminal); got != tc.want {
				t.Errorf("detachedWineStderrGoesToNull(%v, %v) = %v, want %v",
					tc.detached, tc.stdoutIsTerminal, got, tc.want)
			}
		})
	}
}
