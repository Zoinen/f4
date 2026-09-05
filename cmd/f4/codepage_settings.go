package main

import (
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func codepageSettingChoices() ([]int, []string) {
	ids := make([]int, 0, len(vfs.AvailableCodepages))
	labels := make([]string, 0, len(vfs.AvailableCodepages))
	for _, cp := range vfs.AvailableCodepages {
		ids = append(ids, cp.ID)
		labels = append(labels, vfs.CodepageMenuLabel(cp))
	}
	return ids, labels
}

// codepageMenuChrome is what a VMenu row spends on something other than the
// item text: the two border columns, the space it draws before every item,
// and one column kept clear so a scrollbar never lands on a glyph.
const codepageMenuChrome = 4

// newCodepageMenu builds a codepage menu sized to the list it is showing.
//
// The three codepage menus all used to be a fixed 45 columns wide, which was
// enough back when the list held a dozen built-in names. Now that f4 offers
// every codepage the system knows about, Windows contributes entries like
// "1141 (IBM EBCDIC - German (20273 + Euro))" -- and VMenu draws item text
// without clipping it, so a longer name was painted over the right border and
// on across whatever was behind the menu. Anything that still does not fit,
// on a narrow terminal, is cut here instead of by the screen edge.
func newCodepageMenu(title string, items []vtui.MenuItem) *vtui.VMenu {
	screenW := vtui.FrameManager.GetScreenSize()
	screenH := vtui.FrameManager.GetScreenHeight()

	// Separators are drawn as a rule across the menu, never as text, so
	// their captions must not widen it.
	w := vtui.StringWidth(title) + 6
	for _, item := range items {
		if item.Separator {
			continue
		}
		if itemW := vtui.StringWidth(item.Text) + codepageMenuChrome; itemW > w {
			w = itemW
		}
	}
	if maxW := screenW - 2; w > maxW {
		w = maxW
	}
	if w < 20 {
		w = 20
	}

	menu := vtui.NewVMenu(title)
	for _, item := range items {
		if !item.Separator {
			item.Text = vtui.TruncateString(item.Text, w-codepageMenuChrome, "…")
		}
		menu.AddItem(item)
	}

	h := len(menu.Items) + 2
	maxH := screenH - 2
	if maxH < 5 {
		maxH = 5
	}
	if h > maxH {
		h = maxH
	}
	x := (screenW - w) / 2
	y := (screenH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	menu.SetPosition(x, y, x+w-1, y+h-1)
	return menu
}

func codepageChoiceIndex(ids []int, current int) int {
	current = vfs.NormalizeCodepageID(current)
	for i, id := range ids {
		if id == current {
			return i
		}
	}
	return 0
}

func actionViewerSettings(pf *PanelsFrame) {
	width, height := 78, 10
	dlg := vtui.NewCenteredDialog(width, height, Msg("ViewerSettings.Title"))
	dlg.ShowClose = true

	ids, labels := codepageSettingChoices()
	comboDefault := vtui.NewComboBox(0, 0, 40, labels)
	comboDefault.DropdownOnly = true
	selected := codepageChoiceIndex(ids, AppConfig.ViewerDefaultCodePage)
	comboDefault.Menu.SetSelectPos(selected)
	comboDefault.Edit.SetText(labels[selected])
	lblDefault := vtui.NewLabel(0, 0, Msg("ViewerSettings.DefaultCodePage"), comboDefault)

	chkAutodetect := vtui.NewCheckbox(0, 0, Msg("ViewerSettings.AutodetectCodePage"), false)
	if AppConfig.ViewerAutodetectCodePage {
		chkAutodetect.State = 1
	}
	btnOK := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOK.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(chkAutodetect)
	dlg.AddItem(lblDefault)
	dlg.AddItem(comboDefault)
	dlg.AddItem(btnOK)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(chkAutodetect, vtui.Margins{}, vtui.AlignLeft)
	rowDefault := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowDefault.Add(lblDefault, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowDefault.Add(comboDefault, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowDefault, vtui.Margins{Top: 1}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnOK, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOK.OnClick = func() {
		AppConfig.ViewerAutodetectCodePage = chkAutodetect.State == 1
		if pos := comboDefault.Menu.SelectPos; pos >= 0 && pos < len(ids) {
			AppConfig.ViewerDefaultCodePage = ids[pos]
		}
		SaveConfig()
		dlg.Close()
	}

	vtui.FrameManager.Push(dlg)
}
