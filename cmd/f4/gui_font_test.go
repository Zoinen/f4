package main

import "testing"

func TestResolveGuiFont(t *testing.T) {
	tests := []struct {
		name, goos, configured, want string
		useSystem                    bool
	}{
		{name: "Windows system", goos: "windows", configured: "Custom Mono", useSystem: true, want: "Consolas"},
		{name: "macOS system", goos: "darwin", configured: "Custom Mono", useSystem: true, want: "Monaco"},
		{name: "Linux unchanged", goos: "linux", configured: "DejaVu Sans Mono", useSystem: true, want: "DejaVu Sans Mono"},
		{name: "Linux backend default", goos: "linux", configured: "", useSystem: true, want: ""},
		{name: "Custom override", goos: "windows", configured: "JetBrains Mono", useSystem: false, want: "JetBrains Mono"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveGuiFont(test.goos, test.useSystem, test.configured); got != test.want {
				t.Fatalf("resolveGuiFont(%q, %v, %q) = %q, want %q", test.goos, test.useSystem, test.configured, got, test.want)
			}
		})
	}
}

func TestDefaultGuiFontSize(t *testing.T) {
	if got := defaultGuiFontSize("darwin"); got != 17 {
		t.Fatalf("macOS default GUI font size = %d, want 17", got)
	}
	for _, goos := range []string{"windows", "linux", "freebsd"} {
		if got := defaultGuiFontSize(goos); got != 16 {
			t.Fatalf("%s default GUI font size = %d, want 16", goos, got)
		}
	}
}
