//go:build !extralite

package app

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/plugins/proclist"
	"github.com/unxed/f4/vfs"
)

// proclistPanelProviderCommandID mirrors the command plughost.RegisterPanelProvider
// auto-generates for plugins/proclist's PanelProvider ID ("f4.proclist"):
// it prefixes it with "panel." and lowercases it
// (internal/plughost/panel_providers.go). The plugin owns the command; this
// file only gives it a key and a menu row, the same split
// sqlite_actions.go uses for plugins/sqlite.
const proclistPanelProviderCommandID = "panel.f4.proclist"

// actionOpenProcList opens the live process list panel in the active panel
// slot. False means the plugin did not register the command at all -- v1 is
// Linux-only (f4#312 part 1 of 4), so this never happens when
// Visible (below) already hid the menu row and its key.
func actionOpenProcList() bool {
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil {
		return false
	}
	return plughost.ExecutePluginCommand(vfs.PluginCommandPanel, proclistPanelProviderCommandID, pf)
}

func init() {
	registerAction(action.Action{
		Name:        "App.ProcList",
		Area:        "Shell",
		Label:       "Process list",
		LabelKey:    "Action.App.ProcList",
		Description: "Open a live list of running processes in the active panel slot",
		DescKey:     "Action.App.ProcList.Desc",
		// R for "processes/Running processes". Ctrl+Alt is otherwise taken
		// by A, D, L, M, O, P, S, T, Z and the digits that switch workspaces
		// and jump to bookmarks (see sqlite_actions.go's own note on the same
		// modifier and its AltGr caveat on some keyboard layouts).
		DefaultKeys: []string{"CtrlAltR"},
		MenuPath:    "Commands",
		// v1 is Linux-only (f4#312 part 1 of 4): plugins/proclist.Supported()
		// is the same static, platform-only check Plugin.Init uses to decide
		// whether to register the command at all, so the menu row and key
		// never appear where triggering them could only ever fail.
		Visible: func() bool { return proclist.Supported() },
		Handler: actionOpenProcList,
	})
}
