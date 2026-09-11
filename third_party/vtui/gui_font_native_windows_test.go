//go:build windows

package vtui

import "testing"

func TestGUIFontFamilyForPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{`C:\Windows\Fonts\seguisym.ttf`, "Segoe UI Symbol"},
		{`C:\Users\test\AppData\Local\Microsoft\Windows\Fonts\estre.ttf`, "Estrangelo Edessa"},
		{`C:\Windows\Fonts\unknown.ttf`, ""},
	}
	for _, test := range tests {
		if got := guiFontFamilyForPath(test.path); got != test.want {
			t.Errorf("guiFontFamilyForPath(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}
