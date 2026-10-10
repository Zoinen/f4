//go:build !extralite

package app

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/plugins/svcmgr"
	"github.com/unxed/f4/vfs"
)

// svcmgrPanelProviderCommandID mirrors the command plughost.RegisterPanelProvider
// auto-generates for plugins/svcmgr's PanelProvider ID ("f4.svcmgr"): it
// prefixes it with "panel." and lowercases it. The plugin owns the command;
// this file only gives it a key and a menu row, the same split
// proclist_actions.go and git_actions.go use.
const svcmgrPanelProviderCommandID = "panel.f4.svcmgr"

// actionOpenServices opens the Windows services panel in the active panel
// slot (f4#311). False means there is no panels frame to open it in.
func actionOpenServices() bool {
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil {
		return false
	}
	return plughost.ExecutePluginCommand(vfs.PluginCommandPanel, svcmgrPanelProviderCommandID, pf)
}

func init() {
	registerAction(action.Action{
		Name:        "App.Services",
		Area:        "Shell",
		Label:       "Services",
		LabelKey:    "Action.App.Services",
		Description: "Open the list of Windows services in the active panel slot",
		DescKey:     "Action.App.Services.Desc",
		// V for "serVices": Ctrl+Alt+Shift+V is free (Ctrl+Alt+S is the sqlite
		// action's, and the plain Ctrl+Alt letters are mostly taken; see
		// proclist_actions.go's note on the modifier and its AltGr caveat).
		DefaultKeys: []string{"CtrlAltShiftV"},
		MenuPath:    "Commands",
		// Only Windows has a Service Control Manager, and only there does the
		// plugin register the panel at all.
		Visible: func() bool { return svcmgr.Supported() },
		Handler: actionOpenServices,
	})
}
