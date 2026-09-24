//go:build amd64 && !android && !ios && (linux || darwin || freebsd || netbsd || windows || dragonfly || solaris || illumos)

package editor

import (
	"os"
	"testing"

	"golang.org/x/sys/cpu"
)

// The colorer-cpu workflow runs this with features masked through GODEBUG at
// process start. Changing cpu.X86 after init would disagree with wazero's cache.
func TestColorerCPUCompatibility(t *testing.T) {
	mode := os.Getenv("F4_TEST_COLORER_CPU")
	if mode == "" {
		t.Skip("run by the Colorer CPU compatibility workflow")
	}
	if mode != "blocked" && mode != "interpreter" {
		t.Fatalf("unknown CPU test mode: %q", mode)
	}

	if cpu.X86.HasPOPCNT {
		t.Fatal("POPCNT was not disabled by GODEBUG")
	}
	src := ColorerSource{ConfigsDir: checkConfigs(t)}
	if mode == "interpreter" {
		if cpu.X86.HasSSE41 {
			t.Fatal("SSE4.1 was not disabled by GODEBUG")
		}
		if check := CheckColorerSource(t.Context(), src, "default", false, nil); !check.Clean() {
			t.Fatalf("interpreter could not load Colorer: %+v", check)
		}
		return
	}

	if !cpu.X86.HasSSE41 {
		t.Fatal("blocked mode requires SSE4.1 so wazero would select its compiler")
	}
	if session, err := acquireColorerSession(src); err != errColorerUnsupportedCPU {
		if session != nil {
			session.Close()
		}
		t.Fatalf("pooled session: got %v, want %v", err, errColorerUnsupportedCPU)
	}
	if session, err := acquireCancelableColorerSession(t.Context(), src); err != errColorerUnsupportedCPU {
		if session != nil {
			session.Close()
		}
		t.Fatalf("cancellable session: got %v, want %v", err, errColorerUnsupportedCPU)
	}
	if check := CheckColorerSource(t.Context(), src, "default", false, nil); check.Err != errColorerUnsupportedCPU {
		t.Fatalf("source check: got %v, want %v", check.Err, errColorerUnsupportedCPU)
	}
	if schemes := ListColorerSchemesFor(src); len(schemes) != 0 {
		t.Fatalf("unsupported CPU loaded schemes: %v", schemes)
	}
}
