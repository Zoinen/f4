package winex11drag

import (
	"fmt"
	"strconv"
	"strings"
)

// DisplaySpec is a parsed X11 DISPLAY string of the form
// "[hostname]:display[.screen]".
//
// Examples:
//
//	":0"         -> {Hostname: "",       Display: 0, Screen: 0, HasScreen: false}
//	":1.2"       -> {Hostname: "",       Display: 1, Screen: 2, HasScreen: true}
//	"myhost:0"   -> {Hostname: "myhost", Display: 0}
//	"myhost:0.1" -> {Hostname: "myhost", Display: 0, Screen: 1, HasScreen: true}
type DisplaySpec struct {
	Hostname  string
	Display   int
	Screen    int
	HasScreen bool
}

// ParseDisplay parses an X11 DISPLAY string of the form "[hostname]:N[.S]" --
// exactly what winescape.HostGetenv("DISPLAY") hands back under Wine (issue
// #566, step 2). A colon separates an optional hostname from the mandatory
// display number, and an optional dot-separated screen number follows it.
func ParseDisplay(display string) (DisplaySpec, error) {
	colon := strings.LastIndex(display, ":")
	if colon < 0 {
		return DisplaySpec{}, fmt.Errorf("winex11drag: DISPLAY %q has no %q separating hostname from display", display, ":")
	}
	hostname := display[:colon]
	rest := display[colon+1:]
	if rest == "" {
		return DisplaySpec{}, fmt.Errorf("winex11drag: DISPLAY %q has no display number after %q", display, ":")
	}

	displayPart := rest
	screenPart := ""
	hasScreen := false
	if dot := strings.IndexByte(rest, '.'); dot >= 0 {
		displayPart = rest[:dot]
		screenPart = rest[dot+1:]
		hasScreen = true
	}

	displayNum, err := strconv.Atoi(displayPart)
	if err != nil || displayNum < 0 {
		return DisplaySpec{}, fmt.Errorf("winex11drag: DISPLAY %q has an invalid display number %q", display, displayPart)
	}

	spec := DisplaySpec{Hostname: hostname, Display: displayNum}
	if hasScreen {
		screenNum, err := strconv.Atoi(screenPart)
		if err != nil || screenNum < 0 {
			return DisplaySpec{}, fmt.Errorf("winex11drag: DISPLAY %q has an invalid screen number %q", display, screenPart)
		}
		spec.Screen = screenNum
		spec.HasScreen = true
	}
	return spec, nil
}

// IsLocal reports whether this display names the local host's X server over
// a Unix-domain socket rather than a remote one over the network. An empty
// hostname (":0") and the explicit "unix" pseudo-host ("unix:0") both mean
// local; anything else is a hostname this bootstrap would have to dial over
// TCP, which is out of scope here -- see SocketPath.
func (d DisplaySpec) IsLocal() bool {
	return d.Hostname == "" || d.Hostname == "unix"
}

// SocketPath returns the Unix-domain socket path a local X server listens on
// for this display: "/tmp/.X11-unix/X" followed by the display number,
// exactly as issue #566's step 2 specifies. ok is false for a non-local
// display (see IsLocal): dialing a remote X server over TCP is not part of
// this bootstrap.
func (d DisplaySpec) SocketPath() (string, bool) {
	if !d.IsLocal() {
		return "", false
	}
	return "/tmp/.X11-unix/X" + strconv.Itoa(d.Display), true
}
