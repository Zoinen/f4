package main

// UserMenuItem is a single entry in the f4 user menu, mirroring far2l's
// internal flat representation. A leaf item has Commands; a submenu has
// a non-nil Submenu (empty slice is a valid empty submenu). Separators
// are marked by HotKey == "--".
type UserMenuItem struct {
	HotKey   string
	Label    string
	Commands []string
	Submenu  []UserMenuItem
}

func (it *UserMenuItem) IsSeparator() bool { return it.HotKey == "--" }
func (it *UserMenuItem) IsSubmenu() bool   { return it.Submenu != nil }
