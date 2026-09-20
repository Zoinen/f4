package dialog

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func CodepageSettingChoices() ([]int, []string) {
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

// NewCodepageMenu builds a codepage menu sized to the list it is showing.
//
// The three codepage menus all used to be a fixed 45 columns wide, which was
// enough back when the list held a dozen built-in names. Now that f4 offers
// every codepage the system knows about, Windows contributes entries like
// "1141 (IBM EBCDIC - German (20273 + Euro))" -- and VMenu draws item text
// without clipping it, so a longer name was painted over the right border and
// on across whatever was behind the menu. Anything that still does not fit,
// on a narrow terminal, is cut here instead of by the screen edge.
func NewCodepageMenu(title string, items []vtui.MenuItem) *vtui.VMenu {
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

func CodepageChoiceIndex(ids []int, current int) int {
	current = vfs.NormalizeCodepageID(current)
	for i, id := range ids {
		if id == current {
			return i
		}
	}
	return 0
}

// ShowViewerSettings is Options -> Viewer settings: the code page choices, and
// whether pictures and video open in their own viewers.
func ShowViewerSettings() {
	width, height := 78, 14
	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("ViewerSettings.Title"))
	dlg.ShowClose = true

	ids, labels := CodepageSettingChoices()
	comboDefault := vtui.NewComboBox(0, 0, 40, labels)
	comboDefault.DropdownOnly = true
	selected := CodepageChoiceIndex(ids, config.App.ViewerDefaultCodePage)
	comboDefault.Menu.SetSelectPos(selected)
	comboDefault.Edit.SetText(labels[selected])
	lblDefault := vtui.NewLabel(0, 0, i18n.Msg("ViewerSettings.DefaultCodePage"), comboDefault)

	highlightItems := []string{i18n.Msg("ViewerSettings.HighlightOff"), i18n.Msg("ViewerSettings.HighlightQuickView"), i18n.Msg("ViewerSettings.HighlightAll")}
	highlightPos := config.App.ViewerHighlighting
	if highlightPos < 0 || highlightPos >= len(highlightItems) {
		highlightPos = config.ViewerHighlightOff
	}
	comboHighlight := vtui.NewComboBox(0, 0, 40, highlightItems)
	comboHighlight.DropdownOnly = true
	comboHighlight.Menu.SetSelectPos(highlightPos)
	comboHighlight.Edit.SetText(highlightItems[highlightPos])
	lblHighlight := vtui.NewLabel(0, 0, i18n.Msg("ViewerSettings.Highlighting"), comboHighlight)

	chkAutodetect := vtui.NewCheckbox(0, 0, i18n.Msg("ViewerSettings.AutodetectCodePage"), false)
	if config.App.ViewerAutodetectCodePage {
		chkAutodetect.State = 1
	}
	chkByType := vtui.NewCheckbox(0, 0, i18n.Msg("ViewerSettings.OpenAsSupportedType"), false)
	if config.App.ViewerOpenAsSupportedType {
		chkByType.State = 1
	}
	btnOK := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	btnOK.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))

	dlg.AddItem(chkAutodetect)
	dlg.AddItem(lblDefault)
	dlg.AddItem(comboDefault)
	dlg.AddItem(chkByType)
	dlg.AddItem(lblHighlight)
	dlg.AddItem(comboHighlight)
	dlg.AddItem(btnOK)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(chkAutodetect, vtui.Margins{}, vtui.AlignLeft)
	rowDefault := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowDefault.Add(lblDefault, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowDefault.Add(comboDefault, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowDefault, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(chkByType, vtui.Margins{Top: 1}, vtui.AlignLeft)
	// With the editor's highlighter, Chroma or Colorer.
	rowHighlight := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowHighlight.Add(lblHighlight, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowHighlight.Add(comboHighlight, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowHighlight, vtui.Margins{Top: 1}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnOK, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOK.OnClick = func() {
		config.App.ViewerAutodetectCodePage = chkAutodetect.State == 1
		config.App.ViewerOpenAsSupportedType = chkByType.State == 1
		config.App.ViewerHighlighting = comboHighlight.Menu.SelectPos
		if pos := comboDefault.Menu.SelectPos; pos >= 0 && pos < len(ids) {
			config.App.ViewerDefaultCodePage = ids[pos]
		}
		config.SaveConfig()
		dlg.Close()
	}

	vtui.FrameManager.Push(dlg)
}
