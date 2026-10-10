package git

import (
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
)

// showHelp is F1 on the status panel (f4#272): the plugin's own help, a
// Markdown text shown in vtui's Markdown viewer, the window f4's F1 help and
// the F3 view of .md files use. The text is a language-file string
// (GitStatus.Help), so it follows the interface language like the rest of
// the panel; a plugin that wants a help of its own does the same -- declares
// F1 among its PanelKeys (vfs.PanelKeyProvider), which the host runs ahead of
// the global Help binding, and shows its text this way.
func (p *statusPanel) showHelp() {
	vfs.ShowPanelHelp(i18n.Msg("GitStatus.HelpTitle"), i18n.Msg("GitStatus.Help"))
}
