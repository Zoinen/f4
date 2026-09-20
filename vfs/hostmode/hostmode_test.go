package hostmode

import (
	"io"
	"os"
	"sync"
	"testing"
)

// Posix decides the personality once per process and says why in the debug
// log. It must not write to stderr: in f4 stderr is the session's crash log,
// and any byte there keeps that file on disk after a clean exit, so every
// start used to leave a "crash" log behind on Windows and under Wine
// (issue #474). Each branch of the decision is exercised: the two forced
// values of F4_WINE_POSIX and the automatic probe.
func TestPosixWritesNothingToStderr(t *testing.T) {
	for _, env := range []string{"", "1", "0"} {
		t.Run("F4_WINE_POSIX="+env, func(t *testing.T) {
			t.Setenv("F4_WINE_POSIX", env)
			once = sync.Once{}
			t.Cleanup(func() {
				once = sync.Once{}
				posix = false
			})

			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			saved := os.Stderr
			os.Stderr = w
			got := Posix()
			os.Stderr = saved
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			out, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				t.Fatal(err)
			}

			if len(out) != 0 {
				t.Errorf("Posix() wrote %q to stderr, want nothing", out)
			}
			switch env {
			case "1":
				if !got {
					t.Error("F4_WINE_POSIX=1: Posix() = false, want true")
				}
			case "0":
				if got {
					t.Error("F4_WINE_POSIX=0: Posix() = true, want false")
				}
			}
		})
	}
}
