package main

import "strings"

// PanelNavigationMode controls how unmodified keyboard input is interpreted
// while file panels are visible.
type PanelNavigationMode int

const (
	NavigationClassic PanelNavigationMode = iota
	NavigationVim
	NavigationSearchFirst
)

func (m PanelNavigationMode) String() string {
	switch m {
	case NavigationVim:
		return "vim"
	case NavigationSearchFirst:
		return "search"
	default:
		return "classic"
	}
}

func ParsePanelNavigationMode(value string) PanelNavigationMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "vim":
		return NavigationVim
	case "search":
		return NavigationSearchFirst
	default:
		return NavigationClassic
	}
}
