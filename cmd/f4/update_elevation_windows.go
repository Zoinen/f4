//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	seeMaskNoCloseProcess = 0x00000040
	waitObject0           = 0x00000000
)

// updateDirNeedsElevation probes the destination directory without touching
// an installed file. This lets a normal user get one UAC prompt before any
// archive entry is partially installed in a protected directory.
func updateDirNeedsElevation(dir string) bool {
	f, err := os.CreateTemp(dir, ".f4-update-permission-*")
	if err == nil {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		return false
	}
	return isPermissionErrorForUpdate(err)
}

func isPermissionErrorForUpdate(err error) bool {
	if err == nil {
		return false
	}
	return os.IsPermission(err) || errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD)
}

func runElevatedUpdate(data []byte, archiveKind string) error {
	tmp, err := os.CreateTemp("", "f4-update-*.archive")
	if err != nil {
		return fmt.Errorf("failed to create temporary update archive: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to save temporary update archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temporary update archive: %w", err)
	}

	exePath, err := osExecutable()
	if err != nil {
		return fmt.Errorf("failed to locate f4 for UAC elevation: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve f4 path for UAC elevation: %w", err)
	}

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return err
	}
	params, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{updateHelperFlag, tmpPath, archiveKind}))
	if err != nil {
		return err
	}

	info := shellExecuteInfo{
		cbSize:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		fMask:        seeMaskNoCloseProcess | seeMaskFlagNoUI,
		hwnd:         0,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		nShow:        swShow,
	}
	result, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		if callErr != syscall.Errno(0) {
			if errors.Is(callErr, windows.ERROR_CANCELLED) {
				return errors.New("UAC elevation was canceled")
			}
			return fmt.Errorf("failed to start elevated updater: %w", callErr)
		}
		return errors.New("failed to start elevated updater")
	}
	if info.hProcess == 0 {
		return errors.New("elevated updater did not return a process handle")
	}
	defer windows.CloseHandle(windows.Handle(info.hProcess))

	waitResult, err := windows.WaitForSingleObject(windows.Handle(info.hProcess), windows.INFINITE)
	if err != nil {
		return fmt.Errorf("failed waiting for elevated updater: %w", err)
	}
	if waitResult != waitObject0 {
		return fmt.Errorf("elevated updater wait returned %d", waitResult)
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(windows.Handle(info.hProcess), &exitCode); err != nil {
		return fmt.Errorf("failed to read elevated updater result: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("elevated updater exited with code %d", exitCode)
	}
	return nil
}

var runUpdateHelper = runUpdateHelperOS

func runUpdateHelperOS(archivePath, archiveKind string) error {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return fmt.Errorf("failed to read update archive: %w", err)
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate executable: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	return extractUpdateArchive(data, archiveKind, filepath.Dir(exePath))
}
