package panel

import (
	"github.com/unxed/vtui"
)

// Where the editor and the viewer ask about the panels. All four read private
// members of the panel types — pf.closed, pf.visualLeftFSP, pf.GetActivePanel,
// fsp.vfs — so Go requires them in the panels' own package, and they travel to
// internal/panel rather than to internal/editor. They sat in editor_view.go
// without that file ever calling one of them.

// FindPanelsFrameAnyScreen locates the PanelsFrame the user is
// currently working in. The active screen is consulted first: with
// several workspaces open (Ctrl+N) every screen has a PanelsFrame of
// its own, and answering with whichever one happens to sit earliest
// in the slice is how hotkeys ended up hiding panels in one workspace
// while the user was looking at another (issue #424).
//
// When the active screen has none — a full-screen editor or viewer is
// added via AddScreen, so it becomes the active screen while the
// panels stay on the one before — the search walks outwards in
// most-recently-used order. SwitchScreen moves the screen it switches
// to to the end of the slice, so walking down from ActiveIdx visits
// the workspaces in the order the user last used them, and the editor
// finds the panels it was opened from rather than the oldest ones.
func FindPanelsFrameAnyScreen() *PanelsFrame {
	if vtui.FrameManager == nil {
		return nil
	}
	screens := vtui.FrameManager.Screens
	if len(screens) == 0 {
		return nil
	}

	pick := func(idx int) *PanelsFrame {
		if idx < 0 || idx >= len(screens) || screens[idx] == nil {
			return nil
		}
		frames := screens[idx].Frames
		for i := len(frames) - 1; i >= 0; i-- {
			if pf, ok := frames[i].(*PanelsFrame); ok && !pf.Closed {
				return pf
			}
		}
		return nil
	}

	active := vtui.FrameManager.ActiveIdx
	if pf := pick(active); pf != nil {
		return pf
	}
	for i := active - 1; i >= 0; i-- {
		if pf := pick(i); pf != nil {
			return pf
		}
	}
	for i := active + 1; i < len(screens); i++ {
		if pf := pick(i); pf != nil {
			return pf
		}
	}
	return nil
}

// LeftPanelPathForEditor / RightPanelPathForEditor return the
// on-screen visually-left / visually-right file panel's path, or
// "" if the panels frame isn't available. Deliberately uses the
// same visualLeftFSP / visualRightFSP resolvers PanelsFrame uses
// for its own Ctrl+[/Ctrl+] command-line binds, so after Ctrl+U
// the editor stays in lock-step with the panel behaviour instead
// of routing by stale slot index.
func LeftPanelPathForEditor() string {
	pf := FindPanelsFrameAnyScreen()
	if pf == nil {
		return ""
	}
	if fsp := pf.VisualLeftFSP(); fsp != nil {
		return fsp.Vfs.GetPath()
	}
	return ""
}

func RightPanelPathForEditor() string {
	pf := FindPanelsFrameAnyScreen()
	if pf == nil {
		return ""
	}
	if fsp := pf.VisualRightFSP(); fsp != nil {
		return fsp.Vfs.GetPath()
	}
	return ""
}

// ActivePanelNameForEditor returns the currently-selected file name
// on the active file panel, or "" if there isn't one. Not
// distinguishing "no selection" from "no panel" — both yield ""
// which the caller treats as a no-op.
func ActivePanelNameForEditor() string {
	pf := FindPanelsFrameAnyScreen()
	if pf == nil {
		return ""
	}
	fsp := pf.GetActivePanel()
	if fsp == nil {
		return ""
	}
	return fsp.GetSelectedName()
}
