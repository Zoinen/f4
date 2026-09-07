package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findVocabularyPath(t *testing.T) string {
	candidates := []string{
		"vocabulary.json",
		"../../vocabulary.json",
		"../vocabulary.json",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Fatal("vocabulary.json not found")
	return ""
}

func TestVocabularyIntegrity(t *testing.T) {
	path := findVocabularyPath(t)
	v, err := LoadVocabulary(path)
	if err != nil {
		t.Fatalf("Failed to parse vocabulary.json: %v", err)
	}

	if v.Version != 1 {
		t.Errorf("Expected version 1, got %d", v.Version)
	}

	requiredWidgets := []string{
		"Dialog", "BorderedFrame", "Button", "Checkbox", "RadioButton",
		"Edit", "ListBox", "ComboBox", "Table", "VMenu", "MenuBar",
		"KeyBar", "StatusLine", "Label", "GroupBox", "Desktop",
	}

	for _, req := range requiredWidgets {
		if _, ok := v.Widgets[req]; !ok {
			t.Errorf("Required widget %q missing from vocabulary.json", req)
		}
	}

	for name, w := range v.Widgets {
		if w.Summary == "" {
			t.Errorf("Widget %q has empty summary", name)
		}
		for pn, prop := range w.Properties {
			if prop.Type == "" {
				t.Errorf("Widget %q property %q has empty type", name, pn)
			}
			if prop.Summary == "" {
				t.Errorf("Widget %q property %q has empty summary", name, pn)
			}
		}
	}

	if len(v.Commands) == 0 {
		t.Error("Commands table is empty")
	}
	if len(v.PaletteRoles) == 0 {
		t.Error("Palette roles list is empty")
	}
}

func TestGenerateMarkdown(t *testing.T) {
	path := findVocabularyPath(t)
	v, err := LoadVocabulary(path)
	if err != nil {
		t.Fatalf("Failed to parse vocabulary.json: %v", err)
	}

	md := GenerateWidgetsMarkdown(v)
	if !strings.Contains(md, "# Справочник виджетов и свойств vtui") {
		t.Error("Generated markdown missing header")
	}
	if !strings.Contains(md, "### `Button`") {
		t.Error("Generated markdown missing Button section")
	}
	if !strings.Contains(md, "### `Dialog`") {
		t.Error("Generated markdown missing Dialog section")
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "widgets.md")
	if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
		t.Fatalf("Failed to write generated markdown: %v", err)
	}
}
