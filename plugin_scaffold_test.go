package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
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

	for _, file := range []string{"plugin.lua", "manifest.json", "README.md"} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Errorf("%s was not written: %v", file, err)
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
	if err := os.WriteFile(filepath.Join(dir, "important.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := ScaffoldPlugin(dir, "notes"); err == nil {
		t.Fatal("scaffolding into a non-empty directory was allowed")
	}
	if _, err := os.Stat(filepath.Join(dir, "important.txt")); err != nil {
		t.Error("an existing file was destroyed")
	}
}

// TestScaffoldedPluginActuallyRuns is the test that matters. A scaffolder that
// emits a plugin which does not load would fail in front of the one audience
// least equipped to debug it.
func TestScaffoldedPluginActuallyRuns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	if _, err := ScaffoldPlugin(dir, "notes"); err != nil {
		t.Fatalf("ScaffoldPlugin: %v", err)
	}

	api := newLuaTestHostAPI()
	plugin := NewLuaPlugin(filepath.Join(dir, "plugin.lua"))
	if err := plugin.Init(api); err != nil {
		t.Fatalf("the scaffolded plugin did not load: %v", err)
	}
	defer plugin.Close()

	factory, ok := api.drives["notes"]
	if !ok {
		t.Fatalf("the scaffolded plugin did not mount its drive, got %v", api.drives)
	}

	fs := factory()
	ctx := context.Background()

	var items []vfs.VFSItem
	if err := fs.ReadDir(ctx, "/", func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("the drive listed %d files, want the ones the template writes", len(items))
	}

	file, err := fs.Open(ctx, items[0].Name)
	if err != nil {
		t.Fatalf("Open(%q): %v", items[0].Name, err)
	}
	defer file.Close()

	buf := make([]byte, file.Size())
	n, err := file.ReadAt(ctx, buf, 0)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "notes") {
		t.Errorf("the first file reads %q, want it to mention the plugin name", buf[:n])
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
	defer os.Chdir(wd)

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
