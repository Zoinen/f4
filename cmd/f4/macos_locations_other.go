//go:build !darwin

package main

import "github.com/unxed/vtui"

func macOSLocationsMenuItem(*PanelsFrame, int) (vtui.MenuItem, bool) {
	return vtui.MenuItem{}, false
}
