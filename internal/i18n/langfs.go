//go:build !extralite

package i18n

import "embed"

// LangPackFS holds every translation shipped with f4.
//
//go:embed lang/*.lng
var LangPackFS embed.FS
