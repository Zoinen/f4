// Package embedded exposes repository-root files that the application embeds.
// An embed directive can only reference paths inside the source directory, and
// README.md must stay in the repository root to render on GitHub, so this tiny
// root package bridges it to cmd/f4.
package embedded

import _ "embed"

//go:embed README.md
var ReadmeMD string

//go:embed colorer/configs/base/hrd/rgb/radiola.hrd
var RadiolaHRD string
