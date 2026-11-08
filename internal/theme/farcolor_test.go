package theme

import (
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func TestValidateFarColor(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		valid            bool
	}{
		{name: "explicit clear", expression: "", valid: true},
		{name: "legacy names", expression: "F_WHITE | B_BLUE", valid: true},
		{name: "palette reference", expression: "foreground:Panel.Text", valid: true},
		{name: "both components", expression: "foreground:#112233 | background:#ABCDEF", valid: true},
		{name: "partial rgb", expression: "foreground:#123", valid: false},
		{name: "bad rgb", expression: "background:#nope00", valid: false},
		{name: "unknown palette", expression: "foreground:Missing.Palette", valid: false},
		{name: "trailing component", expression: "F_WHITE |", valid: false},
		{name: "nested prefix", expression: "foreground:background:Panel.Text", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateFarColor(tc.expression); (err == nil) != tc.valid {
				t.Fatalf("ValidateFarColor(%q) = %v; valid=%v", tc.expression, err, tc.valid)
			}
		})
	}
}

func TestParseFarColor(t *testing.T) {
	vtui.SetDefaultPalette()
	// The f4 slots live past vtui's own last colour, so the palette has to be
	// grown before ColPanelText can be written below. vtui.SetDefaultPalette
	// sizes it to vtui's range and no further.
	SetDefaultF4Palette()

	// 1. Test named colors
	// F_WHITE is index 15 (0xFFFFFF), B_BLUE is index 1 (0x0028A0)
	attr := ParseFarColor("F_WHITE | B_BLUE", 0)

	if vtui.GetRGBFore(attr) != 0xFFFFFF {
		t.Errorf("Expected Fore RGB FFFFFF, got %06X", vtui.GetRGBFore(attr))
	}
	if vtui.GetRGBBack(attr) != 0x0028A0 {
		t.Errorf("Expected Back RGB 0028A0, got %06X", vtui.GetRGBBack(attr))
	}

	// 2. Test hex colors
	attr = ParseFarColor("foreground:#AABBCC | background:#112233", 0)
	if vtui.GetRGBFore(attr) != 0xAABBCC {
		t.Errorf("Expected Fore RGB AABBCC, got %06X", vtui.GetRGBFore(attr))
	}
	if vtui.GetRGBBack(attr) != 0x112233 {
		t.Errorf("Expected Back RGB 112233, got %06X", vtui.GetRGBBack(attr))
	}

	// 3. Test palette references
	vtui.Palette[ColPanelText] = vtui.SetRGBBoth(0, 0x111111, 0x222222)
	attr = ParseFarColor("foreground:Panel.Text | background:Panel.Text", 0)
	if vtui.GetRGBFore(attr) != 0x111111 {
		t.Errorf("Expected Fore RGB 111111, got %06X", vtui.GetRGBFore(attr))
	}
	if vtui.GetRGBBack(attr) != 0x222222 {
		t.Errorf("Expected Back RGB 222222, got %06X", vtui.GetRGBBack(attr))
	}

	attr = ParseFarColor("Panel.Text", 0)
	if vtui.GetRGBFore(attr) != 0x111111 {
		t.Errorf("Expected Fore RGB 111111, got %06X", vtui.GetRGBFore(attr))
	}
	if vtui.GetRGBBack(attr) != 0x222222 {
		t.Errorf("Expected Back RGB 222222, got %06X", vtui.GetRGBBack(attr))
	}

	// 4. Test default fallback
	def := uint64(0x12345678)
	if ParseFarColor("", def) != def {
		t.Error("Empty expression should return default attribute")
	}
}
func TestFarColorComponentsPreserveUntouchedChannel(t *testing.T) {
	previousPalette := vtui.Palette
	t.Cleanup(func() { vtui.Palette = previousPalette })
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	vtui.Palette[ColPanelText] = vtui.SetRGBBoth(0, 0x111111, 0x222222)

	for _, expr := range []string{
		"", "F_WHITE | B_CYAN", "Panel.Text", "Panel.Text | foreground:#123456",
		"background:#112233 | F_WHITE", "foreground:Panel.Text | background:#112233",
	} {
		foreground, background := FarColorComponents(expr)
		recombined := strings.Trim(strings.Join([]string{foreground, background}, " | "), " |")
		if got, want := ParseFarColor(recombined, 7), ParseFarColor(expr, 7); got != want {
			t.Errorf("%q decomposed to %q: attributes %x, want %x", expr, recombined, got, want)
		}
	}
	foreground, background := FarColorComponents("Panel.Text | F_RED")
	if foreground != "F_RED" || background != "background:Panel.Text" {
		t.Fatalf("later foreground override lost: %q, %q", foreground, background)
	}
}
