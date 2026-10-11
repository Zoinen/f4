//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

// This file selects the keyboard translator vtui uses on the X11 backend.
//
// Historically (vtui#10) this file tried a vtui-local pure Go XKB-over-X11
// path (x11_xkb_translator.go, built directly on github.com/unxed/xkb-go/x11)
// before falling back to keytrans.NewX11Translator on failure. That protocol
// path has since moved into keytrans itself as its own backend
// ("xkbgo-x11", see backend_xkbgo_x11.go in github.com/unxed/keytrans) and
// is now the first backend keytrans.NewX11Translator tries on its own, ahead
// of libxkbcommon/XIM/purexkb/etc. vtui no longer needs to duplicate that
// selection logic here: it just calls through to keytrans, which reuses the
// same already-open *xgb.Conn.
import (
	"os"

	"github.com/jezek/xgb"
	"github.com/unxed/keytrans"
)

// newX11TranslatorFunc is keytrans.NewX11Translator, indirected so tests can
// substitute a fake without a real X11 connection.
var newX11TranslatorFunc = keytrans.NewX11Translator

// newX11Translator builds the keyboard translator vtui uses on the X11
// backend, on the given already-open X11 connection.
func newX11Translator(conn *xgb.Conn, windowID uint32) keytrans.Translator {
	tr := newX11TranslatorFunc(keytrans.OSInfo{
		DisplayString: os.Getenv("DISPLAY"),
		XgbConn:       conn,
		WindowID:      windowID,
	})
	if tr != nil {
		DebugLog("X11: using keytrans keyboard translator (backend: %s)", tr.Name())
	}
	return tr
}
