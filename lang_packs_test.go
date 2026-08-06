package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestLoadAllLanguagePacks(t *testing.T) {
	packs := LoadAllLanguagePacks()
	if len(packs) < 2 {
		t.Fatalf("expected at least the bundled en and ru packs, got %d", len(packs))
	}

	seen := make(map[string]bool)
	for _, p := range packs {
		if p.Name == "" {
			t.Error("language pack without a name")
		}
		if len(p.Strings) == 0 {
			t.Errorf("language pack %q carries no strings", p.Name)
		}
		if seen[p.Name] {
			t.Errorf("duplicate language pack %q", p.Name)
		}
		seen[p.Name] = true
	}

	if !seen["en"] || !seen["ru"] {
		t.Errorf("expected the en and ru packs to be present, got %v", seen)
	}
}

func TestLayout_ButtonRow_AllLanguages(t *testing.T) {
	vtui.SetDefaultPalette()

	packs := LoadAllLanguagePacks()
	if len(packs) == 0 {
		t.Skip("no language packs bundled")
	}

	// A button row built from localized captions must keep clear of the dialog
	// border in every language, not only in the one currently loaded.
	vtui.AssertLayoutInLanguages(t, packs, func() vtui.Container {
		const width, height = 60, 10
		dlg := vtui.NewDialog(0, 0, width-1, height-1, Msg("VisRen.Title"))

		btnRename := vtui.NewButton(0, 0, Msg("VisRen.Rename"))
		btnCancel := vtui.NewButton(0, 0, Msg("VisRen.Cancel"))

		row := vtui.NewHBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, 1)
		row.Spacing = 2
		row.Add(btnRename, vtui.Margins{}, vtui.AlignTop)
		row.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
		row.Apply()

		dlg.AddItem(btnRename)
		dlg.AddItem(btnCancel)
		return dlg
	})
}
