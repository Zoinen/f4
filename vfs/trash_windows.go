//go:build windows

package vfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/zzl/go-win32api/v2/win32"
)

var clsidFileOperation = syscall.GUID{
	Data1: 0x3AD05575,
	Data2: 0x8857,
	Data3: 0x4850,
	Data4: [8]byte{0x92, 0x77, 0x11, 0xB8, 0x5B, 0xDB, 0x8E, 0x09},
}

var _ TrashVFS = (*OSVFS)(nil)

// MoveToTrash uses IFileOperation with FOFX_RECYCLEONDELETE. Unlike the older
// FOF_ALLOWUNDO flag, RECYCLEONDELETE requires recycling and fails instead of
// silently deleting an item permanently when no recycle bin is available.
func (v *OSVFS) MoveToTrash(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	abs, err := v.Abs(path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(stripExtendedPrefix(abs))
	if _, err := os.Lstat(abs); err != nil {
		return err
	}

	// COM apartment initialization and every interface call must remain on the
	// same native thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr := win32.CoInitializeEx(nil, win32.COINIT_APARTMENTTHREADED)
	if win32.FAILED(hr) {
		return windowsTrashHRESULT("initialize COM apartment", hr)
	}
	defer win32.CoUninitialize()

	var operation *win32.IFileOperation
	hr = win32.CoCreateInstance(
		&clsidFileOperation,
		nil,
		win32.CLSCTX_INPROC_SERVER,
		&win32.IID_IFileOperation,
		unsafe.Pointer(&operation),
	)
	if win32.FAILED(hr) {
		return windowsTrashHRESULT("create IFileOperation", hr)
	}
	defer operation.Release()

	flags := uint32(win32.FOF_SILENT | win32.FOF_NOCONFIRMATION | win32.FOF_NOCONFIRMMKDIR | win32.FOF_NOERRORUI |
		win32.FOFX_RECYCLEONDELETE | win32.FOFX_EARLYFAILURE | win32.FOFX_ADDUNDORECORD)
	if hr = operation.SetOperationFlags(flags); win32.FAILED(hr) {
		return windowsTrashHRESULT("set recycle operation flags", hr)
	}

	pathPtr, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return err
	}
	var item *win32.IShellItem
	hr = win32.SHCreateItemFromParsingName(win32.PWSTR(pathPtr), nil, &win32.IID_IShellItem, unsafe.Pointer(&item))
	if win32.FAILED(hr) {
		return windowsTrashHRESULT("create shell item", hr)
	}
	defer item.Release()

	if err := ctx.Err(); err != nil {
		return err
	}
	if hr = operation.DeleteItem(item, nil); win32.FAILED(hr) {
		return windowsTrashHRESULT("queue recycle operation", hr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	performHR := operation.PerformOperations()
	var aborted win32.BOOL
	abortedHR := operation.GetAnyOperationsAborted(&aborted)
	if !win32.FAILED(performHR) && !win32.FAILED(abortedHR) && aborted == 0 {
		// Once the native operation has definitely completed, a concurrent
		// context cancellation must not turn success into a retryable error.
		return nil
	}

	var resultErrs []error
	if win32.FAILED(performHR) {
		resultErrs = append(resultErrs, windowsTrashHRESULT("perform recycle operation", performHR))
	}
	if win32.FAILED(abortedHR) {
		resultErrs = append(resultErrs, windowsTrashHRESULT("query recycle operation result", abortedHR))
	} else if aborted != 0 {
		resultErrs = append(resultErrs, fmt.Errorf("Windows recycle operation was aborted"))
	}
	resultErr := errors.Join(resultErrs...)

	// Microsoft requires querying GetAnyOperationsAborted even when
	// PerformOperations fails. If the source is still present, retrying is
	// safe; if it vanished, we cannot prove whether it reached the Recycle Bin
	// or was removed by another actor, so expose an explicit unknown state.
	if _, statErr := os.Lstat(abs); statErr == nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return resultErr
	}
	return &UnknownOperationStateError{Operation: "Windows recycle operation", Err: resultErr}
}

func windowsTrashHRESULT(operation string, hr win32.HRESULT) error {
	return fmt.Errorf("%s: %s", operation, win32.HRESULT_ToString(hr))
}
