package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func withExecutable(t *testing.T, fn func() (string, error)) {
	t.Helper()
	old := Executable
	Executable = fn
	t.Cleanup(func() { Executable = old })
}

func realDir(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestExecutableDirFollowsSymlinkToTheRealBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs a privilege on Windows")
	}
	root := t.TempDir()
	installDir := filepath.Join(root, "install")
	linkDir := filepath.Join(root, "bin")
	for _, d := range []string{installDir, linkDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(installDir, "f4")
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "f4")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, func() (string, error) { return link, nil })

	if got, want := ExecutableDir(), realDir(t, installDir); got != want {
		t.Errorf("ExecutableDir() through a symlink = %q, want the binary's own folder %q", got, want)
	}
}

func TestExecutableDirFindsABareArgv0OnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executable bit and PATH lookup differ on Windows")
	}
	installDir := t.TempDir()
	// exec.LookPath only finds a file with the executable bit.
	// #nosec G306 -- a throw-away fixture in t.TempDir() that PATH lookup must accept.
	if err := os.WriteFile(filepath.Join(installDir, "f4"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, func() (string, error) { return "", errors.New("not wired") })
	t.Setenv("PATH", installDir)
	oldArgs := os.Args
	os.Args = []string{"f4"}
	t.Cleanup(func() { os.Args = oldArgs })

	if got, want := ExecutableDir(), realDir(t, installDir); got != want {
		t.Errorf("ExecutableDir() for a bare argv[0] = %q, want the folder on PATH %q", got, want)
	}
}

func TestExecutableDirKeepsAnExplicitArgv0Path(t *testing.T) {
	installDir := t.TempDir()
	exe := filepath.Join(installDir, "f4")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, func() (string, error) { return "", errors.New("not wired") })
	oldArgs := os.Args
	os.Args = []string{exe}
	t.Cleanup(func() { os.Args = oldArgs })

	if got, want := ExecutableDir(), realDir(t, installDir); got != want {
		t.Errorf("ExecutableDir() for argv[0] with a path = %q, want %q", got, want)
	}
}
