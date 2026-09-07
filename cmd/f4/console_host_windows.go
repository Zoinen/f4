//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"github.com/unxed/vtui"
	"golang.org/x/sys/windows"
)

var procGetConsoleScreenBufferInfoHost = kernel32SimpleExec.NewProc("GetConsoleScreenBufferInfo")

// consoleHostGone reports whether the console host has died under f4: the
// window is gone, every console call fails, and there is nothing left to
// draw on. That is what conhost crashing looks like from inside (f4 #397,
// microsoft/terminal#4308), and it is not a failure f4 can recover from.
func consoleHostGone() error {
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || h == 0 || h == syscall.InvalidHandle {
		return nil
	}
	var info struct {
		size, cursor      [2]int16
		attributes        uint16
		window            [4]int16
		maximumWindowSize [2]int16
	}
	r1, _, callErr := procGetConsoleScreenBufferInfoHost.Call(uintptr(h), uintptr(unsafe.Pointer(&info)))
	if r1 != 0 {
		return nil
	}
	if errors.Is(callErr, windows.ERROR_PIPE_NOT_CONNECTED) || errors.Is(callErr, windows.ERROR_INVALID_HANDLE) {
		return callErr
	}
	return nil
}

// reportConsoleHostGone leaves a note where crash reports go. The run loop
// ended in good order -- the input channel closed because the host is dead
// -- so no crash report is written, and until now the only trace of what
// happened was a log that stopped after the session was saved.
func reportConsoleHostGone(cause error) {
	vtui.DebugLog("CONSOLE: the console host is gone (%v): the window closed under f4 and every console call fails, "+
		"so f4 exits. On Windows 10 conhost this happens with \"Wrap text output on resize\" turned off "+
		"in the shortcut (microsoft/terminal#4308, f4 #397).", cause)
	dir := vtui.CrashDirFull
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := fmt.Sprintf("console_host_gone_%s_%d.log", time.Now().Format("20060102_150405"), os.Getpid())
	body := fmt.Sprintf("f4 %s\n%s\n\nThe console host (conhost.exe) died while f4 was running: %v\n\n"+
		"This is not a crash in f4. The window belongs to conhost, and when conhost goes down every "+
		"console call from f4 fails with this error, so f4 saves its session and exits.\n\n"+
		"Known cause: Windows 10 conhost, shortcut with \"Wrap text output on resize\" turned off "+
		"(Layout tab), window resized -- microsoft/terminal#4308, unxed/f4#397.\n",
		getFormattedVersionInfo(), time.Now().Format(time.RFC3339), cause)
	_ = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
}
