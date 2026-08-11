package main

import (
	"fmt"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// actionPluginPermissions shows what has been granted and lets the user take
// it back.
//
// The list is global rather than per plugin. The question somebody opens this
// with is "what have I allowed", and answering it should not mean walking
// every installed plugin in turn. It reads the store the gate writes, so an
// answer given a minute ago is already in it.
//
// The store arrives as an argument rather than through PluginPermissions() so
// that the dialog can be driven by a test without touching real grants.
//
// The wording here is English, as it is everywhere else in the permission
// model: the permission vocabulary itself is not translated yet, and half a
// translation reads worse than none.
func actionPluginPermissions(store *PermissionStore) {
	width, height := 66, 16
	dlg := vtui.NewCenteredDialog(width, height, Msg("Permissions.Title"))
	dlg.ShowClose = true

	grants := store.Grants()

	lb := vtui.NewListBox(0, 0, width-4, height-6, permissionDialogLines(grants))

	btnRevoke := vtui.NewButton(0, 0, Msg("Permissions.BtnRevoke"))
	btnClose := vtui.NewButton(0, 0, Msg("Permissions.BtnClose"))

	dlg.AddItem(lb)
	dlg.AddItem(btnRevoke)
	dlg.AddItem(btnClose)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(lb, vtui.Margins{Bottom: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnRevoke, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnClose, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()

	btnRevoke.OnClick = func() {
		idx := lb.SelectPos
		if idx < 0 || idx >= len(grants) {
			// Also the empty case: the placeholder line is not a grant.
			return
		}
		grant := grants[idx]

		confirm := vtui.ShowMessageOn(dlg, " Revoke Permission ",
			fmt.Sprintf("Stop letting %s %s?\n\nIt will be asked again the next time it tries.",
				grant.Plugin, permissionTitle(grant.Permission)),
			[]string{"&Revoke", "Cancel"})
		if confirm == nil {
			return
		}
		confirm.OnResult = func(code int) {
			if code != 0 {
				return
			}
			if err := store.Revoke(grant.Plugin, grant.Permission); err != nil {
				vtui.DebugLog("PERMISSIONS: cannot revoke %s from %s: %v", grant.Permission, grant.Plugin, err)
			}
			grants = store.Grants()
			lb.Items = permissionDialogLines(grants)
			lb.UpdateRows()
			vtui.FrameManager.Redraw()
		}
	}

	btnClose.OnClick = func() { dlg.Close() }

	lb.OnKeyDown = func(e *vtinput.InputEvent) bool {
		if !e.KeyDown {
			return false
		}
		switch e.VirtualKeyCode {
		case vtinput.VK_DELETE, vtinput.VK_F8:
			btnRevoke.OnClick()
			return true
		}
		return false
	}

	vtui.FrameManager.Push(dlg)
}

// permissionDialogLines keeps the empty case from looking like a broken
// dialog. An empty list says nothing at all, and "nothing yet" is an answer.
func permissionDialogLines(grants []PermissionGrant) []string {
	if len(grants) == 0 {
		return []string{"Nothing has been granted yet."}
	}
	return PermissionGrantLines(grants)
}
