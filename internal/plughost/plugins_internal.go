//go:build !extralite

package plughost

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/plugins/chroma"
	dotnetplugin "github.com/unxed/f4/plugins/dotnet"
	"github.com/unxed/f4/plugins/dummy_internal"
	"github.com/unxed/f4/plugins/envman"
	gitplugin "github.com/unxed/f4/plugins/git"
	"github.com/unxed/f4/plugins/id3editor"
	"github.com/unxed/f4/plugins/ide"
	"github.com/unxed/f4/plugins/intchecker"
	"github.com/unxed/f4/plugins/mediainfo"
	"github.com/unxed/f4/plugins/netbrowse"
	pdfviewplugin "github.com/unxed/f4/plugins/pdfview"
	"github.com/unxed/f4/plugins/proclist"
	sqliteplugin "github.com/unxed/f4/plugins/sqlite"
	"github.com/unxed/f4/plugins/svcmgr"
	"github.com/unxed/f4/plugins/visren"
)

// internalPlugins are the built-in plugins registered at start, in this order.
// The extra-lite profile keeps only what mc has an equivalent of (see
// plugins_internal_extralite.go and docs/OPENWRT.md).
func internalPlugins() []Plugin {
	return []Plugin{
		&chroma.Plugin{},
		&dummy_internal.InternalDummyPlugin{},
		&visren.Plugin{},
		&id3editor.ID3EditorPlugin{},
		// Integrity checker (f4#1623 step 1): checksum file generation.
		intchecker.NewPlugin(config.GetF4ConfigDir()),
		envman.NewPlugin(config.GetF4ConfigDir()),
		mediainfo.NewPlugin(config.GetF4ConfigDir()),
		// Both builds: the lite build's SQLite client runs the host's
		// sqlite3 tool instead of linking the engine (plugins/sqlite's
		// backend_default_lite.go).
		sqliteplugin.NewPlugin(),
		proclist.NewPlugin(config.GetF4ConfigDir()),
		// Windows service list (f4#311 part 1): registers nothing off Windows.
		svcmgr.NewPlugin(),
		// Windows network browser (f4#1702 part 1): registers nothing off Windows.
		netbrowse.NewPlugin(),
		// Git status view (f4#659 part 1 of N): wraps the host's own `git`
		// binary, no platform gate at this layer -- gitplugin.Available()
		// (internal/app/git_actions.go's Visible check) covers "git is
		// missing from PATH" instead, the same plugin/action split
		// plugins/sqlite's CLI backend uses.
		gitplugin.NewPlugin(),
		// .NET assembly browser (f4#1666): Ctrl+PgDn on a .dll/.exe with .NET
		// metadata mounts its references, types and resources read-only.
		dotnetplugin.NewPlugin(),
		// PDF browser (f4#1665): Ctrl+PgDn on a .pdf mounts its text and pictures
		// read-only; F3 on a picture opens f4's image viewer.
		pdfviewplugin.NewPlugin(),
		// IDE mode (f4#382): scaffold only for now -- registration and the
		// three IDE.Build/Run/Test commands, no toolchain integration yet.
		// See plugins/ide's package doc for the full plan.
		ide.NewPlugin(),
	}
}
