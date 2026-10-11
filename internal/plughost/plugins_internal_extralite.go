//go:build extralite

package plughost

import "github.com/unxed/f4/plugins/chroma"

// internalPlugins of the extra-lite profile (unxed/f4#1671): only what has an
// equivalent in Midnight Commander. Syntax highlighting (mc's editor and
// viewer highlight) stays; the visual renamer, the tag editor, the checksum
// tool, the environment manager, media information, the SQLite client, the
// process, service and git panels, the .NET and PDF browsers, the IDE mode
// and the test dummy do not, and are left out. Archives and network drives
// come from optionalVFSPlugins. See docs/OPENWRT.md for the reasoning.
func internalPlugins() []Plugin {
	return []Plugin{&chroma.Plugin{}}
}
