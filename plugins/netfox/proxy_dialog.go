package netfox

import (
	"strings"

	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/vtui"
)

// proxyModeOrder maps the combo rows to netproxy modes. Unlike the app-wide
// dialog this one starts with "use the f4 setting", which is what a
// connection does until the user says otherwise.
var proxyModeOrder = []int{netproxy.ModeGlobal, netproxy.ModeSystem, netproxy.ModeDirect, netproxy.ModeHTTP, netproxy.ModeSOCKS5}

func proxyModeItems() []string {
	return []string{
		vtui.Msg("NetFox.ProxyModeGlobal"),
		vtui.Msg("NetFox.ProxyModeSystem"),
		vtui.Msg("NetFox.ProxyModeDirect"),
		vtui.Msg("NetFox.ProxyModeHTTP"),
		vtui.Msg("NetFox.ProxyModeSOCKS5"),
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

// showProxyDialog edits the proxy of a single site. It writes straight into
// cfg, which the connection dialog saves together with everything else, so
// pressing Cancel there discards the proxy edits as well.
func showProxyDialog(cfg *NetFoxConfig) {
	width, height := 64, 15
	dlg := vtui.NewCenteredDialog(width, height, vtui.Msg("NetFox.ProxyTitle"))
	dlg.ShowClose = true

	modes := proxyModeItems()
	comboMode := vtui.NewComboBox(0, 0, 28, modes)
	comboMode.DropdownOnly = true
	idx := proxyModeIndex(cfg.ProxyMode)
	comboMode.Menu.SetSelectPos(idx)
	comboMode.Edit.SetText(modes[idx])
	lblMode := vtui.NewLabel(0, 0, padProxyLabel(vtui.Msg("NetFox.ProxyMode")), comboMode)

	editHost := vtui.NewEdit(0, 0, 24, cfg.ProxyHost)
	lblHost := vtui.NewLabel(0, 0, padProxyLabel(vtui.Msg("NetFox.ProxyHost")), editHost)
	editPort := vtui.NewEdit(0, 0, 8, cfg.ProxyPort)
	lblPort := vtui.NewLabel(0, 0, padProxyLabel(vtui.Msg("NetFox.ProxyPort")), editPort)
	editUser := vtui.NewEdit(0, 0, 30, cfg.ProxyUser)
	lblUser := vtui.NewLabel(0, 0, padProxyLabel(vtui.Msg("NetFox.ProxyUser")), editUser)
	editPass := vtui.NewPasswordEdit(0, 0, 30, cfg.ProxyPass)
	lblPass := vtui.NewLabel(0, 0, padProxyLabel(vtui.Msg("NetFox.ProxyPassword")), editPass)
	lblHint := vtui.NewLabel(0, 0, padProxyLabel(vtui.Msg("NetFox.ProxyHint"))+" "+netproxy.Global().Describe(), nil)

	btnOk := vtui.NewButton(0, 0, vtui.Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, vtui.Msg("vtui.Cancel"))

	for _, it := range []vtui.UIElement{lblMode, comboMode, lblHost, editHost, lblPort, editPort,
		lblUser, editUser, lblPass, editPass, lblHint, btnOk, btnCancel} {
		dlg.AddItem(it)
	}

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	row := func(l, e vtui.UIElement) *vtui.HBoxLayout {
		hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
		hbox.Add(l, vtui.Margins{Right: 1}, vtui.AlignLeft)
		hbox.Add(e, vtui.Margins{}, vtui.AlignLeft)
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
		cfg.ProxyMode = proxyModeOrder[sel]
		cfg.ProxyHost = strings.TrimSpace(editHost.GetText())
		cfg.ProxyPort = strings.TrimSpace(editPort.GetText())
		cfg.ProxyUser = editUser.GetText()
		cfg.ProxyPass = editPass.GetText()
		dlg.Close()
	}

	vtui.FrameManager.Push(dlg)
}
