// The generator writes rsrc_windows_amd64.syso and rsrc_windows_arm64.syso into
// this directory, because the toolchain links a .syso only from the directory of
// the main package. The directive has to live here for the same reason CI runs
// `go generate ./cmd/f4`: go generate reads the files of the package it is
// given, and a package with no directives succeeds silently.
//go:generate go -C ../../tools/icons run .

// Command f4 is the composition root: it exists to build the application and
// run it. Everything it used to hold now lives in internal/app, which is the
// only package allowed to know that every subsystem exists.
package main

import "github.com/unxed/f4/internal/app"

func main() {
	app.Main()
}
