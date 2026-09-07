# Issue #807 solution review

## Report and current behavior

Bookmarks opens from the Commands menu and from the command palette, but
Ctrl+Shift+\ does nothing. The menu and the palette invoke the handler
directly; only the keyboard route goes through the hotkey tables, so the
binding itself is what fails.

`Panel.Bookmarks` declares `DefaultKeys: []string{"CtrlShiftVK_DC"}`, and the
key a press is looked up under comes from `EventToFarString`. That function
tries, in order: named keys, F1-F24, A-Z, 0-9, `e.Char` when it is printable,
and only then `VK_%X`. The character branch wins whenever the backend fills
`e.Char` in.

f4 turns on the kitty keyboard protocol with `\x1b[>15u`, which includes the
alternate-key reporting flag, so a terminal that speaks it sends the shifted
key alongside the base key. Feeding the reported sequences through
`vtinput.ParseKitty`:

    "\x1b[92;5u"      -> VK=0xDC Char='\'  Ctrl
    "\x1b[92:124;6u"  -> VK=0xDC Char='|'  Ctrl+Shift

The lookup keys are therefore `Ctrl\` and `CtrlShift|`, and no VK_DC binding
matches. `Panel.GoRoot` (`CtrlVK_DC`) is broken the same way, as are
`CtrlVK_DB` and `CtrlVK_DD`. The far2l, Win32 and legacy-tty backends deliver
VK_OEM_5 with no character when Ctrl is held, which is why the bindings work
there and the report is terminal-specific.

## Three-pass review

### Pass 1: change `EventToFarString`

Make the global normalizer prefer the virtual key for punctuation. Reject:
that function also names recorded macros and Lua macro triggers, where Far
compatibility and existing user files decide the spelling. The review for
issue #492 rejected a global change to this function for the same reason.

### Pass 2: resolve an alias in `HotkeyManager.GetAction`

Add a punctuation counterpart to `delKeyAlias`, mapping `CtrlShift|` to
`CtrlShiftVK_DC` on a miss. Reject: the alias table would have to know which
character every layout puts on a shifted key, and the assignment dialog would
still write the character spelling into hotkeys.ini, so a hotkey assigned
under kitty would stop working under far2l.

### Pass 3: normalize in `EventToHotkeyString`

Spell the punctuation keys after their virtual key in the
configurable-hotkey layer, which already exists to differ from the macro
layer over RCtrl and which both the runtime dispatcher (`MacroManager.Filter`,
`LookupHotkey`) and the assignment dialog use. Runtime lookup and capture stay
in step, macros keep their Far spelling, and the same key gets the same name
on every backend. Select this pass.

The set is the one `keyTokenDisplayNames` already renders back into
punctuation for the UI, so `FormatKeyForUI` keeps showing `Ctrl+Shift+\`.

## Scope note

The normalization is not conditional on Ctrl or Alt: an unmodified `\` is
named `VK_DC` in the hotkey layer too. That keeps one spelling in hotkeys.ini
whatever the dialog captured. No default binds an unmodified punctuation key,
and text input never goes through this path, so nothing shipped changes
behavior. A hand-written hotkeys.ini that binds a bare character on one of
these keys would need the VK spelling; the file is diffed against defaults on
save, so it is rewritten in the canonical form on the next save.

As a side effect, a layout that types another character on one of these keys
now resolves to the same binding: Ctrl+Ё is `CtrlVK_C0`, which is what the
three-spelling `NativeKeys` list on `Panel.ToggleCommandLineFocus` works
around today.

## Safety checks

`TestEventToHotkeyStringNamesPunctuationByVirtualKey` pins the naming for the
kitty, far2l and RCtrl forms of the key, and asserts that `EventToFarString`
still returns `CtrlShift|` so the macro layer is provably untouched.
`TestKittyBackslashReachesBookmarks` walks the bytes a terminal in kitty mode
sends through `vtinput.ParseKitty`, the hotkey naming and the default
bindings, and checks that Ctrl+Shift+\ lands on `Panel.Bookmarks` and Ctrl+\
on `Panel.GoRoot`. `TestHotkeyManager_BookmarksDefault` and
`TestEventToHotkeyStringPreservesRightCtrl` continue to guard the default
table and the RCtrl spelling.

Manual validation is still worth doing in a terminal in kitty mode, in far2l
and on Windows, since only the first of the three is exercised by the tests.
