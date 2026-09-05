package main

import "testing"

func TestSudoDispatcherPath(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "separate value", args: []string{"--sudo-dispatcher", "/tmp/f4.sock"}, want: "/tmp/f4.sock"},
		{name: "equals value", args: []string{"--sudo-dispatcher=/tmp/f4.sock"}, want: "/tmp/f4.sock"},
		{name: "missing value", args: []string{"--sudo-dispatcher"}},
		{name: "unrelated arguments", args: []string{"--debug", "--tty", "ansi"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := sudoDispatcherPath(test.args); got != test.want {
				t.Fatalf("sudoDispatcherPath(%q) = %q, want %q", test.args, got, test.want)
			}
		})
	}
}

func TestSudoStartupModeDispatcherWinsOverAskpass(t *testing.T) {
	dispatcher, askpass := sudoStartupMode([]string{"--sudo-dispatcher", "/tmp/f4.sock"}, true)
	if dispatcher != "/tmp/f4.sock" || askpass {
		t.Fatalf("sudoStartupMode dispatcher+askpass = (%q, %v), want (%q, false)", dispatcher, askpass, "/tmp/f4.sock")
	}

	dispatcher, askpass = sudoStartupMode(nil, true)
	if dispatcher != "" || !askpass {
		t.Fatalf("sudoStartupMode askpass = (%q, %v), want (empty, true)", dispatcher, askpass)
	}
}
