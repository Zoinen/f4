//go:build windows

package main

import "testing"

func TestMatchWindowsFontFamily(t *testing.T) {
	entries := []fontEntry{
		{base: "Consolas", file: "consola.ttf"},
		{base: "Consolas Bold", file: "consolab.ttf"},
		{base: "Cascadia Mono Regular", file: "CascadiaMono.ttf"},
		{base: "FiraCode Nerd Font Mono Reg", file: "FiraCodeNerdFontMono-Regular.ttf"},
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
