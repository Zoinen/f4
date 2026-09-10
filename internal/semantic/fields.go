// Package semantic holds the readers every frame needs to speak the GUI
// semantic protocol: the four that unpack a value out of an incoming action
// map, and the two that turn a row of screen cells into the run models an
// external UI draws.
//
// They live here because four layer-3 packages need them — the panel, the
// editor, the viewer and the command line — and a function three of them had
// copied is a function without a home. The package imports nothing above
// layer 0, which is what lets all four have it.
package semantic

import (
	"path/filepath"
)

func BaseName(v interface{ Base(string) string }, path string) string {
	if path == "" {
		return ""
	}
	if v != nil {
		return v.Base(path)
	}
	return filepath.Base(path)
}
