//go:build extralite

package i18n

import "embed"

// LangPackFS of the extra-lite profile (unxed/f4#1671, docs/OPENWRT.md) holds
// English and Russian only: the other translations are about 5 MB of a binary
// meant for routers. A translation is still found on disk, in the language directories
// InitLang searches (SearchDirs), so a user who needs one copies its .lng
// there; nothing else in the program depends on the embedded set.
//
//go:embed lang/en.lng lang/ru.lng
var LangPackFS embed.FS
