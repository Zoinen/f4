package i18n

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/vtui"
	"strings"
	"testing"
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

func TestEmbeddedCurrentLanguageContainsNewCommandStrings(t *testing.T) {
	ru := LoadEmbeddedLanguageMap("ru")
	if got := ru["Action.Workspace.Close"]; !strings.Contains(got, "Закрыть") {
		t.Fatalf("embedded Russian workspace label = %q", got)
	}
	if got := ru["Archive.Command.Extract"]; !strings.Contains(got, "Извлечь") {
		t.Fatalf("embedded Russian plugin label = %q", got)
	}
	if got := LoadEmbeddedLanguageMap("does-not-exist"); got != nil {
		t.Fatalf("unknown embedded language = %#v, want nil", got)
	}
}

func TestInitLangUsesEmbeddedCurrentLanguageAndResetsFallback(t *testing.T) {
	t.Cleanup(func() { InitLang("", "", "") })

	InitLang("ru", "", "")
	if got := Msg("Action.Workspace.Close"); !strings.Contains(got, "Закрыть") {
		t.Fatalf("Russian current-language label = %q", got)
	}

	InitLang("de", "", "")
	if got := Msg("Action.Workspace.Close"); action.PlainLabel(got) != "Close workspace" {
		t.Fatalf("missing German translation inherited a stale language: %q", got)
	}
}

func TestInitLangPreservesRuntimePluginStrings(t *testing.T) {
	previousStrings := vtui.SnapshotStrings()
	languageState.Lock()
	previousCore := make(map[string]string, len(languageState.core))
	for key, value := range languageState.core {
		previousCore[key] = value
	}
	languageState.Unlock()
	t.Cleanup(func() {
		languageState.Lock()
		languageState.core = previousCore
		vtui.ReplaceStrings(previousStrings)
		languageState.Unlock()
	})

	const key = "Test.Plugin.RuntimeTranslation"
	vtui.AddStrings(map[string]string{key: "runtime plugin text"})
	InitLang("ru", "", "")
	if got := Msg(key); got != "runtime plugin text" {
		t.Fatalf("runtime plugin string after language switch = %q", got)
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
