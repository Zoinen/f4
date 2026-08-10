package vtui

import (
	"testing"
)

func TestFilterValidator(t *testing.T) {
	v := &FilterValidator{ValidChars: "0123456789ABCDEF"}

	if !v.IsValidInput("123AB") {
		t.Error("Should accept hex string")
	}
	if v.IsValidInput("123AG") {
		t.Error("Should reject non-hex character G")
	}
}

func TestLookupValidator(t *testing.T) {
	v := &LookupValidator{
		List:       []string{"UTF-8", "CP866", "Windows-1251"},
		IgnoreCase: true,
	}

	if !v.Validate("utf-8") {
		t.Error("Lookup failed with IgnoreCase=true")
	}
	if v.Validate("ASCII") {
		t.Error("Lookup should fail for item not in list")
	}
}

func TestMaskValidator(t *testing.T) {
	// Pattern: 2 digits, a dash, 3 letters
	v := &MaskValidator{Mask: "##-???"}

	if !v.IsValidInput("12") {
		t.Error("Partial valid input rejected")
	}
	if v.IsValidInput("1A") {
		t.Error("Mask violation (# expected digit) not detected")
	}
	if !v.Validate("12-ABC") {
		t.Error("Full valid string rejected")
	}
	if v.Validate("12-AB") {
		t.Error("Incomplete string should fail Validate")
	}
}

func TestMaskValidator_Uppercase(t *testing.T) {
	v := &MaskValidator{Mask: "&&&"}

	// Real test of Edit integration would require a mock InputEvent,
	// here we just test the logic of check.
	if !v.check("ABC", false) {
		t.Error("Upper case letters should be valid for '&'")
	}
}

func TestMaskValidator_SpecialMarkers(t *testing.T) {
	// ! - Any (Upper), @ - Any
	v := &MaskValidator{Mask: "!@#"}

	// 1. Valid inputs
	if !v.IsValidInput("A12") {
		t.Error("Valid input rejected")
	}
	if !v.IsValidInput("!%1") {
		t.Error("Symbols should be allowed by ! and @")
	}

	// 2. Marker # constraint
	if v.IsValidInput("AB A") {
		t.Error("Letter in digit slot (#) should be rejected")
	}

	// 3. Length constraint
	if v.IsValidInput("ABCD") {
		t.Error("Input exceeding mask length should be rejected")
	}
}

func TestMaskValidator_LiteralEscaping(t *testing.T) {
	// Test that non-marker characters in mask are treated as literals
	v := &MaskValidator{Mask: "Ref-####"}

	if !v.IsValidInput("Ref-1") {
		t.Error("Valid prefix rejected")
	}
	if v.IsValidInput("Rex-1") {
		t.Error("Mismatching literal 'x' instead of 'f' should be rejected")
	}
}
