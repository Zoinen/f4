package dialog

import (
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/vtui"
)

// proxyModeOrder maps the combo box rows to netproxy modes. The app-wide
// dialog has no "use global" row — there is nothing above it to inherit from.
var proxyModeOrder = []int{netproxy.ModeSystem, netproxy.ModeDirect, netproxy.ModeHTTP, netproxy.ModeSOCKS5}

func proxyModeItems() []string {
	return []string{
		i18n.Msg("ProxySettings.ModeSystem"),
		i18n.Msg("ProxySettings.ModeDirect"),
		i18n.Msg("ProxySettings.ModeHTTP"),
		i18n.Msg("ProxySettings.ModeSOCKS5"),
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

// ActionProxySettings edits the proxy every outgoing connection uses: update
// checks and downloads, the plugin ring, colorer schemes and — unless the
// connection says otherwise — netfox sites.
func ActionProxySettings() {
	width, height := 64, 15
	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("ProxySettings.Title"))
	dlg.ShowClose = true

	modes := proxyModeItems()
	comboMode := vtui.NewComboBox(0, 0, 28, modes)
	comboMode.DropdownOnly = true
	modeIdx := proxyModeIndex(config.App.ProxyMode)
	comboMode.Menu.SetSelectPos(modeIdx)
	comboMode.Edit.SetText(modes[modeIdx])
	lblMode := vtui.NewLabel(0, 0, padProxyLabel(i18n.Msg("ProxySettings.Mode")), comboMode)

	editHost := vtui.NewEdit(0, 0, 24, config.App.ProxyHost)
	lblHost := vtui.NewLabel(0, 0, padProxyLabel(i18n.Msg("ProxySettings.Host")), editHost)
	editPort := vtui.NewEdit(0, 0, 8, config.App.ProxyPort)
	lblPort := vtui.NewLabel(0, 0, padProxyLabel(i18n.Msg("ProxySettings.Port")), editPort)
	editUser := vtui.NewEdit(0, 0, 30, config.App.ProxyUser)
	lblUser := vtui.NewLabel(0, 0, padProxyLabel(i18n.Msg("ProxySettings.User")), editUser)
	editPass := vtui.NewPasswordEdit(0, 0, 30, config.App.ProxyPass)
	lblPass := vtui.NewLabel(0, 0, padProxyLabel(i18n.Msg("ProxySettings.Password")), editPass)
	lblHint := vtui.NewLabel(0, 0, padProxyLabel(i18n.Msg("ProxySettings.Hint")), nil)

	btnOk := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))

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
		config.App.ProxyMode = proxyModeOrder[sel]
		config.App.ProxyHost = strings.TrimSpace(editHost.GetText())
		config.App.ProxyPort = strings.TrimSpace(editPort.GetText())
		config.App.ProxyUser = editUser.GetText()
		config.App.ProxyPass = editPass.GetText()
		// config.SaveConfig republishes the settings, so the next download already
		// takes the new route.
		config.SaveConfig()
		dlg.Close()
	}

	vtui.FrameManager.Push(dlg)
}
