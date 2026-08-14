//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/unxed/vtui"
)

// installHangDumpHandler makes SIGUSR1 write a full goroutine dump to the
// crash directory, reusing vtui.RecordCrash's format (goroutine stacks,
// TERM/LANG, open fd count, terminal size, UI frame stack, recent log
// history) without treating it as an actual crash: no exit, no recover, the
// process keeps running afterwards.
//
// This exists for reports like "f4 hangs at start" (see PORTABILITY_BSD.md,
// 4.8, issue #429): the client/daemon split means a stuck daemon has no way
// to tell the user anything on its own, and asking someone to reproduce
// under `--debug` is a slow round trip. `kill -USR1 <pid>` on the stuck
// process (found via `ps` or the session picker) is a one-line ask that
// works on every architecture and OS this handler is compiled for, and the
// resulting dump usually pinpoints the stall directly.
func installHangDumpHandler() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)
	go func() {
		for range ch {
			path := vtui.RecordCrash("SIGUSR1: manual diagnostic dump (process is alive, this is not a crash)", nil)
			vtui.DebugLog("DIAG: SIGUSR1 received, goroutine dump written to %s", path)
		}
	}()
}
