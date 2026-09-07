//go:build windows

package main

import "testing"

func TestIconSizesForDPI(t *testing.T) {
	tests := []struct {
		dpi         uint32
		wantSmall   int
		wantTaskbar int
	}{
		{dpi: 96, wantSmall: 16, wantTaskbar: 24},
		{dpi: 168, wantSmall: 28, wantTaskbar: 42},
		{dpi: 192, wantSmall: 32, wantTaskbar: 48},
	}

	for _, tt := range tests {
		small, taskbar := iconSizesForDPI(tt.dpi)
		if small != tt.wantSmall || taskbar != tt.wantTaskbar {
			t.Fatalf("iconSizesForDPI(%d) = (%d, %d), want (%d, %d)",
				tt.dpi, small, taskbar, tt.wantSmall, tt.wantTaskbar)
		}
	}
}

func TestWindowsThemeFromRegistry(t *testing.T) {
	if got := windowsThemeFromRegistry(0); got != windowsThemeDark {
		t.Fatalf("AppsUseLightTheme=0 maps to %v, want dark", got)
	}
	if got := windowsThemeFromRegistry(1); got != windowsThemeLight {
		t.Fatalf("AppsUseLightTheme=1 maps to %v, want light", got)
	}
}
