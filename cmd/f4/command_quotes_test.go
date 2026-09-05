package main

import "testing"

func TestCommandHasUnmatchedQuote(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		windowsShell bool
		want         bool
	}{
		{name: "posix single quote", command: "echo 'ok'", want: false},
		{name: "posix open single quote", command: "echo 'broken", want: true},
		{name: "posix open double quote", command: `echo "broken`, want: true},
		{name: "posix escaped quote", command: `echo can\'t`, want: false},
		{name: "posix closed backtick", command: "echo `date`", want: false},
		{name: "posix open backtick", command: "echo `date", want: true},
		{name: "posix backtick in single quote", command: "echo '`literal'", want: false},
		{name: "posix escaped backtick", command: "echo \\`literal", want: false},
		{name: "posix backtick inside double quote", command: "echo \"`date\"", want: true},
		{name: "posix backtick inside closed double quote", command: "echo \"`date`\"", want: false},
		{name: "windows open double quote", command: `echo "broken`, windowsShell: true, want: true},
		{name: "windows single quote is literal", command: "echo 'literal", windowsShell: true, want: false},
		{name: "windows backtick is literal", command: "echo `literal", windowsShell: true, want: false},
		{name: "windows caret escaped quote", command: `echo ^"literal`, windowsShell: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := commandHasUnmatchedQuote(test.command, test.windowsShell); got != test.want {
				t.Fatalf("commandHasUnmatchedQuote(%q, windows=%v) = %v, want %v", test.command, test.windowsShell, got, test.want)
			}
		})
	}
}
