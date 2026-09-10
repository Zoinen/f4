package keymap

import (
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// GlobalKeyRemap holds the user's input remapping table, read from keymap.ini
// in the profile directory. It stays nil until the profile is located, and an
// absent or empty file leaves every keystroke untouched.
var GlobalKeyRemap *KeyRemap

const (
	// keyRemapWildcard ends both sides of a rule that rewrites a modifier
	// prefix ("CtrlAlt*=Ctrl*") instead of one whole key.
	keyRemapWildcard = "*"
	// keyRemapCommonSection applies to every area, exactly like the Common
	// section of hotkeys.ini.
	keyRemapCommonSection = "Common"
)

// keyRemapPrefixRule rewrites the leading modifiers of a key spelling. from is
// stored lower-cased for matching, to in the canonical spelling ParseFarKey
// expects.
type keyRemapPrefixRule struct {
	from string
	to   string
}

// KeyRemap substitutes one key for another before anything else in f4 looks
// at the event. It exists for terminals f4 does not have to itself: a
// multiplexer (tmux, zellij, screen, dvtm) claims its own chords upstream, so
// keys such as Ctrl+B, Ctrl+A, Ctrl+O or Ctrl+P may never arrive, and small
// keyboards have no F-row to send F1-F12 with at all. Both are the same
// problem seen from the input side, and the hotkey manager cannot solve
// either one on its own: it binds actions, while frameworks, dialogs, menus
// and the frames themselves also read keys directly.
//
// It is not the only answer to a lost chord: the command palette (CtrlShiftP)
// runs any command by name and needs no configuration. A rule is for the key
// itself — one modifier for everything, an F-row that does not exist, or a key
// a dialog reads on its own.
//
// Sections are area names, as in hotkeys.ini; Common applies everywhere. A
// rule maps the spelling shown in the Hotkey Configurator to the spelling f4
// should see instead:
//
//	[Common]
//	CtrlAltO=CtrlO    ; Ctrl+Alt+O now toggles the panels
//	Alt1=F1           ; a keyboard without an F-row
//	CtrlAlt*=Ctrl*    ; every Ctrl chord also answers to Ctrl+Alt
//
// Both sides pass through canonicalKeySpelling, so the modifier order, the
// casing and the several names a terminal may give a shifted key all collapse
// onto one form before anything is stored or looked up.
//
// Substitution happens once per keystroke: the result is never fed back
// through the table, so rules cannot chain or loop.
type KeyRemap struct {
	// Exact maps an area to lower-cased source spellings and their canonical
	// replacements.
	Exact map[string]map[string]string
	// Prefix holds the wildcard rules of an area, longest source first so
	// that CtrlAltShift* wins over CtrlAlt*.
	Prefix map[string][]keyRemapPrefixRule

	IniPath string
}

// NewKeyRemap reads keymap.ini. A missing file is not an error: it yields an
// empty table that Apply short-circuits on.
func NewKeyRemap(iniPath string) *KeyRemap {
	kr := &KeyRemap{IniPath: iniPath}
	kr.Load()
	return kr
}

// Load re-reads the table from disk, discarding what was held before.
func (kr *KeyRemap) Load() {
	if kr == nil {
		return
	}
	kr.Exact = make(map[string]map[string]string)
	kr.Prefix = make(map[string][]keyRemapPrefixRule)
	if kr.IniPath == "" {
		return
	}
	ini := ini.Load(kr.IniPath)
	for area, rules := range ini.Sections() {
		for source, target := range rules {
			kr.addRule(area, source, target)
		}
	}
	for area := range kr.Prefix {
		rules := kr.Prefix[area]
		sort.SliceStable(rules, func(i, j int) bool {
			if len(rules[i].from) != len(rules[j].from) {
				return len(rules[i].from) > len(rules[j].from)
			}
			return rules[i].from < rules[j].from
		})
		kr.Prefix[area] = rules
	}
}

// IsEmpty reports whether the table can be skipped entirely. Every keystroke
// passes through this check, so the common case of no keymap.ini must cost
// nothing.
func (kr *KeyRemap) IsEmpty() bool {
	return kr == nil || (len(kr.Exact) == 0 && len(kr.Prefix) == 0)
}

// stripIniComment removes a trailing ";" or "#" note from one side of a rule.
// f4's INI reader has no notion of comments, so "Alt1=F1 ; no F-row here"
// reaches us with the note still attached, and ParseFarKey would then read
// "F1 ; no F-row here" as the letter F rather than as a function key. A marker
// only starts a comment at the beginning of the field or after whitespace, so
// a rule can still name ";" or "#" as the key itself.
func stripIniComment(s string) string {
	s = strings.TrimSpace(s)
	if s == ";" || s == "#" {
		return s
	}
	for i := 0; i < len(s); i++ {
		if s[i] != ';' && s[i] != '#' {
			continue
		}
		if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}

func (kr *KeyRemap) addRule(area, source, target string) {
	area = strings.TrimSpace(area)
	// A commented-out sample line arrives here with its marker attached, and
	// a live one may carry an explanatory note; both are handled by the same
	// rule, so the shipped keymap.ini can stay self-documenting.
	source = stripIniComment(source)
	target = stripIniComment(target)
	if area == "" || source == "" {
		return
	}

	sourceWild := strings.HasSuffix(source, keyRemapWildcard)
	targetWild := strings.HasSuffix(target, keyRemapWildcard)
	if sourceWild != targetWild {
		// One side alone is meaningless: "Ctrl*=F1" would collapse every Ctrl
		// chord onto a single key, and "CtrlB=Ctrl*" names no key at all.
		return
	}

	if sourceWild {
		from := strings.ToLower(canonicalKeySpelling(strings.TrimSuffix(source, keyRemapWildcard)))
		to := canonicalKeySpelling(strings.TrimSuffix(target, keyRemapWildcard))
		if from == "" || strings.EqualFold(from, to) {
			// An empty source prefix would match every key, and a prefix that
			// rewrites to itself is a no-op.
			return
		}
		kr.Prefix[area] = append(kr.Prefix[area], keyRemapPrefixRule{from: from, to: to})
		return
	}

	if target == "" {
		return
	}
	source = canonicalKeySpelling(source)
	target = canonicalKeySpelling(target)
	if strings.EqualFold(source, target) {
		return
	}
	if kr.Exact[area] == nil {
		kr.Exact[area] = make(map[string]string)
	}
	kr.Exact[area][strings.ToLower(source)] = target
}

// keyRemapShiftedChars maps every character the US layout produces with Shift
// held back to the key that was actually pressed.
//
// A terminal that speaks neither the kitty keyboard protocol nor win32 input
// mode has no way to report Shift separately for a printable key: Shift+1
// arrives as a bare "!" and Alt+Shift+1 as ESC "!", so EventToFarString names
// them "!" and "Alt!". Under kitty the same keys come back as "Shift!" and
// "AltShift!", because the shifted rune is what the protocol reports. None of
// those spellings is what a user writes, and multiplexers routinely strip the
// protocol negotiation, so the spelling changes underneath a working
// keymap.ini. Folding the shifted character back onto its key gives one
// spelling — "Shift1" — that names the chord everywhere, which matters most
// for the Shift F-row substitutes this file exists for.
var keyRemapShiftedChars = map[rune]string{
	'~': "`", '!': "1", '@': "2", '#': "3", '$': "4", '%': "5",
	'^': "6", '&': "7", '*': "8", '(': "9", ')': "0",
	'_': "-", '+': "=", '{': "[", '}': "]", '|': "\\",
	':': ";", '"': "'", '<': ",", '>': ".", '?': "/",
}

// canonicalKeySpelling rewrites a hand-written rule, or the spelling of a live
// event, into one normal form: the casing ParseFarKey and EventToFarString
// use, the modifier order EventToFarString writes, and the unshifted name of a
// shifted character. "ctrlaltf5", "AltCtrlF5" and "CtrlAltF5" therefore mean
// the same thing, and so do "AltShift1", "Alt!" and "AltShift!".
func canonicalKeySpelling(key string) string {
	rest := strings.TrimSpace(key)
	var rctrl, ctrl, alt, shift bool
	for rest != "" {
		matched := true
		switch {
		case len(rest) >= 5 && strings.EqualFold(rest[:5], "RCtrl"):
			rctrl, rest = true, rest[5:]
		case len(rest) >= 4 && strings.EqualFold(rest[:4], "Ctrl"):
			ctrl, rest = true, rest[4:]
		case len(rest) >= 3 && strings.EqualFold(rest[:3], "Alt"):
			alt, rest = true, rest[3:]
		case len(rest) >= 5 && strings.EqualFold(rest[:5], "Shift"):
			shift, rest = true, rest[5:]
		default:
			matched = false
		}
		if !matched {
			break
		}
	}

	token := canonicalKeyToken(rest)
	if runes := []rune(token); len(runes) == 1 {
		if base, ok := keyRemapShiftedChars[runes[0]]; ok {
			token = base
			shift = true
		}
	}

	var sb strings.Builder
	if rctrl {
		sb.WriteString("RCtrl")
	} else if ctrl {
		sb.WriteString("Ctrl")
	}
	if alt {
		sb.WriteString("Alt")
	}
	if shift {
		sb.WriteString("Shift")
	}
	sb.WriteString(token)
	return sb.String()
}

func canonicalKeyToken(key string) string {
	if len(key) > 3 && strings.EqualFold(key[:3], "VK_") {
		return "VK_" + strings.ToUpper(key[3:])
	}
	for _, name := range farKeyNames {
		if strings.EqualFold(key, name) {
			return name
		}
	}
	if len(key) >= 2 && (key[0] == 'F' || key[0] == 'f') {
		if n, err := strconv.Atoi(key[1:]); err == nil && n >= 1 && n <= 24 {
			return "F" + strconv.Itoa(n)
		}
	}
	if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
		return strings.ToUpper(key)
	}
	return key
}

// Resolve returns the spelling that replaces key in area, or "" when no rule
// applies. Exact rules win over wildcard ones, and an area wins over Common,
// mirroring how HotkeyManager resolves a binding.
func (kr *KeyRemap) Resolve(area, key string) string {
	if kr.IsEmpty() || key == "" {
		return ""
	}

	// The event may name the key any of the several ways a terminal makes
	// possible; the rules were stored in one normal form, so bring the key
	// into it too before looking anything up.
	key = canonicalKeySpelling(key)

	// Far treats both Ctrl keys as one, and so does the hotkey dispatcher
	// unless something is bound on the RCtrl spelling specifically. A rule
	// written for Ctrl therefore also answers Right Ctrl.
	candidates := []string{key}
	if strings.HasPrefix(key, "RCtrl") {
		candidates = append(candidates, "Ctrl"+strings.TrimPrefix(key, "RCtrl"))
	}

	areas := []string{area}
	if !strings.EqualFold(area, keyRemapCommonSection) {
		areas = append(areas, keyRemapCommonSection)
	}

	for _, candidateArea := range areas {
		for _, candidate := range candidates {
			if target, ok := kr.Exact[candidateArea][strings.ToLower(candidate)]; ok {
				return target
			}
		}
	}
	for _, candidateArea := range areas {
		for _, candidate := range candidates {
			if target := kr.prefixTarget(candidateArea, candidate); target != "" {
				return target
			}
		}
	}
	return ""
}

func (kr *KeyRemap) prefixTarget(area, key string) string {
	lower := strings.ToLower(key)
	for _, rule := range kr.Prefix[area] {
		// The rule must leave a key behind, not just its modifiers.
		if len(lower) > len(rule.from) && strings.HasPrefix(lower, rule.from) {
			return rule.to + key[len(rule.from):]
		}
	}
	return ""
}

// keyRemapMods are the event flags a substitution owns. Everything else in
// ControlKeyState (lock states in particular) describes the keyboard, not the
// chord, and is carried over untouched.
const keyRemapMods = vtinput.ShiftPressed | vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed |
	vtinput.LeftAltPressed | vtinput.RightAltPressed | vtinput.EnhancedKey

// Apply rewrites e in place when a rule matches, and reports whether it did.
// Both key down and key up are rewritten, so a frame that pairs them sees one
// consistent key.
func (kr *KeyRemap) Apply(area string, e *vtinput.InputEvent) bool {
	if kr.IsEmpty() || e == nil || e.Type != vtinput.KeyEventType {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU:
		// A modifier pressed on its own is not a chord. Rewriting it would
		// desynchronize the key bar and the terminal forwarder, both of which
		// track these keys separately.
		return false
	}

	source := canonicalKeySpelling(EventToHotkeyString(e))
	target := kr.Resolve(area, source)
	if target == "" || strings.EqualFold(target, source) {
		return false
	}
	mapped := ParseFarKey(target)
	if mapped == nil || (mapped.VirtualKeyCode == 0 && mapped.Char == 0) {
		return false
	}

	e.VirtualKeyCode = mapped.VirtualKeyCode
	// The scan code described the physical key that was actually pressed;
	// after a substitution it belongs to no key at all. Injected macro events
	// carry none either.
	e.VirtualScanCode = 0
	e.Char = mapped.Char
	e.UnshiftedChar = mapped.Char
	e.ControlKeyState = (e.ControlKeyState &^ keyRemapMods) | (mapped.ControlKeyState & keyRemapMods)
	vtui.DebugLog("KEYREMAP: %s -> %s in area %s", source, target, area)
	return true
}

// defaultKeymapIni ships as pure documentation: every rule is commented out,
// so loading it changes nothing until the user removes a semicolon.
const defaultKeymapIni = `; Key remapping for f4.
;
; A rule substitutes one key for another before f4 looks at the event:
;
;                 <key f4 should see instead>
;                 v
;     PressedKey=TargetKey
;
; Spell both sides the way the Hotkey Configurator (Options -> Hotkey
; Configuration) shows them: Ctrl, Alt, Shift and RCtrl prefixes, then
; the key itself (A, 5, F7, Enter, Ins, PgDn, VK_DC ...). Case and the
; order of the prefixes do not matter, and a shifted character may be
; written either way: Shift1 and ! name the same chord.
;
; Everything after a ";" or a "#" is a note, not part of the rule.
; Section names are the areas of hotkeys.ini (Shell, Terminal, Editor,
; Viewer, Dialog, Menu, Disks); Common applies to all of them.
;
; Substitution happens once per keystroke, so rules never chain. It is
; also skipped while a full-screen or busy program (vim, htop, mc) owns
; the terminal, because those keys belong to that program: a key that
; stands in for F10 will not quit f4 from inside such a program.
;
; Not every command needs a key. Ctrl+Shift+P opens the command
; palette, which finds any command by name and shows the key it is on,
; so it is the quickest way out of a chord the terminal ate.

[Common]

; --- Terminal multiplexers -------------------------------------------
;
; tmux, zellij, screen and dvtm claim their own chords before f4 ever
; sees them: Ctrl+B (tmux), Ctrl+A (screen), and Ctrl+P, Ctrl+T,
; Ctrl+N, Ctrl+O, Ctrl+G, Ctrl+Q (zellij). Give the affected f4
; commands a second key here.
;
;CtrlAltO=CtrlO           ; toggle the panels, when zellij eats Ctrl+O
;CtrlAltB=CtrlB           ; toggle the key bar, when tmux eats Ctrl+B
;CtrlAltP=CtrlP           ; toggle the passive panel, when zellij eats Ctrl+P
;CtrlAltShiftP=CtrlShiftP ; command palette, when something eats Ctrl+Shift+P
;
; A trailing * on both sides rewrites the modifiers of every key at
; once, which is the one-line way to move f4 off a prefix the
; multiplexer wants. Longer prefixes are matched first.
;
;CtrlAlt*=Ctrl*           ; every Ctrl chord also answers to Ctrl+Alt

; --- Keyboards without an F-row --------------------------------------
;
;Alt1=F1
;Alt2=F2
;Alt3=F3
;Alt4=F4
;Alt5=F5
;Alt6=F6
;Alt7=F7
;Alt8=F8
;Alt9=F9
;Alt0=F10
;Alt-=F11                 ; the key right of 0
;AltShift-=F12            ; and the same key with Shift
;
; ("=" cannot be named on the left: the first "=" of a line separates
; the two sides of the rule, so pick another key for that one.)
;
; The Shift, Ctrl and Alt F-key rows work the same way. Write the
; shifted digits as Shift1 ... Shift0 rather than as ! ... ) — both are
; understood, but the first form says which key you meant.
;
;AltShift1=ShiftF1
;AltShift2=ShiftF2

; --- What a terminal cannot send -------------------------------------
;
; Ctrl does nothing to a digit in a plain terminal: Ctrl+1, Alt+1 and
; Ctrl+Alt+1 all arrive as the same bytes, so a CtrlAlt<digit> rule is
; shadowed by the Alt<digit> one and cannot be given a key of its own.
; The kitty keyboard protocol and win32 input mode do distinguish them,
; but a multiplexer in between usually strips that negotiation. Pick a
; letter or a punctuation key for such a rule instead.
`

// CreateDefaultKeymapIni writes the commented sample file on first start.
// A failure is not worth reporting: the feature is optional and an absent
// file simply means no remapping.
func CreateDefaultKeymapIni(path string) {
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(defaultKeymapIni), 0644)
}

// Suspended reports whether a foreign application currently owns the keyboard.
// With the panels hidden and an AltScreen program or a busy child running,
// every key is forwarded to it verbatim; substituting there would send vim or
// htop a chord the user never pressed. Only the panel frame can answer that, so
// the root assigns this — it is the same handover the noaltscreenapp and
// noterminalapp hotkey conditions respect.
//
// The default answers "yes, suspended", which switches remapping off rather
// than leaving it on. Unwired, the wrong way round would send a foreign program
// chords out of somebody's keymap.ini and look like the program misbehaving;
// this way the only person who notices is the one who configured a remap, and
// what they see is that it does nothing.
var Suspended = func() bool { return true }

// ApplyKeyRemap performs the substitution for a live keystroke and keeps the
// key bar honest about it. It is the only entry point the input filter uses.
func ApplyKeyRemap(area string, e *vtinput.InputEvent) bool {
	kr := GlobalKeyRemap
	if kr.IsEmpty() || Suspended() {
		return false
	}
	if !kr.Apply(area, e) {
		return false
	}
	syncKeyBarModifiers(e)
	return true
}

// syncKeyBarModifiers re-derives the key bar's modifier row from a rewritten
// event. vtui sets that row from the raw event just before calling the input
// filter, so without this the bar would still advertise the Alt labels while
// an "Alt1=F1" rule runs the plain F1 command.
func syncKeyBarModifiers(e *vtinput.InputEvent) {
	if vtui.FrameManager == nil || vtui.FrameManager.KeyBar == nil {
		return
	}
	vtui.FrameManager.KeyBar.SetModifiers(
		e.ControlKeyState&vtinput.ShiftPressed != 0,
		e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0,
		e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0,
	)
}
