//go:build !extralite

package app

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/plugins/netbrowse"
	"github.com/unxed/f4/vfs"
)

// netbrowsePanelProviderCommandID mirrors the command plughost.RegisterPanelProvider
// auto-generates for plugins/netbrowse's PanelProvider ID ("f4.netbrowse"). The
// plugin owns the command; this file only gives it a key and a menu row, the
// same split svcmgr_actions.go uses.
const netbrowsePanelProviderCommandID = "panel.f4.netbrowse"

// actionOpenNetwork opens the network browser in the active panel slot
// (f4#1702). False means there is no panels frame to open it in.
func actionOpenNetwork() bool {
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil {
		return false
	}
	return plughost.ExecutePluginCommand(vfs.PluginCommandPanel, netbrowsePanelProviderCommandID, pf)
}

func init() {
	registerAction(action.Action{
		Name:        "App.Network",
		Area:        "Shell",
		Label:       "Network",
		LabelKey:    "Action.App.Network",
		Description: "Open the network (servers and their shares) in the active panel slot",
		DescKey:     "Action.App.Network.Desc",
		// W for "netWork": Ctrl+Alt+Shift+W is free (N is the mount list's).
		DefaultKeys: []string{"CtrlAltShiftW"},
		MenuPath:    "Commands",
		// Present where the network can be browsed: Windows (WNet) and builds
		// with the SMB client.
		Visible: func() bool { return netbrowse.Supported() },
		Handler: actionOpenNetwork,
	})
}
