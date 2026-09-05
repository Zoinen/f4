package main

import "testing"

func TestNestedInputMode(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		nested   bool
		windows  bool
		want     string
	}{
		{name: "nested windows defaults to ansi", nested: true, windows: true, want: "ansi"},
		{name: "explicit conpty wins", explicit: "ConPTY", nested: true, windows: true, want: "ConPTY"},
		{name: "explicit ansi wins", explicit: "ansi", nested: true, windows: true, want: "ansi"},
		{name: "top level windows unchanged", windows: true, want: ""},
		{name: "nested unix unchanged", nested: true, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nestedInputMode(tt.explicit, tt.nested, tt.windows); got != tt.want {
				t.Fatalf("nestedInputMode(%q, %v, %v) = %q, want %q", tt.explicit, tt.nested, tt.windows, got, tt.want)
			}
		})
	}
}
