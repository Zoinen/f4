package vtui

import "testing"

func TestLocalization_Msg(t *testing.T) {
	// 1. Test built-in string
	if Msg("vtui.Ok") != "&Ok" {
		t.Errorf("Expected '&Ok', got %q", Msg("vtui.Ok"))
	}

	// 2. Test missing key
	if Msg("non.existent.key") != "{non.existent.key}" {
		t.Errorf("Expected placeholder for missing key, got %q", Msg("non.existent.key"))
	}

	// 3. Test AddStrings / Overriding
	AddStrings(map[string]string{
		"test.hello": "Привет",
		"vtui.Ok":    "[Да]",
	})

	if Msg("test.hello") != "Привет" {
		t.Errorf("AddStrings failed, got %q", Msg("test.hello"))
	}

	if Msg("vtui.Ok") != "[Да]" {
		t.Errorf("Overriding existing key failed, got %q", Msg("vtui.Ok"))
	}
}
