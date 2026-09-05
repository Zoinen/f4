package main

import (
	"testing"
)

// Alt+Left/Right folder history navigation is a pair of actions, not
// hardcoded keys in PanelsFrame.ProcessKey.
func TestFolderHistoryActionsRegistered(t *testing.T) {
	hm := NewHotkeyManager("")

	if got := hm.GetAction("Shell", "AltLeft"); got != "Panel.HistoryBack" {
		t.Errorf("AltLeft should be bound to Panel.HistoryBack, got %q", got)
	}
	if got := hm.GetAction("Shell", "AltRight"); got != "Panel.HistoryForward" {
		t.Errorf("AltRight should be bound to Panel.HistoryForward, got %q", got)
	}

	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = hm
	defer func() { GlobalHotkeysMgr = old }()

	// Both appear in the generated Commands menu.
	items := BuildMenuBarItems("Shell")
	var commands []string
	for _, m := range items {
		if m.Label == "&Commands" {
			for _, it := range m.SubItems {
				commands = append(commands, it.Text)
			}
		}
	}
	var back, fwd bool
	for _, text := range commands {
		if text == "History &back" {
			back = true
		}
		if text == "History &forward" {
			fwd = true
		}
	}
	if !back || !fwd {
		t.Errorf("History actions missing from Commands menu: %v", commands)
	}
}
