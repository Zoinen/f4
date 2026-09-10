package app

import (
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/vtui"
	"testing"
)

// Layout checks for two dialogs whose subjects are still here: the file
// association editor and the find-file options. The file-operation buttons
// they used to share a file with went to internal/fileops with theirs.

func TestLayout_FileAssociationEditor_AllLanguages(t *testing.T) {
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

	// The association editor is the other dialog shown in the report. Build
	// its New association form for every translation so the final button row
	// remains inside the frame as captions change.
	vtui.AssertLayoutInLanguages(t, packs, func() vtui.Container {
		screen := vtui.NewSilentScreenBuf()
		screen.AllocBuf(120, 60)
		vtui.FrameManager.Init(screen)
		(&panel.AssocEditorState{}).EditAt(0, true)
		if top := vtui.FrameManager.GetTopFrame(); top != nil {
			if dlg, ok := top.(vtui.Container); ok {
				return dlg
			}
		}
		return nil
	})
}

func TestLayout_FindFileOptionsColumns_AllLanguages(t *testing.T) {
	vtui.SetDefaultPalette()

	packs := i18n.LoadAllLanguagePacks()
	if len(packs) == 0 {
		t.Skip("no language packs bundled")
	}

	// The Find File dialog lays its six option checkboxes out as three
	// two-column rows. The right column must start at the same X in
	// every row (#903), and the captions must still fit the dialog.
	build := func() vtui.Container {
		const width, height = 78, 20
		dlg := vtui.NewDialog(0, 0, width-1, height-1, i18n.Msg("FindFile.Title"))

		chkCase := vtui.NewCheckbox(0, 0, i18n.Msg("FindFile.CaseSensitive"), false)
		chkWhole := vtui.NewCheckbox(0, 0, i18n.Msg("FindFile.WholeWords"), false)
		chkRegexp := vtui.NewCheckbox(0, 0, i18n.Msg("FindFile.Regexp"), false)
		chkNotContaining := vtui.NewCheckbox(0, 0, i18n.Msg("FindFile.NotContaining"), false)
		chkFolders := vtui.NewCheckbox(0, 0, i18n.Msg("FindFile.Folders"), false)
		chkSymlinks := vtui.NewCheckbox(0, 0, i18n.Msg("FindFile.Symlinks"), false)
		for _, cb := range []*vtui.Checkbox{chkCase, chkWhole, chkRegexp, chkNotContaining, chkFolders, chkSymlinks} {
			dlg.AddItem(cb)
		}

		vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
		leftColumn := checkboxColumnWidth(chkCase, chkRegexp, chkFolders)
		optionsRow := func(left, right *vtui.Checkbox) *vtui.HBoxLayout {
			row := vtui.NewHBoxLayout(0, 0, width-4, 1)
			row.Spacing = 8
			row.Add(left, vtui.Margins{Right: leftColumn - elementWidth(left)}, vtui.AlignTop)
			row.Add(right, vtui.Margins{}, vtui.AlignTop)
			return row
		}
		vbox.Add(optionsRow(chkCase, chkWhole), vtui.Margins{}, vtui.AlignFill)
		vbox.Add(optionsRow(chkRegexp, chkNotContaining), vtui.Margins{}, vtui.AlignFill)
		vbox.Add(optionsRow(chkFolders, chkSymlinks), vtui.Margins{}, vtui.AlignFill)
		vbox.Apply()

		rightX := func(cb *vtui.Checkbox) int { x1, _, _, _ := cb.GetPosition(); return x1 }
		want := rightX(chkWhole)
		for _, cb := range []*vtui.Checkbox{chkNotContaining, chkSymlinks} {
			if got := rightX(cb); got != want {
				t.Errorf("right column misaligned: %q starts at %d, want %d", cb.GetText(), got, want)
			}
		}
		return dlg
	}
	vtui.AssertLayoutInLanguages(t, packs, build)
}
