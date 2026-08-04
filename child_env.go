package main

// The environment the built-in terminal hands to the program it starts. This
// is where the terminal says what it can do: a program that draws pictures
// picks its protocol long before it prints anything, and it picks by looking
// at these variables.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/unxed/vtui"
)

// kittyGraphicsEnv is the variable kitty exports and image tools look for.
const kittyGraphicsEnv = "KITTY_WINDOW_ID"

// kittyTermName is the terminal type a program looks up when it wants to
// know whether it may draw pictures. Tools written before terminals could be
// asked directly, chafa 1.14 among them, know no other way to find out.
const kittyTermName = "xterm-kitty"

// terminalGraphicsSeen remembers that the screen f4 draws on can show
// images. A shell can be started before the first frame is drawn, and one
// that missed the news would keep the wrong environment for its whole life.
var terminalGraphicsSeen atomic.Bool

// terminalChildEnv builds the environment of a program started in the
// built-in terminal.
func terminalChildEnv() []string {
	graphics := terminalShowsImages()
	return buildChildEnv(os.Environ(), graphics, graphics && announceKittyTerm())
}

// buildChildEnv is the half of terminalChildEnv that depends on nothing but
// its arguments.
func buildChildEnv(env []string, graphics, kittyTerm bool) []string {
	out := make([]string, 0, len(env)+4)
	for _, kv := range env {
		// Whatever we inherited describes the terminal that started f4; the
		// program we are about to start talks to us instead.
		if strings.HasPrefix(kv, kittyGraphicsEnv+"=") || strings.HasPrefix(kv, "TERM_PROGRAM=") {
			continue
		}
		if kittyTerm && strings.HasPrefix(kv, "TERM=") {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "F4_NESTED=1", "TERM_PROGRAM=f4")

	if graphics {
		// The built-in terminal speaks the kitty graphics protocol, so it
		// says so the way kitty itself does. Claiming it while the screen
		// f4 draws on cannot show a picture would only make programs
		// produce output nobody sees.
		out = append(out, kittyGraphicsEnv+"=1")
	}
	if kittyTerm {
		out = append(out, "TERM="+kittyTermName)
	}
	return out
}

// announceKittyTerm decides whether to introduce the built-in terminal as
// kitty. The description has to be installed: a TERM the system cannot look
// up breaks every program that opens the terminfo database, which is a far
// worse trade than a picture drawn with characters.
func announceKittyTerm() bool {
	if !AppConfig.AnnounceKittyTerm {
		return false
	}
	return terminfoExists(kittyTermName)
}

// terminalShowsImages reports whether the screen f4 draws on can display
// images at all.
func terminalShowsImages() bool {
	if scr := vtui.FrameManager.Screen(); scr != nil {
		graphics := scr.SupportsGraphics()
		terminalGraphicsSeen.Store(graphics)
		return graphics
	}
	return terminalGraphicsSeen.Load()
}

// terminfoExists reports whether the compiled description of a terminal is
// installed.
func terminfoExists(name string) bool {
	if name == "" {
		return false
	}
	// An entry lives either under the first letter of its name or under the
	// hexadecimal code of that letter, depending on the file system the
	// database was built for.
	subdirs := []string{name[:1], strconv.FormatUint(uint64(name[0]), 16)}
	for _, dir := range terminfoDirs() {
		for _, sub := range subdirs {
			st, err := os.Stat(filepath.Join(dir, sub, name))
			if err == nil && st.Mode().IsRegular() {
				return true
			}
		}
	}
	return false
}

// terminfoDirs lists the places ncurses looks for terminal descriptions, in
// the order it looks at them.
func terminfoDirs() []string {
	var dirs []string
	if v := os.Getenv("TERMINFO"); v != "" {
		dirs = append(dirs, v)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, ".terminfo"))
	}
	for _, v := range filepath.SplitList(os.Getenv("TERMINFO_DIRS")) {
		if v == "" {
			// An empty entry stands for the built in default.
			v = "/usr/share/terminfo"
		}
		dirs = append(dirs, v)
	}
	return append(dirs, "/etc/terminfo", "/lib/terminfo", "/usr/share/terminfo",
		"/usr/lib/terminfo", "/usr/local/share/terminfo")
}
