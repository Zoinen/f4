package vtui

import "testing"

func TestKittyGraphicsTerminal(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"kitty by window id", map[string]string{"KITTY_WINDOW_ID": "3"}, true},
		{"kitty by TERM", map[string]string{"TERM": "xterm-kitty"}, true},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, true},
		{"ghostty resources", map[string]string{"GHOSTTY_RESOURCES_DIR": "/x"}, true},
		{"contour", map[string]string{"TERM_PROGRAM": "contour"}, true},
		{"wayst", map[string]string{"TERM": "wayst"}, true},
		{"rio", map[string]string{"TERM": "rio"}, true},
		{"rio versioned", map[string]string{"TERM": "rio-256"}, true},
		{"warp", map[string]string{"TERM_PROGRAM": "WarpTerminal"}, true},
		{"konsole 22.04", map[string]string{"KONSOLE_VERSION": "220400"}, true},
		{"konsole newer", map[string]string{"KONSOLE_VERSION": "230805"}, true},
		{"konsole older", map[string]string{"KONSOLE_VERSION": "210800"}, false},
		{"konsole unparsable", map[string]string{"KONSOLE_VERSION": "x"}, false},
		{"iterm", map[string]string{"TERM_PROGRAM": "iTerm.app"}, false},
		{"plain xterm", map[string]string{"TERM": "xterm-256color"}, false},
		{"nothing", map[string]string{}, false},
		{"wezterm in WSL", map[string]string{"TERM_PROGRAM": "WezTerm", "WSL_DISTRO_NAME": "Ubuntu"}, false},
	}
	for _, tc := range cases {
		env := func(k string) string { return tc.env[k] }
		if got := KittyGraphicsTerminal(env); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
	if KittyGraphicsTerminal(nil) {
		t.Error("a nil environment names no terminal")
	}
}
