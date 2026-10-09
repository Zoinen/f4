//go:build go2xp

package main

// Linking the go2xp shim in gives the binary the polyfills and the GO2XPTBL
// table that "go2xp patch" needs to point absent imports at; on any target
// other than windows/386 the package is empty. Only the legacy-Windows job
// sets this tag, and only that job adds the module to a throwaway modfile, so
// ordinary builds neither see the import nor gain a dependency.
import _ "github.com/unxed/go2xp/shim"
