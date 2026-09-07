package main

import (
	"strings"

	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/vtui"
)

// proxyModeOrder maps the combo box rows to netproxy modes. The app-wide
// dialog has no "use global" row — there is nothing above it to inherit from.
var proxyModeOrder = []int{netproxy.ModeSystem, netproxy.ModeDirect, netproxy.ModeHTTP, netproxy.ModeSOCKS5}

func proxyModeItems() []string {
	return []string{
		Msg("ProxySettings.ModeSystem"),
		Msg("ProxySettings.ModeDirect"),
		Msg("ProxySettings.ModeHTTP"),
		Msg("ProxySettings.ModeSOCKS5"),
	}
}

// padProxyLabel keeps a cell of air between a label and its field.
func padProxyLabel(s string) string { return s + " " }

func proxyModeIndex(mode int) int {
	for i, m := range proxyModeOrder {
		if m == mode {
			return i
		}
	}
	return 0
}

// actionProxySettings edits the proxy every outgoing connection uses: update
// checks and downloads, the plugin ring, colorer schemes and — unless the
// connection says otherwise — netfox sites.
func actionProxySettings() {
	width, height := 64, 15
	dlg := vtui.NewCenteredDialog(width, height, Msg("ProxySettings.Title"))
	dlg.ShowClose = true

	modes := proxyModeItems()
	comboMode := vtui.NewComboBox(0, 0, 28, modes)
	comboMode.DropdownOnly = true
	modeIdx := proxyModeIndex(AppConfig.ProxyMode)
	comboMode.Menu.SetSelectPos(modeIdx)
	comboMode.Edit.SetText(modes[modeIdx])
	lblMode := vtui.NewLabel(0, 0, padProxyLabel(Msg("ProxySettings.Mode")), comboMode)

	editHost := vtui.NewEdit(0, 0, 24, AppConfig.ProxyHost)
	lblHost := vtui.NewLabel(0, 0, padProxyLabel(Msg("ProxySettings.Host")), editHost)
	editPort := vtui.NewEdit(0, 0, 8, AppConfig.ProxyPort)
	lblPort := vtui.NewLabel(0, 0, padProxyLabel(Msg("ProxySettings.Port")), editPort)
	editUser := vtui.NewEdit(0, 0, 30, AppConfig.ProxyUser)
	lblUser := vtui.NewLabel(0, 0, padProxyLabel(Msg("ProxySettings.User")), editUser)
	editPass := vtui.NewPasswordEdit(0, 0, 30, AppConfig.ProxyPass)
	lblPass := vtui.NewLabel(0, 0, padProxyLabel(Msg("ProxySettings.Password")), editPass)
	lblHint := vtui.NewLabel(0, 0, padProxyLabel(Msg("ProxySettings.Hint")), nil)

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	for _, it := range []vtui.UIElement{lblMode, comboMode, lblHost, editHost, lblPort, editPort,
		lblUser, editUser, lblPass, editPass, lblHint, btnOk, btnCancel} {
		dlg.AddItem(it)
	}

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	row := func(l, e vtui.UIElement, extra ...vtui.UIElement) *vtui.HBoxLayout {
		hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
		hbox.Add(l, vtui.Margins{Right: 1}, vtui.AlignLeft)
		hbox.Add(e, vtui.Margins{}, vtui.AlignLeft)
		for _, x := range extra {
			hbox.Add(x, vtui.Margins{Left: 1}, vtui.AlignLeft)
		}
		return hbox
	}
	vbox.Add(row(lblMode, comboMode), vtui.Margins{}, vtui.AlignFill)
	// Host and port share a row: giving every field a line of its own would
	// not fit, and rows that touch are compared horizontally by the layout
	// validator as if they were one.
	hostRow := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hostRow.Add(lblHost, vtui.Margins{Right: 1}, vtui.AlignLeft)
	hostRow.Add(editHost, vtui.Margins{Right: 2}, vtui.AlignLeft)
	hostRow.Add(lblPort, vtui.Margins{Right: 1}, vtui.AlignLeft)
	hostRow.Add(editPort, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(hostRow, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(row(lblUser, editUser), vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(row(lblPass, editPass), vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(lblHint, vtui.Margins{Top: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		sel := comboMode.Menu.SelectPos
		if sel < 0 || sel >= len(proxyModeOrder) {
			sel = 0
		}
		AppConfig.ProxyMode = proxyModeOrder[sel]
		AppConfig.ProxyHost = strings.TrimSpace(editHost.GetText())
		AppConfig.ProxyPort = strings.TrimSpace(editPort.GetText())
		AppConfig.ProxyUser = editUser.GetText()
		AppConfig.ProxyPass = editPass.GetText()
		// SaveConfig republishes the settings, so the next download already
		// takes the new route.
		SaveConfig()
		dlg.Close()
	}

	vtui.FrameManager.Push(dlg)
}
