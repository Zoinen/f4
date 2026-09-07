package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func graphicsCompatEnv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func newGraphicsCompatScreen() *vtui.ScreenBuf {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	return scr
}

func TestPreferCompatibleGraphicsProtocolUsesKittyInKonsole(t *testing.T) {
	scr := newGraphicsCompatScreen()
	scr.Graphics().SetProtocol(vtui.GraphicsSixel)
	env := graphicsCompatEnv(map[string]string{"KONSOLE_VERSION": "230805"})

	preferCompatibleGraphicsProtocol(scr, env)

	if got := scr.Graphics().Protocol(); got != vtui.GraphicsKitty {
		t.Fatalf("Konsole protocol: got %v, want kitty", got)
	}
}

func TestPreferCompatibleGraphicsProtocolUsesKittyForKnownTerminals(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		from vtui.GraphicsProtocol
	}{
		{name: "kitty", env: map[string]string{"TERM": "xterm-kitty"}, from: vtui.GraphicsSixel},
		{name: "ghostty", env: map[string]string{"TERM_PROGRAM": "ghostty"}, from: vtui.GraphicsSixel},
		{name: "contour", env: map[string]string{"TERM_PROGRAM": "contour"}, from: vtui.GraphicsSixel},
		{name: "wayst", env: map[string]string{"TERM": "wayst"}, from: vtui.GraphicsSixel},
		{name: "rio", env: map[string]string{"TERM": "rio"}, from: vtui.GraphicsNone},
		{name: "warp", env: map[string]string{"TERM_PROGRAM": "WarpTerminal"}, from: vtui.GraphicsNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scr := newGraphicsCompatScreen()
			scr.Graphics().SetProtocol(tc.from)

			preferCompatibleGraphicsProtocol(scr, graphicsCompatEnv(tc.env))

			if got := scr.Graphics().Protocol(); got != vtui.GraphicsKitty {
				t.Fatalf("protocol: got %v, want kitty", got)
			}
		})
	}
}

func TestPreferCompatibleGraphicsProtocolKeepsExplicitProtocol(t *testing.T) {
	for _, forced := range []string{"sixel", "kitty", "none"} {
		t.Run(forced, func(t *testing.T) {
			scr := newGraphicsCompatScreen()
			want, ok := vtui.ParseGraphicsProtocol(forced)
			if !ok {
				t.Fatalf("test protocol %q is invalid", forced)
			}
			scr.Graphics().SetProtocol(want)
			env := graphicsCompatEnv(map[string]string{
				"KONSOLE_VERSION": "230805",
				"VTUI_GRAPHICS":   forced,
			})

			preferCompatibleGraphicsProtocol(scr, env)

			if got := scr.Graphics().Protocol(); got != want {
				t.Fatalf("explicit protocol: got %v, want %v", got, want)
			}
		})
	}
}

func TestPreferCompatibleGraphicsProtocolLeavesOtherTerminalsAlone(t *testing.T) {
	scr := newGraphicsCompatScreen()
	scr.Graphics().SetProtocol(vtui.GraphicsNone)
	env := graphicsCompatEnv(map[string]string{"TERM": "xterm-256color"})

	preferCompatibleGraphicsProtocol(scr, env)

	if got := scr.Graphics().Protocol(); got != vtui.GraphicsNone {
		t.Fatalf("non-Konsole protocol: got %v, want none", got)
	}
}

func TestPreferCompatibleGraphicsProtocolLeavesWindowsTerminalOnSixel(t *testing.T) {
	scr := newGraphicsCompatScreen()
	scr.Graphics().SetProtocol(vtui.GraphicsSixel)
	env := graphicsCompatEnv(map[string]string{"WT_SESSION": "session"})

	preferCompatibleGraphicsProtocol(scr, env)

	if got := scr.Graphics().Protocol(); got != vtui.GraphicsSixel {
		t.Fatalf("Windows Terminal protocol: got %v, want sixel", got)
	}
}

func TestPreferCompatibleGraphicsProtocolLeavesOldKonsoleOnSixel(t *testing.T) {
	scr := newGraphicsCompatScreen()
	scr.Graphics().SetProtocol(vtui.GraphicsSixel)
	env := graphicsCompatEnv(map[string]string{"KONSOLE_VERSION": "220300"})

	preferCompatibleGraphicsProtocol(scr, env)

	if got := scr.Graphics().Protocol(); got != vtui.GraphicsSixel {
		t.Fatalf("old Konsole protocol: got %v, want sixel", got)
	}
}
