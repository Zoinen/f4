package plughost

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePluginName(t *testing.T) {
	for _, name := range []string{"notes", "my-drive", "my_drive_2", "A1"} {
		if err := validatePluginName(name); err != nil {
			t.Errorf("validatePluginName(%q) = %v, want it accepted", name, err)
		}
	}
	for _, name := range []string{"", "a/b", "..", "a b", "drive!", strings.Repeat("x", 65)} {
		if err := validatePluginName(name); err == nil {
			t.Errorf("validatePluginName(%q) accepted a name that cannot be a directory, a drive and an id at once", name)
		}
	}
}

func TestScaffoldPluginWritesItsFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")

	created, err := ScaffoldPlugin(dir, "notes")
	if err != nil {
		t.Fatalf("ScaffoldPlugin: %v", err)
	}
	if len(created) != 3 {
		t.Fatalf("created %v, want three files", created)
	}

	for _, File := range []string{"plugin.lua", "manifest.json", "README.md"} {
		if _, err := os.Stat(filepath.Join(dir, File)); err != nil {
			t.Errorf("%s was not written: %v", File, err)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	var item PlugRingItem
	if err := json.Unmarshal(manifest, &item); err != nil {
		t.Fatalf("the manifest is not valid JSON: %v", err)
	}
	if item.ID != "notes" || item.Entrypoint != "plugin.lua" {
		t.Errorf("manifest = %+v, want it to name the plugin and its entrypoint", item)
	}
}

func TestScaffoldPluginRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "important.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := ScaffoldPlugin(dir, "notes"); err == nil {
		t.Fatal("scaffolding into a non-empty directory was allowed")
	}
	if _, err := os.Stat(filepath.Join(dir, "important.txt")); err != nil {
		t.Error("an existing file was destroyed")
	}
}

func TestRunNewPluginReportsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := RunNewPlugin("  ", &out, &errOut); code == 0 {
		t.Fatal("a missing name was accepted")
	}
	if !strings.Contains(errOut.String(), "--new-plugin") {
		t.Errorf("stderr = %q, want a usage line", errOut.String())
	}
}

func TestRunNewPluginPrintsNextSteps(t *testing.T) {
	base := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(base); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	var out, errOut bytes.Buffer
	if code := RunNewPlugin("notes", &out, &errOut); code != 0 {
		t.Fatalf("RunNewPlugin = %d, stderr %q", code, errOut.String())
	}

	printed := out.String()
	if !strings.Contains(printed, "plugin.lua") {
		t.Errorf("output = %q, want it to name the file to edit", printed)
	}
	if !strings.Contains(printed, "Alt+F1") {
		t.Errorf("output = %q, want it to say how to see the plugin working", printed)
	}
}
