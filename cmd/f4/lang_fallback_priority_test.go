package main

import "testing"

// A fallback language may only fill keys the primary language lacks — it must
// never override the primary. With primary English the embedded base already
// covers every key, so a configured fallback must change nothing.
func TestInitLang_EnglishPrimaryNotOverriddenByFallback(t *testing.T) {
	oldLang, oldFallback := AppConfig.Language, AppConfig.FallbackLanguage
	defer func() {
		AppConfig.Language, AppConfig.FallbackLanguage = oldLang, oldFallback
		InitLang()
	}()

	AppConfig.Language = "en"
	AppConfig.FallbackLanguage = "ru"
	InitLang()

	if got := Msg("Menu.Exit"); got != "E&xit" {
		t.Errorf("primary=en fallback=ru: Msg(Menu.Exit) = %q, want English \"E&xit\"", got)
	}
}
