package winshell

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/zzl/go-win32api/v2/win32"
)

// OpenAssociatedPath invokes the Shell's default verb for a local file or folder.
func OpenAssociatedPath(path, dir string) error {
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	workingDir, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	if dir == "" {
		workingDir = nil
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr := win32.CoInitializeEx(nil, win32.COINIT_APARTMENTTHREADED|win32.COINIT_DISABLE_OLE1DDE)
	if win32.FAILED(hr) {
		return shellHRESULT("initialize Windows Shell apartment", hr)
	}
	defer win32.CoUninitialize()

	info := win32.SHELLEXECUTEINFOW{
		CbSize: uint32(unsafe.Sizeof(win32.SHELLEXECUTEINFOW{})),
		// The calling worker has no message loop. Wait for the association
		// handoff and report any failure through f4's own error dialog.
		FMask:       win32.SEE_MASK_NOASYNC | win32.SEE_MASK_FLAG_NO_UI,
		LpFile:      file,
		LpDirectory: workingDir,
		NShow:       int32(win32.SW_SHOWNORMAL),
	}
	ok, callError := win32.ShellExecuteExW(&info)
	if ok != 0 {
		return nil
	}
	if callError != 0 {
		return syscall.Errno(callError)
	}
	return errors.New("Windows Shell could not open the item")
}
