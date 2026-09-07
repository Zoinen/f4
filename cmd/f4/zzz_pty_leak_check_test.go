package main

import (
	"fmt"
	"os"
	"testing"
)

// TestZZZPTYLeakCheck is not a real test: the "zzz" prefix is a deliberate
// hack to make this file sort last among *_test.go files in this package,
// so `go test` links and runs it after (almost) everything else that
// might open a PTY. It reports how many PTYs are still open at that
// point.
//
// It only warns for now, on purpose: until you've confirmed the count is
// reliably zero across a full local `go test ./...`, failing here would
// just turn one known problem (a silent hang) into a different one (a
// spuriously red CI). Once you trust it, flip the t.Log below to
// t.Errorf to make leaks a hard failure.
func TestZZZPTYLeakCheck(t *testing.T) {
	if n := LivePTYCount(); n != 0 {
		msg := fmt.Sprintf("PTY leak guard: %d pseudo-terminal(s) opened via NewPTY were never Close()'d by the time this ran", n)
		_, _ = fmt.Fprintln(os.Stderr, "WARNING:", msg)
		t.Log(msg)
	}
}
