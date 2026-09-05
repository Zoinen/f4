package main

import (
	"github.com/unxed/vtui"
	"testing"
)

func TestParseFarColor(t *testing.T) {
	vtui.SetDefaultPalette()

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
