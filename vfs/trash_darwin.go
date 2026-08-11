//go:build darwin

package vfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	foundationOnce sync.Once
	foundationErr  error
	objcGetClass   func(name string) uintptr
	objcRegister   func(name string) uintptr
	objcMsgSend    func(object uintptr, selector uintptr, args ...any) uintptr
)

var _ TrashVFS = (*OSVFS)(nil)

// MoveToTrash calls NSFileManager's trashItemAtURL API directly through the
// project's pureffi runtime, retaining CGO_ENABLED=0 builds and native Trash
// semantics without shelling out to Finder or sudo.
func (v *OSVFS) MoveToTrash(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	abs, err := v.Abs(path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	sourceInfo, err := os.Lstat(abs)
	if err != nil {
		return err
	}

	foundationOnce.Do(func() {
		_, foundationErr = purego.Dlopen(
			"/System/Library/Frameworks/Foundation.framework/Foundation",
			purego.RTLD_GLOBAL|purego.RTLD_NOW,
		)
		if foundationErr != nil {
			return
		}
		objcLib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			foundationErr = err
			return
		}
		purego.RegisterLibFunc(&objcGetClass, objcLib, "objc_getClass")
		purego.RegisterLibFunc(&objcRegister, objcLib, "sel_registerName")
		msgSend, err := purego.Dlsym(objcLib, "objc_msgSend")
		if err != nil {
			foundationErr = err
			return
		}
		purego.RegisterFunc(&objcMsgSend, msgSend)
	})
	if foundationErr != nil {
		return fmt.Errorf("load Foundation: %w", foundationErr)
	}

	// The autorelease pool and every object it owns are thread-affine.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	pool := darwinSend(darwinSend(objcGetClass("NSAutoreleasePool"), "alloc"), "init")
	if pool != 0 {
		defer darwinSend(pool, "drain")
	}

	pathBytes := append([]byte(abs), 0)
	isDirectory := uintptr(0)
	if sourceInfo.IsDir() {
		isDirectory = 1
	}
	fileURL := darwinSend(
		objcGetClass("NSURL"),
		"fileURLWithFileSystemRepresentation:isDirectory:relativeToURL:",
		unsafe.Pointer(&pathBytes[0]),
		isDirectory,
		uintptr(0),
	)
	runtime.KeepAlive(pathBytes)
	if fileURL == 0 {
		return fmt.Errorf("cannot create file URL for trash")
	}
	manager := darwinSend(objcGetClass("NSFileManager"), "defaultManager")
	if manager == 0 {
		return fmt.Errorf("NSFileManager is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	var nsError uintptr
	ok := darwinSend(
		manager,
		"trashItemAtURL:resultingItemURL:error:",
		fileURL,
		uintptr(0),
		unsafe.Pointer(&nsError),
	)
	if ok != 0 {
		return nil
	}
	if nsError != 0 {
		description := darwinSend(nsError, "localizedDescription")
		utf8 := darwinSend(description, "UTF8String")
		return fmt.Errorf("macOS Trash: %s", darwinCString(utf8))
	}
	return fmt.Errorf("macOS Trash rejected the item without an error description")
}

func darwinSend(object uintptr, selector string, args ...any) uintptr {
	return objcMsgSend(object, objcRegister(selector), args...)
}

func darwinCString(pointer uintptr) string {
	if pointer == 0 {
		return ""
	}
	const maxErrorDescription = 1 << 20
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(pointer)), maxErrorDescription)
	for i, b := range bytes {
		if b == 0 {
			return string(bytes[:i])
		}
	}
	return string(bytes)
}
