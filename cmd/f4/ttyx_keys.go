package main

// The last hop of issue #662: the key combinations a TTY cannot carry, taken
// from the X server and fed into the same stream every other key arrives on.
//
// A terminal turns Ctrl+Shift+Up and Ctrl+Up into the same three bytes unless
// it implements one of the modern keyboard protocols, and most do not. Where
// vtinput can negotiate the kitty protocol or Win32 input mode it does, and
// none of this is needed. Where it cannot, and the session is a local X one,
// the real key is still available — from the X server, over the head of the
// terminal.
//
// This is on by default, because a file manager whose Ctrl+Enter only works
// after the user has found a setting is a file manager whose Ctrl+Enter does
// not work. far2l does the same and asks nobody.
//
// A grab is still shared state on the X server, so two things bound it. It is
// held only while f4's terminal has the focus — which is exactly when those
// keys are wanted, and no other application is reading the keyboard then
// anyway — and it covers only the combinations a TTY genuinely cannot carry.
// The set is a setting for whoever disagrees about which those are.
//
// The grab is also what keeps the key from arriving twice. A grabbed key does
// not reach the terminal at all, so there is no second, blunter copy of it
// coming up through the TTY: f4 sees Ctrl+Enter and not Ctrl+Enter followed
// by a bare Enter. Selecting key events on the terminal's window instead
// would leave both.

import (
	"strings"
	"sync"

	"github.com/unxed/f4/internal/ttyx"
	"github.com/unxed/vtui"
)

// ttyxKeysyms are the keys that can be named in the configuration. The list is
// deliberately short: it covers what f4 has bindings for and a TTY loses, and
// nothing that a terminal already delivers correctly on its own.
var ttyxKeysyms = map[string]uint32{
	"enter":     0xFF0D,
	"return":    0xFF0D,
	"tab":       0xFF09,
	"backspace": 0xFF08,
	"insert":    0xFF63,
	"delete":    0xFFFF,
	"home":      0xFF50,
	"end":       0xFF57,
	"pageup":    0xFF55,
	"pagedown":  0xFF56,
	"up":        0xFF52,
	"down":      0xFF54,
	"left":      0xFF51,
	"right":     0xFF53,
	"f1":        0xFFBE,
	"f2":        0xFFBF,
	"f3":        0xFFC0,
	"f4":        0xFFC1,
	"f5":        0xFFC2,
	"f6":        0xFFC3,
	"f7":        0xFFC4,
	"f8":        0xFFC5,
	"f9":        0xFFC6,
	"f10":       0xFFC7,
	"f11":       0xFFC8,
	"f12":       0xFFC9,
}

// parseTTYXCombos turns a configuration line such as
//
//	Ctrl+Shift+Up, Alt+Shift+F3, Ctrl+Enter
//
// into the set to ask the X server for. An entry that names nothing we know is
// skipped rather than refused: one typo should not cost the user the rest of
// the list. The names that were skipped come back so that they can be logged.
func parseTTYXCombos(spec string) ([]ttyx.Combo, []string) {
	var out []ttyx.Combo
	var bad []string

	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		var mods ttyx.Modifier
		var keysym uint32
		ok := true
		parts := strings.Split(entry, "+")
		for i, part := range parts {
			name := strings.ToLower(strings.TrimSpace(part))
			if i < len(parts)-1 {
				switch name {
				case "ctrl", "control":
					mods |= ttyx.ModCtrl
				case "shift":
					mods |= ttyx.ModShift
				case "alt", "meta":
					mods |= ttyx.ModAlt
				default:
					ok = false
				}
				continue
			}
			if sym, found := ttyxKeysyms[name]; found {
				keysym = sym
			} else if len([]rune(name)) == 1 {
				// A single character is its own keysym for the
				// Latin block, which is what a letter or a digit
				// in a binding is.
				keysym = uint32([]rune(name)[0])
			} else {
				ok = false
			}
		}
		if !ok || keysym == 0 || mods == 0 {
			// A combination with no modifier is a plain key the
			// terminal delivers perfectly well, and taking it from
			// the desktop would be pure loss.
			bad = append(bad, entry)
			continue
		}
		out = append(out, ttyx.Combo{Keysym: keysym, Mods: mods})
	}
	return out, bad
}

// ttyxKeyboard holds the session for as long as f4 runs.
type ttyxKeyboard struct {
	sess *ttyx.Session
	stop chan struct{}
	once sync.Once
}

// startTTYXKeyboard opens the session, asks for the configured combinations
// and starts forwarding them. It returns nil, quietly, whenever any of that is
// not possible: this is an improvement on a terminal that cannot do better,
// never a requirement.
func startTTYXKeyboard() *ttyxKeyboard {
	if !AppConfig.TTYXKeys {
		return nil
	}
	combos, bad := parseTTYXCombos(AppConfig.TTYXKeyList)
	if len(bad) > 0 {
		vtui.DebugLog("TTYX_KEYS: these could not be read and were skipped: %v", bad)
	}
	if len(combos) == 0 {
		return nil
	}

	sess := sharedTTYXSession()
	if sess == nil {
		return nil
	}
	if err := sess.GrabKeys(combos); err != nil {
		vtui.DebugLog("TTYX_KEYS: %v", err)
		return nil
	}
	// GrabKeys reports what the X server said to each request, because a
	// grab is the one thing here that can be refused for reasons outside
	// f4 — another application holding the same combination, most often a
	// desktop shortcut.
	for _, r := range sess.GrabReport() {
		if r.Err != nil {
			vtui.DebugLog("TTYX_KEYS: keysym %#x mods %d refused: %v", r.Keysym, r.Mods, r.Err)
		} else {
			vtui.DebugLog("TTYX_KEYS: keysym %#x mods %d taken as keycode %d", r.Keysym, r.Mods, r.Code)
		}
	}

	k := &ttyxKeyboard{sess: sess, stop: make(chan struct{})}
	go k.forward()
	vtui.DebugLog("TTYX_KEYS: %d combinations asked for on window %d", len(combos), sess.Window())
	return k
}

// forward moves events from the X session onto the stream the frame manager
// dispatches from, which is the same channel the terminal's own keys arrive
// on: past this point nothing can tell the two apart.
func (k *ttyxKeyboard) forward() {
	keys := k.sess.Keys()
	for {
		select {
		case <-k.stop:
			return
		case ev, ok := <-keys:
			if !ok {
				return
			}
			if ev == nil {
				continue
			}
			ch := vtui.FrameManager.EventChan
			if ch == nil {
				continue
			}
			select {
			case ch <- ev:
			case <-k.stop:
				return
			}
		}
	}
}

// Close gives the grabs back to the desktop.
func (k *ttyxKeyboard) Close() {
	if k == nil {
		return
	}
	k.once.Do(func() {
		close(k.stop)
		// The session belongs to the process, not to the keyboard, so
		// only the grabs go back.
		k.sess.UngrabKeys()
	})
}

// defaultTTYXKeyList is what is asked for when the feature is switched on and
// nothing else is said. Every entry is a combination f4 binds and a plain TTY
// cannot distinguish from a simpler one, and nothing here is a combination a
// desktop is likely to want for itself.
const defaultTTYXKeyList = "Ctrl+Shift+Up, Ctrl+Shift+Down, Ctrl+Shift+Left, Ctrl+Shift+Right, " +
	"Ctrl+Enter, Shift+Enter, Ctrl+Shift+Enter, Ctrl+Tab, Ctrl+Shift+Tab, " +
	"Alt+Shift+F3, Alt+Shift+F4"
