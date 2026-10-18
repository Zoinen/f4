package terminal

import (
	"slices"
	"testing"
)

// f4#2057: pbpaste with no UTF-8 locale pasted "Лунобот" as "???????", and one
// locale variable that is not UTF-8 is enough. The clipboard tools get LANG
// and no other locale variable, whatever the locale of f4 is.
func TestUTF8LocaleEnvLeavesOnlyAUTF8Lang(t *testing.T) {
	got := utf8LocaleEnv([]string{
		"HOME=/Users/u", "LANG=C", "LC_CTYPE=C", "LC_ALL=en_US.UTF-8", "LC_MESSAGES=C", "LC_COLLATE=C",
		"__CF_USER_TEXT_ENCODING=0x1F5:0x0:0x0", "PATH=/usr/bin", "LCX=kept",
	})
	want := []string{"HOME=/Users/u", "__CF_USER_TEXT_ENCODING=0x1F5:0x0:0x0", "PATH=/usr/bin", "LCX=kept", "LANG=" + pbcopyLocale}
	if !slices.Equal(got, want) {
		t.Errorf("utf8LocaleEnv = %q, want %q", got, want)
	}
}

func TestUTF8LocaleEnvAddsTheLocaleWhenThereIsNone(t *testing.T) {
	got := utf8LocaleEnv([]string{"HOME=/Users/u"})
	want := []string{"HOME=/Users/u", "LANG=" + pbcopyLocale}
	if !slices.Equal(got, want) {
		t.Errorf("utf8LocaleEnv = %q, want %q", got, want)
	}
}
