package panel

import "testing"

func TestKittyLocalCommandTitleSequence(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "command", title: "__cm:calc", want: "\x1b]0;__cm:calc\x07"},
		{name: "other local command", title: "__ie:https://example.com", want: "\x1b]0;__ie:https://example.com\x07"},
		{name: "ordinary title", title: "user@host", want: ""},
		{name: "empty title", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(kittyLocalCommandTitleSequence(tt.title)); got != tt.want {
				t.Fatalf("kittyLocalCommandTitleSequence(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}
