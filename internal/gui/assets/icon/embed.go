// Package icon carries the application icon files inside the binary, so that
// `f4 --install` can put them where the desktop looks for them (f4#1290): a
// self-updating single-file install has no archive to take them from. The
// files themselves are generated (see README.md); this package only embeds the
// sizes a desktop needs.
package icon

import "embed"

// Files holds f4.svg (the scalable icon) and generated/f4-N.png for the sizes
// in PNGSizes.
//
//go:embed f4.svg generated/f4-16.png generated/f4-24.png generated/f4-32.png generated/f4-48.png generated/f4-64.png generated/f4-128.png generated/f4-256.png generated/f4-512.png
var Files embed.FS

// PNGSizes are the pixel sizes of the embedded PNG icons.
var PNGSizes = []int{16, 24, 32, 48, 64, 128, 256, 512}
