package app

import (
	"strings"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// setComboItems replaces a combo box's list and shows text in its field.
func setComboItems(cb *vtui.ComboBox, items []string, selected int, text string) {
	cb.Menu.Items = cb.Menu.Items[:0]
	for _, item := range items {
		cb.Menu.AddItem(vtui.MenuItem{Text: item})
	}
	if selected >= 0 && selected < len(items) {
		cb.Menu.SetSelectPos(selected)
	}
	cb.Edit.SetText(text)
	cb.Edit.ClearSelection()
}

// padLabels gives labels one width, so the fields after them line up.
func padLabels(labels ...string) []string {
	width := 0
	for _, l := range labels {
		width = max(width, vtui.StringWidth(strings.ReplaceAll(l, "&", "")))
	}
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = l + strings.Repeat(" ", width-vtui.StringWidth(strings.ReplaceAll(l, "&", "")))
	}
	return out
}

// actionColorerTypeSettings is FarColorer's HRC settings dialog: pick a file
// type and one of its parameters, and set the parameter's value, or pick its
// default to take the user's value back. OK writes the changes to
// HrcSettings.ini.
func actionColorerTypeSettings(src editor.ColorerSource) {
	types, err := editor.LoadColorerTypeParams(src)
	title := i18n.Msg("ColorerTypeSettings.Title")
	if err != nil || len(types) == 0 {
		message := i18n.Msg("Colorer.NothingFound")
		if err != nil {
			message = err.Error()
		}
		vtui.ShowMessage(title, message, []string{i18n.Msg("vtui.Ok")})
		return
	}

	width, height := 76, 12
	dlg := vtui.NewCenteredDialog(width, height, title)
	dlg.ShowClose = true
	labels := padLabels(i18n.Msg("ColorerTypeSettings.Type"), i18n.Msg("ColorerTypeSettings.Param"), i18n.Msg("ColorerTypeSettings.Value"))

	typeItems := make([]string, len(types))
	for i, t := range types {
		typeItems[i] = t.Group + ": " + t.Description
	}
	field := width - 8 - vtui.StringWidth(strings.ReplaceAll(labels[0], "&", ""))
	comboType := vtui.NewComboBox(0, 0, field, typeItems)
	comboType.DropdownOnly = true
	lblType := vtui.NewLabel(0, 0, labels[0], comboType)
	comboParam := vtui.NewComboBox(0, 0, field, nil)
	comboParam.DropdownOnly = true
	lblParam := vtui.NewLabel(0, 0, labels[1], comboParam)
	comboValue := vtui.NewComboBox(0, 0, field, nil)
	lblValue := vtui.NewLabel(0, 0, labels[2], comboValue)
	editDescription := vtui.NewEdit(0, 0, width-6, "")
	btnOk := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))

	for _, item := range []vtui.UIElement{lblType, comboType, lblParam, comboParam, lblValue, comboValue, editDescription, btnOk, btnCancel} {
		dlg.AddItem(item)
	}
	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	row := func(lbl *vtui.Text, cb *vtui.ComboBox, top int) {
		r := vtui.NewHBoxLayout(0, 0, width-4, 1)
		r.Add(lbl, vtui.Margins{Right: 1}, vtui.AlignLeft)
		r.Add(cb, vtui.Margins{}, vtui.AlignFill)
		vbox.Add(r, vtui.Margins{Top: top}, vtui.AlignFill)
	}
	row(lblType, comboType, 0)
	row(lblParam, comboParam, 1)
	row(lblValue, comboValue, 0)
	vbox.Add(editDescription, vtui.Margins{}, vtui.AlignFill)
	rowButtons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowButtons.HorizontalAlign = vtui.AlignCenter
	rowButtons.Spacing = 2
	rowButtons.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	rowButtons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(rowButtons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	changes := map[string]map[string]*string{}
	curType, curParam := -1, -1

	// saveValue records what the value field holds for the parameter shown,
	// as FarEditorSet::SaveChangedValueParam does whenever the dialog moves
	// on from it.
	saveValue := func() {
		if curType < 0 || curParam < 0 || curParam >= len(types[curType].Params) {
			return
		}
		t := &types[curType]
		if value, changed := editor.ColorerParamEdit(&t.Params[curParam], comboValue.Edit.GetText()); changed {
			if changes[t.Name] == nil {
				changes[t.Name] = map[string]*string{}
			}
			changes[t.Name][t.Params[curParam].Name] = value
		}
	}
	showParam := func(idx int) {
		saveValue()
		curParam = idx
		params := types[curType].Params
		if idx < 0 || idx >= len(params) {
			setComboItems(comboValue, nil, -1, "")
			editDescription.SetText("")
			return
		}
		p := params[idx]
		choices, fixed := editor.ColorerParamChoices(p)
		comboValue.DropdownOnly = fixed
		text := editor.ColorerParamText(p)
		selected := len(choices) - 1
		for i, c := range choices {
			if c == text {
				selected = i
			}
		}
		setComboItems(comboValue, choices, selected, text)
		editDescription.SetText(p.Description)
	}
	showType := func(idx int) {
		saveValue()
		curType, curParam = idx, -1
		names := make([]string, len(types[idx].Params))
		for i, p := range types[idx].Params {
			names[i] = p.Name
		}
		first := ""
		if len(names) > 0 {
			first = names[0]
		}
		setComboItems(comboParam, names, 0, first)
		showParam(0)
	}

	typeAction := comboType.Menu.OnAction
	comboType.Menu.OnAction = func(idx int) {
		typeAction(idx)
		if idx >= 0 && idx < len(types) && idx != curType {
			showType(idx)
		}
	}
	paramAction := comboParam.Menu.OnAction
	comboParam.Menu.OnAction = func(idx int) {
		paramAction(idx)
		if idx != curParam {
			showParam(idx)
		}
	}

	comboType.Menu.SetSelectPos(0)
	comboType.Edit.SetText(typeItems[0])
	showType(0)

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		saveValue()
		if err := editor.SaveColorerParams(changes); err != nil {
			vtui.ShowMessage(title, err.Error(), []string{i18n.Msg("vtui.Ok")})
			return
		}
		dlg.Close()
	}
	vtui.FrameManager.Push(dlg)
}
