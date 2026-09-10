package i18n

import (
	"github.com/unxed/f4/internal/ini"
	"strings"
	"testing"
)

func TestMsg(t *testing.T) {
	// 1. Test existing key
	got := Msg("Panel.UpDir")
	want := "UP-DIR"
	if got != want {
		t.Errorf("Msg(Panel.UpDir) = %q; want %q", got, want)
	}

	// 2. Test missing key (should return {key})
	got = Msg("NonExistentKey")
	want = "{NonExistentKey}"
	if got != want {
		t.Errorf("Msg(NonExistentKey) = %q; want %q", got, want)
	}
}

func TestLoadLangMapFromINI(t *testing.T) {
	iniContent := `
[Language]
Name=TestLang

[Strings]
Key1=Value1
Key2=Line1\nLine2
`
	ini := ini.Parse(strings.NewReader(iniContent))
	m := LoadLangMapFromINI(ini)

	if m["Key1"] != "Value1" {
		t.Errorf("Expected 'Value1', got %q", m["Key1"])
	}

	// Проверяем, что \n корректно превращается в реальный перенос строки
	if m["Key2"] != "Line1\nLine2" {
		t.Errorf("Expected unescaped newline, got %q", m["Key2"])
	}
}
