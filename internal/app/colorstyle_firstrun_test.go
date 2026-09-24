package app

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// The rule behind issue #513's "classic theme on first start in wineconsole and
// in every 16-colour environment". The table is the whole rule: a configured
// style always wins, and otherwise a 16-colour console -- the Win32 console API
// renderer or a 16-colour terminal profile -- takes Classic.
func TestFirstRunColorStyle(t *testing.T) {
	tests := []struct {
		name       string
		configured bool
		backend    string
		profile    vtui.ColorProfile
		want       string
		wantOK     bool
	}{
		{"wineconsole: winapi renderer", false, "winapi", vtui.ColorProfileTrueColor, "Classic", true},
		{"win32 renderer", false, "win32", vtui.ColorProfileTrueColor, "Classic", true},
		{"16-colour terminal profile", false, "ansi", vtui.ColorProfile16, "Classic", true},
		{"16-colour profile with no backend chosen", false, "", vtui.ColorProfile16, "Classic", true},
		{"true-colour ansi terminal keeps the default", false, "ansi", vtui.ColorProfileTrueColor, "", false},
		{"256-colour terminal keeps the default", false, "ansi", vtui.ColorProfile256, "", false},
		{"no backend, true colour keeps the default", false, "", vtui.ColorProfileTrueColor, "", false},
		{"a configured style wins over winapi", true, "winapi", vtui.ColorProfileTrueColor, "", false},
		{"a configured style wins over a 16-colour profile", true, "ansi", vtui.ColorProfile16, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := firstRunColorStyle(tc.configured, tc.backend, tc.profile)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("firstRunColorStyle(configured=%v, %q, %v) = (%q, %v), want (%q, %v)",
					tc.configured, tc.backend, tc.profile, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// Classic must be a style the theme package actually has, or the first start
// would fall back to Modern through ApplyColorStyle's error path.
func TestFirstRunColorStyleNamesARealStyle(t *testing.T) {
	if classicColorStyle != "Classic" {
		t.Fatalf("classicColorStyle = %q; theme/styles/classic.ini names its style Classic", classicColorStyle)
	}
}

func TestApplyFirstRunColorStyle(t *testing.T) {
	old := config.App.ColorStyle
	t.Cleanup(func() { config.App.ColorStyle = old })

	t.Run("nil hook changes nothing", func(t *testing.T) {
		config.App.ColorStyle = "Radiola"
		applyFirstRunColorStyle(nil)
		if config.App.ColorStyle != "Radiola" {
			t.Fatalf("ColorStyle = %q, want Radiola", config.App.ColorStyle)
		}
	})
	t.Run("a hook that declines changes nothing", func(t *testing.T) {
		config.App.ColorStyle = "Radiola"
		applyFirstRunColorStyle(func() (string, bool) { return "", false })
		if config.App.ColorStyle != "Radiola" {
			t.Fatalf("ColorStyle = %q, want Radiola", config.App.ColorStyle)
		}
	})
	t.Run("a hook that names a style sets it", func(t *testing.T) {
		config.App.ColorStyle = "Radiola"
		applyFirstRunColorStyle(func() (string, bool) { return "Classic", true })
		if config.App.ColorStyle != "Classic" {
			t.Fatalf("ColorStyle = %q, want Classic", config.App.ColorStyle)
		}
	})
}
