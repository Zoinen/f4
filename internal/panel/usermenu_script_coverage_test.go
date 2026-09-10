package panel

import (
	"github.com/unxed/f4/vfs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUserMenuInterpreterRecognizesShebangs(t *testing.T) {
	cases := []struct {
		name        string
		commands    []string
		interpreter string
		start       bool
	}{
		{name: "empty"},
		{name: "ordinary", commands: []string{"echo hello"}},
		{name: "empty shebang", commands: []string{"#!"}},
		{name: "shebang", commands: []string{"#!/usr/bin/env bash", "echo hello"}, interpreter: "/usr/bin/env bash", start: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interpreter, start := userMenuInterpreter(tc.commands)
			if interpreter != tc.interpreter || start != tc.start {
				t.Fatalf("userMenuInterpreter() = (%q, %v), want (%q, %v)", interpreter, start, tc.interpreter, tc.start)
			}
		})
	}
}

func TestUserMenuCommandDialectUsesNativeFallback(t *testing.T) {
	want := vfs.CommandDialectPOSIX
	if runtime.GOOS == "windows" {
		want = vfs.CommandDialectCmd
	}
	if got := userMenuCommandDialect(nil); got != want {
		t.Fatalf("userMenuCommandDialect(nil) = %d, want %d", got, want)
	}
}

func TestBuildUserMenuScriptCommandRejectsInvalidInput(t *testing.T) {
	if _, err := buildUserMenuScriptCommand("", "echo hello", vfs.CommandDialectPOSIX); err == nil {
		t.Fatal("empty interpreter was accepted")
	}
	if _, err := buildUserMenuScriptCommand("sh", "echo\x00hello", vfs.CommandDialectPOSIX); err == nil {
		t.Fatal("script containing NUL was accepted")
	}
	if _, err := buildUserMenuScriptCommand("sh", "echo hello", vfs.CommandDialectUnknown); err == nil {
		t.Fatal("unsupported dialect was accepted")
	}
}

func TestBuildUserMenuScriptCommandUsesPOSIXStdin(t *testing.T) {
	command, err := buildUserMenuScriptCommand("bash", "echo 'hello world'", vfs.CommandDialectPOSIX)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "printf") || !strings.Contains(command, "bash -") {
		t.Fatalf("POSIX command = %q", command)
	}
}

func TestBuildUserMenuScriptCommandCreatesCmdScript(t *testing.T) {
	before, err := filepath.Glob(filepath.Join(os.TempDir(), "f4-usermenu-*.script"))
	if err != nil {
		t.Fatal(err)
	}

	command, err := buildUserMenuScriptCommand("cmd", "line one\nline two", vfs.CommandDialectCmd)
	if err != nil {
		t.Fatal(err)
	}
	after, err := filepath.Glob(filepath.Join(os.TempDir(), "f4-usermenu-*.script"))
	if err != nil {
		t.Fatal(err)
	}
	known := make(map[string]bool, len(before))
	for _, path := range before {
		known[path] = true
	}
	created := make([]string, 0, 1)
	for _, path := range after {
		if !known[path] {
			created = append(created, path)
		}
	}
	if len(created) != 1 {
		t.Fatalf("created temp scripts = %v", created)
	}
	path := created[0]
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove temp script: %v", err)
		}
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "line one\r\nline two" {
		t.Fatalf("temp script = %q", data)
	}
	if !strings.Contains(command, "del /Q") {
		t.Fatalf("cmd command = %q", command)
	}
}
