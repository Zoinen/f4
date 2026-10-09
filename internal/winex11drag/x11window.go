package winex11drag

// WineX11PropName is the window property winex11.drv sets on every
// top-level HWND it owns, holding the Xlib Window ID of that window on the
// host's X server (issue #566, step 1; see
// https://forum.winehq.org/viewtopic.php?p=138438). GetPropA(hwnd,
// WineX11PropName) is how this is read back; see x11window_windows.go for
// that call, which only exists on GOOS=windows -- there is nothing to look
// up on a native Linux build, which talks to X11 directly through vtui's
// own backend instead of through Wine.
const WineX11PropName = "__wine_x11_whole_window"

// wineX11WindowFromProp turns the raw value GetPropA returns for
// WineX11PropName into a Window ID and an ok flag. It is kept separate from
// the actual syscall (x11window_windows.go) so this one piece of step 1's
// logic -- "zero means not applicable" -- builds and is tested on every
// platform, not only on GOOS=windows.
//
// GetPropA answers 0 both when winex11.drv never set the property (real
// Windows, a Win32 backend on native Linux, or an HWND that never went
// through winex11.drv's window-creation path) and, per the Win32 API
// contract, "no such property" cannot be told apart from "property value
// 0". That ambiguity is harmless here: 0 is not a valid Xlib Window ID
// either (it names "None" in the X11 protocol), so the normal case and the
// one useful answer never collide.
func wineX11WindowFromProp(raw uintptr) (xid uintptr, ok bool) {
	return raw, raw != 0
}
