//go:build !extralite

package app

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	gitplugin "github.com/unxed/f4/plugins/git"
	"github.com/unxed/f4/vfs"
)

// gitStatusPanelProviderCommandID mirrors the command plughost.RegisterPanelProvider
// auto-generates for plugins/git's PanelProvider ID ("f4.gitstatus"): it
// prefixes it with "panel." and lowercases it (internal/plughost/panel_providers.go).
// The plugin owns the command; this file only gives it a key and a menu row,
// the same split proclist_actions.go uses for plugins/proclist.
const gitStatusPanelProviderCommandID = "panel.f4.gitstatus"

// actionOpenGitStatus opens the git status panel in the active panel slot.
// False means the panel provider was not registered -- it always is
// (plugins/git.Plugin.Init has no platform gate), so this only fails if
// there is no active panels frame at all, the same as ProcList's own
// actionOpenProcList.
func actionOpenGitStatus() bool {
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil {
		return false
	}
	return plughost.ExecutePluginCommand(vfs.PluginCommandPanel, gitStatusPanelProviderCommandID, pf)
}

func init() {
	registerAction(action.Action{
		Name:        "App.GitStatus",
		Area:        "Shell",
		Label:       "Git status",
		LabelKey:    "Action.App.GitStatus",
		Description: "Open the working tree status of the active panel's git repository",
		DescKey:     "Action.App.GitStatus.Desc",
		// G for "Git". Ctrl+Alt is otherwise taken by A, B, D, F, I, L, M,
		// O, P, R, S, T, Z and the digits/bookmarks (see
		// proclist_actions.go's own note on the same modifier and its
		// AltGr caveat on some keyboard layouts).
		DefaultKeys: []string{"CtrlAltG"},
		MenuPath:    "Commands",
		// Visible whenever a git binary is on PATH -- opening the panel
		// itself, not this menu gate, is what reports "not a git
		// repository" for a directory that has git but is not one
		// (plugins/git/panel.go's newStatusPanel).
		Visible: func() bool { return gitplugin.Available() },
		Handler: actionOpenGitStatus,
	})
}
