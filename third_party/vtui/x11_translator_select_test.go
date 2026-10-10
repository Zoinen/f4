//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"testing"

	"github.com/jezek/xgb"
	"github.com/unxed/keytrans"
	"github.com/unxed/winkeys"
)

// fakeX11Translator is a minimal keytrans.Translator stand-in so
// newX11Translator can be tested without a real X11 connection or keymap.
type fakeX11Translator struct {
	name string
}

func (f *fakeX11Translator) Name() string { return f.name }
func (f *fakeX11Translator) TranslateX11(detail uint8, state uint16, isDown bool) winkeys.InputEvent {
	return winkeys.InputEvent{}
}
func (f *fakeX11Translator) TranslateWayland(keycode uint32, isDown bool) winkeys.InputEvent {
	return winkeys.InputEvent{}
}
func (f *fakeX11Translator) UpdateWaylandModifiers(modsDepressed, modsLatched, modsLocked, group uint32) {
}
func (f *fakeX11Translator) Close() {}

func TestNewX11Translator_PassesConnDisplayAndWindowIDToKeytrans(t *testing.T) {
	want := &fakeX11Translator{name: "xkbgo-x11"}
	var gotInfo keytrans.OSInfo

	orig := newX11TranslatorFunc
	defer func() { newX11TranslatorFunc = orig }()
	newX11TranslatorFunc = func(info keytrans.OSInfo) keytrans.Translator {
		gotInfo = info
		return want
	}

	t.Setenv("DISPLAY", ":42")

	var conn *xgb.Conn
	got := newX11Translator(conn, 7)

	if got != keytrans.Translator(want) {
		t.Errorf("expected the keytrans translator to be returned, got %v", got)
	}
	if gotInfo.WindowID != 7 {
		t.Errorf("expected WindowID 7 to be passed through, got %d", gotInfo.WindowID)
	}
	if gotInfo.DisplayString != ":42" {
		t.Errorf("expected DisplayString %q to be passed through, got %q", ":42", gotInfo.DisplayString)
	}
	if c, ok := gotInfo.XgbConn.(*xgb.Conn); !ok || c != conn {
		t.Errorf("expected conn to be passed through as XgbConn, got %#v", gotInfo.XgbConn)
	}
}

func TestNewX11Translator_ReturnsNilWhenKeytransFails(t *testing.T) {
	orig := newX11TranslatorFunc
	defer func() { newX11TranslatorFunc = orig }()
	newX11TranslatorFunc = func(info keytrans.OSInfo) keytrans.Translator {
		return nil
	}

	if got := newX11Translator(nil, 0); got != nil {
		t.Errorf("expected nil when keytrans.NewX11Translator fails, got %v", got)
	}
}
