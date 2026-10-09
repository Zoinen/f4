package app

import "testing"

func TestShouldToggleDirectWindowsTerminalWindow(t *testing.T) {
	for _, tc := range []struct {
		name          string
		activeBackend string
		class         string
		want          bool
	}{
		{name: "default terminal handoff", class: windowsTerminalHostingWindowClass, want: true},
		{name: "gui backend owns its window", activeBackend: "win32", class: windowsTerminalHostingWindowClass},
		{name: "classic console", class: "ConsoleWindowClass"},
		{name: "pseudo helper", class: "PseudoConsoleWindow"},
		{name: "other terminal", class: "Chrome_WidgetWin_1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldToggleDirectWindowsTerminalWindow(tc.activeBackend, tc.class); got != tc.want {
				t.Fatalf("shouldToggleDirectWindowsTerminalWindow(%q, %q) = %v, want %v", tc.activeBackend, tc.class, got, tc.want)
			}
		})
	}
}
