// Package paneltest holds the test scaffolding that needs a panels frame.
//
// It is separate from testutil because of what building a frame costs: the
// mock frame constructs a terminal view, a command line and a file panel, so a
// package holding it imports internal/panel, internal/cmdline and
// internal/terminal. Those three cannot then import it back, which is why the
// harness that only needs vtui lives in testutil and can be imported by
// everybody.
//
// A test inside panel, cmdline or term that wants this must therefore be an
// external test package — `package panel_test` — so the import runs one way.
//
// This package is filled when internal/panel exists; until then it is empty on
// purpose, so that the boundary is recorded where it is decided rather than
// discovered later.
package paneltest
