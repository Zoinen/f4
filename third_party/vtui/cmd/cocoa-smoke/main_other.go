//go:build !darwin || ios || vtui_nococoa

// Command cocoa-smoke checks vtui's Cocoa backend end to end; see main.go.
// There is no Cocoa backend in this build, so there is nothing to check.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "cocoa-smoke: this build has no Cocoa backend (macOS only, and not with -tags vtui_nococoa)")
	os.Exit(2)
}
