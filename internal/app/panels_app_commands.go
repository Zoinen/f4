package app

import (
	"github.com/unxed/f4/internal/appcmd"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/settings"
)

// handlePanelsAppCommand answers the frame commands the panels raise and do not
// serve themselves: every one of them opens a dialog or runs an action, which
// is the application's half of the frame. The order of the cases is the order
// PanelsFrame.HandleCommand had them in, because a command can reach a frame
// that is not the active one and the switch is what decides which panels
// answer.
func handlePanelsAppCommand(pf *panel.PanelsFrame, cmd int, args any) bool {
	switch cmd {
	case appcmd.CmNew:
		ActionNewFile(pf)
		return true
	case appcmd.CmView:
		ActionViewFile(pf)
		return true
	case appcmd.CmEdit:
		ActionEditFile(pf)
		return true
	case appcmd.CmCopy, appcmd.CmMove:
		ActionCopyMove(pf, cmd == appcmd.CmMove)
		return true
	case appcmd.CmRename:
		ActionRename(pf)
		return true
	case appcmd.CmMkDir:
		ActionMkDir(pf)
		return true
	case appcmd.CmDelete:
		ActionDelete(pf)
		return true
	case appcmd.CmFindFile:
		ActionFindFile(pf)
		return true
	case appcmd.CmPanelSettings:
		settings.Open("panels")
		return true
	case appcmd.CmEditorSettings:
		settings.Open("editor")
		return true
	case appcmd.CmColorerSettings:
		settings.Open("syntax")
		return true
	case appcmd.CmAppearanceSettings:
		settings.Open("appearance")
		return true
	case appcmd.CmConfirmationsSettings:
		settings.Open("operations")
		return true
	case appcmd.CmHotkeyConfig:
		settings.Open("keyboard")
		return true
	case appcmd.CmLanguage:
		settings.Open("appearance")
		return true
	case appcmd.CmHelpLanguage:
		settings.Open("appearance")
		return true
	case appcmd.CmUpdateSettings:
		settings.Open("updates")
		return true
	case appcmd.CmProxySettings:
		return settings.Open("network")
	case appcmd.CmPlugins:
		settings.Open("plugins")
		return true
	case appcmd.CmPlugRing:
		settings.Open("plugins")
		return true
	case appcmd.CmBackground:
		return ActionBackground()
	case appcmd.CmWorkspaceNew:
		return ActionWorkspaceNew()
	}
	return false
}
