//go:build windows

package gui

import (
	"testing"
)

func TestMatchWindowsFontFamily(t *testing.T) {
	t.Setenv("WINDIR", `C:\Windows`)
	entries := []fontEntry{
		{base: "Consolas", File: "consola.ttf"},
		{base: "Consolas Bold", File: "consolab.ttf"},
		{base: "Cascadia Mono Regular", File: "CascadiaMono.ttf"},
		{base: "FiraCode Nerd Font Mono Reg", File: "FiraCodeNerdFontMono-Regular.ttf"},
	}
	cases := []struct {
		in   string
		want string
	}{
		{"Consolas", `C:\Windows\Fonts\consola.ttf`},
		{"consolas", `C:\Windows\Fonts\consola.ttf`},
		{"  Consolas  ", `C:\Windows\Fonts\consola.ttf`},
		{"Cascadia Mono", `C:\Windows\Fonts\CascadiaMono.ttf`},
		{"FiraCode Nerd Font Mono", `C:\Windows\Fonts\FiraCodeNerdFontMono-Regular.ttf`},
		{"NoSuchFont", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := matchWindowsFontFamily(c.in, entries); got != c.want {
			t.Errorf("matchWindowsFontFamily(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
