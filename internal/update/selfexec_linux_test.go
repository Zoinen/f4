//go:build linux && (amd64 || arm64)

package update

import (
	"os"
	"reflect"
	"strconv"
	"testing"
)

// With the guard set, f4 must build the same argv goffi's bridge builds:
// loader, --preload, host libc, the image argv[0] names, then our arguments.
func TestSelfExecArgvUniversal(t *testing.T) {
	t.Setenv(GoffiUniversalGuard, "1")

	loader, libc, ok := universalHostLoader()
	if !ok {
		t.Skip("host runs a libc goffi does not recognise; nothing to imitate")
	}

	name, argv := selfExecArgv("/usr/bin/f4", []string{"--server", "/tmp/f4.sock"})
	if name != loader {
		t.Errorf("program = %q, want the host loader %q", name, loader)
	}
	want := []string{"--preload", libc, os.Args[0], "--server", "/tmp/f4.sock"}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("args = %q, want %q", argv, want)
	}
}

// No guard, no detour: an ordinary build must not be sent through a loader.
func TestUniversalHostLoaderNeedsGuard(t *testing.T) {
	t.Setenv(GoffiUniversalGuard, "")
	if loader, libc, ok := universalHostLoader(); ok {
		t.Errorf("universalHostLoader() = %q, %q, true without the guard set", loader, libc)
	}
}

// The path f4 installs to is not recoverable from a re-execed process, so a
// universal build with nothing recorded must say so rather than answer with
// the loader's own path -- which is what os.Executable() returns there, and
// what would have sent the updater to /usr/lib.
func TestF4ExecutableUnknownInUniversalBuild(t *testing.T) {
	t.Setenv(GoffiUniversalGuard, "1")
	t.Setenv(GoffiUniversalExe, "")
	t.Setenv(F4ExeEnv, "")

	if got, err := executable(); err == nil {
		t.Errorf("executable() = %q, nil; want an error", got)
	}
}

func TestF4ExecutablePrefersGoffiRecord(t *testing.T) {
	t.Setenv(GoffiUniversalGuard, "1")
	t.Setenv(GoffiUniversalExe, strconv.Itoa(os.Getpid())+":/usr/bin/f4")
	t.Setenv(F4ExeEnv, "/handed/down/f4")

	got, err := executable()
	if err != nil {
		t.Fatalf("executable() error: %v", err)
	}
	if want := "/usr/bin/f4"; got != want {
		t.Errorf("executable() = %q, want %q", got, want)
	}
}

// A record tagged with another pid was inherited from a parent. The parent
// hands its answer down deliberately instead, through F4_EXE.
func TestF4ExecutableIgnoresInheritedRecord(t *testing.T) {
	t.Setenv(GoffiUniversalGuard, "1")
	t.Setenv(GoffiUniversalExe, strconv.Itoa(os.Getpid()+1)+":/usr/bin/some-other-program")
	t.Setenv(F4ExeEnv, "/usr/bin/f4")

	got, err := executable()
	if err != nil {
		t.Fatalf("executable() error: %v", err)
	}
	if want := "/usr/bin/f4"; got != want {
		t.Errorf("executable() = %q, want %q", got, want)
	}
}

// An ordinary build asks the operating system and is right.
func TestF4ExecutableWithoutGuard(t *testing.T) {
	t.Setenv(GoffiUniversalGuard, "")
	t.Setenv(F4ExeEnv, "/should/be/ignored")

	want, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable here: %v", err)
	}
	got, err := executable()
	if err != nil {
		t.Fatalf("executable() error: %v", err)
	}
	if got != want {
		t.Errorf("executable() = %q, want %q", got, want)
	}
}

func TestSelfExecEnvCarriesThePath(t *testing.T) {
	t.Setenv(GoffiUniversalGuard, "1")
	t.Setenv(GoffiUniversalExe, strconv.Itoa(os.Getpid())+":/usr/bin/f4")
	t.Setenv(F4ExeEnv, "")

	var found bool
	for _, e := range selfExecEnv() {
		if e == F4ExeEnv+"=/usr/bin/f4" {
			found = true
		}
	}
	if !found {
		t.Errorf("selfExecEnv() does not carry %s=/usr/bin/f4", F4ExeEnv)
	}
}
