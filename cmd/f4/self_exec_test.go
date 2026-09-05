package main

import (
	"reflect"
	"testing"
)

func TestLoaderArgv(t *testing.T) {
	got := loaderArgv("libc.so.6", "/proc/self/fd/3", []string{"--server", "/tmp/f4.sock"})
	want := []string{"--preload", "libc.so.6", "/proc/self/fd/3", "--server", "/tmp/f4.sock"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loaderArgv() = %q, want %q", got, want)
	}
}

func TestLoaderArgvWithoutArguments(t *testing.T) {
	got := loaderArgv("libc.so.6", "/usr/bin/f4", nil)
	want := []string{"--preload", "libc.so.6", "/usr/bin/f4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loaderArgv() = %q, want %q", got, want)
	}
}

// Outside a universal build -- which is every test run that is not itself
// started through goffi's bridge -- selfExecArgv must hand back exactly what
// the caller asked for.
func TestSelfExecArgvPlain(t *testing.T) {
	t.Setenv("GOFFI_UNIVERSAL_REEXEC", "")

	args := []string{"--server", "/tmp/f4.sock"}
	name, argv := selfExecArgv("/usr/bin/f4", args)
	if name != "/usr/bin/f4" {
		t.Errorf("program = %q, want %q", name, "/usr/bin/f4")
	}
	if !reflect.DeepEqual(argv, args) {
		t.Errorf("args = %q, want %q", argv, args)
	}
}
