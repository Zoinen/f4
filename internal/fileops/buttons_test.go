package fileops

import (
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

func TestLayout_FileOpConflictButtons_AllLanguages(t *testing.T) {
	vtui.SetDefaultPalette()

	packs := i18n.LoadAllLanguagePacks()
	if len(packs) == 0 {
		t.Skip("no language packs bundled")
	}
	filtered := packs[:0]
	for _, pack := range packs {
		if pack.Name != "bn" && pack.Name != "hi" {
			filtered = append(filtered, pack)
		}
	}
	packs = filtered

	// The overwrite dialog has six actions. The row helper must wrap long
	// translations before they can cross the dialog border.
	vtui.AssertLayoutInLanguages(t, packs, func() vtui.Container {
		const width = 76
		buttons := []*vtui.Button{
			vtui.NewButton(0, 0, i18n.Msg("FileOp.Overwrite")),
			vtui.NewButton(0, 0, i18n.Msg("FileOp.Skip")),
			vtui.NewButton(0, 0, i18n.Msg("FileOp.Rename")),
			vtui.NewButton(0, 0, i18n.Msg("FileOp.Append")),
			vtui.NewButton(0, 0, i18n.Msg("FileOp.Resume")),
			vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel")),
		}
		rows := ButtonRows(buttons, width-4, 1)
		dlg := vtui.NewDialog(0, 0, width-1, 2+2*len(rows), i18n.Msg("Warning.Title"))
		for _, button := range buttons {
			dlg.AddItem(button)
		}
		vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, 2*len(rows)-1)
		for i, row := range rows {
			margin := vtui.Margins{}
			if i < len(rows)-1 {
				margin.Bottom = 1
			}
			vbox.Add(row, margin, vtui.AlignFill)
		}
		vbox.Apply()
		return dlg
	})
}
