package keymap

import (
	"fmt"
	"github.com/unxed/vtinput"
	"strconv"
	"strings"
	"unicode"
)

// The Far key-name codec: the spelling f4 reads out of hotkeys.ini, keymap.ini
// and macro definitions, and the event it turns into. It lives here rather than
// with the macro recorder that first needed it because every one of those files
// is a keymap file.

func NormalizeMods(mods vtinput.ControlKeyState) vtinput.ControlKeyState {
	var n vtinput.ControlKeyState
	if mods.Contains(vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed) {
		n |= vtinput.LeftCtrlPressed
	}

	if mods.Contains(vtinput.LeftAltPressed | vtinput.RightAltPressed) {
		n |= vtinput.LeftAltPressed
	}

	if mods.Contains(vtinput.ShiftPressed) {
		n |= vtinput.ShiftPressed
	}

	if mods.Contains(vtinput.EnhancedKey) {
		n |= vtinput.EnhancedKey
	}
	return n
}

var farKeyNames = map[uint16]string{
	vtinput.VK_RETURN:   "Enter",
	vtinput.VK_ESCAPE:   "Esc",
	vtinput.VK_SPACE:    "Space",
	vtinput.VK_TAB:      "Tab",
	vtinput.VK_BACK:     "BS",
	vtinput.VK_INSERT:   "Ins",
	vtinput.VK_DELETE:   "Del",
	vtinput.VK_HOME:     "Home",
	vtinput.VK_END:      "End",
	vtinput.VK_PRIOR:    "PgUp",
	vtinput.VK_NEXT:     "PgDn",
	vtinput.VK_UP:       "Up",
	vtinput.VK_DOWN:     "Down",
	vtinput.VK_LEFT:     "Left",
	vtinput.VK_RIGHT:    "Right",
	vtinput.VK_MULTIPLY: "Multiply",
	vtinput.VK_ADD:      "Add",
	vtinput.VK_SUBTRACT: "Subtract",
	vtinput.VK_DECIMAL:  "Decimal",
	vtinput.VK_DIVIDE:   "Divide",
}

func EventToFarString(e *vtinput.InputEvent) string {
	var sb strings.Builder
	mods := NormalizeMods(e.ControlKeyState)
	if mods.Contains(vtinput.LeftCtrlPressed) {
		sb.WriteString("Ctrl")
	}
	if mods.Contains(vtinput.LeftAltPressed) {
		sb.WriteString("Alt")
	}
	if mods.Contains(vtinput.ShiftPressed) {
		sb.WriteString("Shift")
	}

	vk := e.VirtualKeyCode
	// Windows marks the numeric-keypad Enter as enhanced, while the main
	// keyboard Enter is not enhanced. Delete is the opposite: the navigation
	// cluster key is enhanced and the keypad decimal/delete key is not.
	if mods.Contains(vtinput.EnhancedKey) && vk == vtinput.VK_RETURN {
		sb.WriteString("NumEnter")
		return sb.String()
	}
	if !mods.Contains(vtinput.EnhancedKey) {
		if vk == vtinput.VK_DELETE {
			sb.WriteString("NumDel")
			return sb.String()
		}
	}

	if name, ok := farKeyNames[vk]; ok {
		sb.WriteString(name)
	} else if vk >= vtinput.VK_F1 && vk <= vtinput.VK_F24 {
		fmt.Fprintf(&sb, "F%d", vk-vtinput.VK_F1+1)
	} else if unicode.IsLetter(e.Char) || unicode.IsDigit(e.Char) {
		// Terminal readers expose non-Latin input as a text-only event, while
		// Win32 supplies both the physical VK and the translated character.
		// The translated rune is the only stable identity for a Cyrillic
		// shortcut, so prefer it whenever it is a Unicode letter or digit.
		sb.WriteRune(unicode.ToUpper(e.Char))
	} else if vk >= 'A' && vk <= 'Z' {
		// Hotkey key strings are always uppercase for A-Z ("CtrlV",
		// "ShiftA"). Wayland/X11 gui backends deliver Ctrl+letter events
		// with Char set to the lowercase typed letter — writing e.Char
		// here would produce "Ctrlv" and miss every default binding
		// (Ctrl+V paste, Ctrl+A select-all, Ctrl+O toggle-panels, …).
		sb.WriteRune(rune(vk))
	} else if vk >= '0' && vk <= '9' {
		sb.WriteRune(rune(vk))
	} else if e.Char > 32 && e.Char < 127 {
		sb.WriteRune(e.Char)
	} else {
		fmt.Fprintf(&sb, "VK_%X", vk)
	}
	return sb.String()
}

// vkSpelledHotkeys lists the punctuation keys that configurable hotkeys name
// after their virtual-key code ("VK_DC" for backslash) rather than after the
// character they type. It is the same set keyTokenDisplayNames renders back
// into punctuation for the UI, and the set DefaultKeys already uses:
// CtrlVK_DC (Panel.GoRoot), CtrlShiftVK_DC (Panel.Bookmarks), CtrlVK_DB and
// CtrlVK_DD (bracket navigation).
//
// EventToFarString names a key after e.Char whenever the backend fills that
// field in, and for these keys the character depends on Shift and on the
// active layout. Under the kitty keyboard protocol, which f4 turns on with
// its alternate-key reporting flag, Ctrl+\ arrives as VK_OEM_5 with Char '\'
// and Ctrl+Shift+\ as the same VK_OEM_5 with Char '|', so the two produced
// "Ctrl\" and "CtrlShift|" and missed every VK_DC binding. The far2l, Win32
// and legacy-tty backends send no Char with Ctrl held and did match. Naming
// these keys after the virtual key gives one spelling on every backend and
// every layout.
var vkSpelledHotkeys = map[uint16]bool{
	vtinput.VK_OEM_1:      true, // ;
	vtinput.VK_OEM_PLUS:   true, // =
	vtinput.VK_OEM_COMMA:  true, // ,
	vtinput.VK_OEM_MINUS:  true, // -
	vtinput.VK_OEM_PERIOD: true, // .
	vtinput.VK_OEM_2:      true, // /
	vtinput.VK_OEM_3:      true, // `
	vtinput.VK_OEM_4:      true, // [
	vtinput.VK_OEM_5:      true, // \
	vtinput.VK_OEM_6:      true, // ]
	vtinput.VK_OEM_7:      true, // '
	vtinput.VK_OEM_102:    true, // \ on 102-key keyboards
}

// EventToHotkeyString preserves an otherwise normalized Right Ctrl modifier
// for configurable hotkeys, and spells the punctuation keys in
// vkSpelledHotkeys after their virtual key. Macros intentionally keep
// treating Left Ctrl and Right Ctrl as the same key and keep naming keys the
// way Far does, while actions such as AI.TogglePanel may be rebound or
// explicitly unbound on the RCtrl spelling.
func EventToHotkeyString(e *vtinput.InputEvent) string {
	key := EventToFarString(e)
	if vkSpelledHotkeys[e.VirtualKeyCode] && e.Char != 0 {
		// Re-run the naming without the character so the modifiers,
		// and only the modifiers, keep coming from one place.
		withoutChar := *e
		withoutChar.Char = 0
		key = EventToFarString(&withoutChar)
	}
	mods := e.ControlKeyState
	if mods.Contains(vtinput.RightCtrlPressed) && !mods.Contains(vtinput.LeftCtrlPressed) && strings.HasPrefix(key, "Ctrl") {
		return "RCtrl" + key[len("Ctrl"):]
	}
	return key
}

// keymap.ConfiguredHotkeyAction resolves a hotkey the way Far users expect: Right
// Ctrl is the same modifier as Ctrl unless something is bound on the RCtrl
// spelling specifically. For an "RCtrl…" key the precedence is:
//
//  1. an explicit user binding on the RCtrl spelling to a real action;
//  2. an explicit user binding on the plain Ctrl spelling (None included, so
//     unbinding CtrlA silences both Ctrl+A and Right Ctrl+A);
//  3. the plain Ctrl binding, when the RCtrl spelling was explicitly unbound;
//  4. the built-in RCtrl default (e.g. the RCtrlA AI shortcut);
//  5. the plain Ctrl binding.
//
// Step 3 is what makes unbinding a built-in Right Ctrl shortcut useful:
// "RCtrlA=None" only removes the RCtrl-specific shortcut, after which Right
// Ctrl+A behaves like Ctrl+A (File.Attributes by default) instead of being
// swallowed as a dead key (#492).
func ParseFarKey(s string) *vtinput.InputEvent {
	e := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true}
	orig := s
	if strings.HasPrefix(s, "RCtrl") {
		e.ControlKeyState |= vtinput.RightCtrlPressed
		s = strings.TrimPrefix(s, "RCtrl")
	} else if strings.HasPrefix(s, "Ctrl") {
		e.ControlKeyState |= vtinput.LeftCtrlPressed
		s = strings.TrimPrefix(s, "Ctrl")
	}
	if strings.HasPrefix(s, "Alt") {
		e.ControlKeyState |= vtinput.LeftAltPressed
		s = strings.TrimPrefix(s, "Alt")
	}
	if strings.HasPrefix(s, "Shift") {
		e.ControlKeyState |= vtinput.ShiftPressed
		s = strings.TrimPrefix(s, "Shift")
	}
	if len(s) == 0 {
		s = orig
	}

	if strings.EqualFold(s, "NumEnter") {
		e.VirtualKeyCode = vtinput.VK_RETURN
		e.Char = '\r'
		e.ControlKeyState |= vtinput.EnhancedKey
		return e
	}
	if strings.EqualFold(s, "NumDel") {
		e.VirtualKeyCode = vtinput.VK_DELETE
		return e
	}

	for vk, name := range farKeyNames {
		if strings.EqualFold(s, name) {
			e.VirtualKeyCode = vk
			if vk == vtinput.VK_RETURN {
				e.Char = '\r'
			}
			if vk == vtinput.VK_SPACE {
				e.Char = ' '
			}
			if vk == vtinput.VK_TAB {
				e.Char = '\t'
			}
			if vk == vtinput.VK_BACK {
				e.Char = '\b'
			}
			if vk == vtinput.VK_INSERT || vk == vtinput.VK_DELETE || vk == vtinput.VK_HOME || vk == vtinput.VK_END ||
				vk == vtinput.VK_PRIOR || vk == vtinput.VK_NEXT || vk == vtinput.VK_UP || vk == vtinput.VK_DOWN ||
				vk == vtinput.VK_LEFT || vk == vtinput.VK_RIGHT {
				e.ControlKeyState |= vtinput.EnhancedKey
			}
			return e
		}
	}

	if len(s) >= 2 && (s[0] == 'F' || s[0] == 'f') {
		if n, err := strconv.Atoi(s[1:]); err == nil && n >= 1 && n <= 24 {
			e.VirtualKeyCode = vtinput.VK_F1 + uint16(n-1)
			return e
		}
	}

	if strings.HasPrefix(s, "VK_") {
		_, _ = fmt.Sscanf(s, "VK_%X", &e.VirtualKeyCode)
		return e
	}

	if len(s) > 0 {
		char := []rune(s)[0]
		e.Char = char
		if char >= 'a' && char <= 'z' {
			e.VirtualKeyCode = uint16(char - 'a' + 'A')
		} else if char >= 'A' && char <= 'Z' {
			e.VirtualKeyCode = uint16(char)
		} else if char >= '0' && char <= '9' {
			e.VirtualKeyCode = uint16(char)
		} else {
			switch char {
			case '.':
				e.VirtualKeyCode = vtinput.VK_OEM_PERIOD
			case ',':
				e.VirtualKeyCode = vtinput.VK_OEM_COMMA
			case '-':
				e.VirtualKeyCode = vtinput.VK_OEM_MINUS
			case '=':
				e.VirtualKeyCode = vtinput.VK_OEM_PLUS
			case '/':
				e.VirtualKeyCode = vtinput.VK_OEM_2
			case '`', '~':
				e.VirtualKeyCode = vtinput.VK_OEM_3
			case '[', '{':
				e.VirtualKeyCode = vtinput.VK_OEM_4
			case '\\', '|':
				e.VirtualKeyCode = vtinput.VK_OEM_5
			case ']', '}':
				e.VirtualKeyCode = vtinput.VK_OEM_6
			case '\'', '"':
				e.VirtualKeyCode = vtinput.VK_OEM_7
			case ';', ':':
				e.VirtualKeyCode = vtinput.VK_OEM_1
			}
		}
	}
	return e
}

// Filter is hooked into FrameManager. Returns true if the event was consumed.
