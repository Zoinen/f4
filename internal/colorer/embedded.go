// Package colorer carries the colour scheme f4 writes out for colorer4go.
//
// The scheme lives beside the code that installs it because //go:embed cannot
// reach above its own directory: embedded from the repository root, one data
// file would keep a package alive there for no other reason.
package colorer

import (
	_ "embed"
)

// RadiolaHRD is the default RGB colour scheme, written to the user's colorer
// configuration directory on first use.
//
//go:embed configs/base/hrd/rgb/radiola.hrd
var RadiolaHRD string
