package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findVuiFile(t *testing.T, name string) string {
	candidates := []string{
		filepath.Join("testdata", name),
		filepath.Join("..", "..", "testdata", name),
		filepath.Join("..", "testdata", name),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Fatalf("file %s not found in testdata", name)
	return ""
}

func TestVuic_GeneratesValidGoCode(t *testing.T) {
	vuiPath := findVuiFile(t, "hello.vui")
	data, err := os.ReadFile(vuiPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", vuiPath, err)
	}

	var doc VuiDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Failed to parse %s: %v", vuiPath, err)
	}

	code := generateGoCode(&doc, vuiPath, "main")

	if !strings.Contains(code, "package main") {
		t.Error("Generated code missing package main")
	}
	if !strings.Contains(code, "type Ui_HelloDlg struct") {
		t.Error("Generated code missing Ui_HelloDlg struct")
	}
	if !strings.Contains(code, "NameEdit *vtui.Edit") {
		t.Error("Generated code missing NameEdit field")
	}
	if !strings.Contains(code, "BuildHelloDlg()") {
		t.Error("Generated code missing BuildHelloDlg constructor")
	}
	if !strings.Contains(code, "belongs to the author of the .vui file") {
		t.Error("Generated code missing author ownership header")
	}
}
