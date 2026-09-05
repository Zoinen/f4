package main

import (
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

func TestFormatVersionSHA(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"v0.1.1-alpha-a1b2c3d4", "v0.1.1-alpha-a1b2c3d"},
		{"f4 version 0.1 (commit ffeeddcc)", "f4 version 0.1 (commit ffeeddc)"},
		{"v0.1.1-a1b2c3d45", "v0.1.1-a1b2c3d45"},
		{"v0.1.1-a1b2c3d", "v0.1.1-a1b2c3d"},
	}

	for _, tt := range tests {
		if got := formatVersionSHA(tt.in); got != tt.want {
			t.Errorf("formatVersionSHA(%q) = %q, want %q", tt.in, got, tt.want)
		}
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
	ini := ParseIni(strings.NewReader(iniContent))
	m := loadLangMapFromINI(ini)

	if m["Key1"] != "Value1" {
		t.Errorf("Expected 'Value1', got %q", m["Key1"])
	}

	// Проверяем, что \n корректно превращается в реальный перенос строки
	if m["Key2"] != "Line1\nLine2" {
		t.Errorf("Expected unescaped newline, got %q", m["Key2"])
	}
}
