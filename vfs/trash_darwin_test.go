//go:build darwin

package vfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

func installDarwinTestStubs(t *testing.T, send func(object uintptr, selector string, args ...any) uintptr) {
	t.Helper()
	oldOnce, oldErr := foundationOnce, foundationErr
	oldGetClass, oldRegister, oldMsgSend := objcGetClass, objcRegister, objcMsgSend
	var initialized sync.Once
	initialized.Do(func() {})
	foundationOnce = initialized
	foundationErr = nil
	selectors := make(map[string]uintptr)
	var nextSelector uintptr = 100
	objcGetClass = func(name string) uintptr {
		switch name {
		case "NSAutoreleasePool":
			return 1
		case "NSURL":
			return 2
		case "NSFileManager":
			return 3
		default:
			return 0
		}
	}
	objcRegister = func(name string) uintptr {
		if selector, ok := selectors[name]; ok {
			return selector
		}
		nextSelector++
		selectors[name] = nextSelector
		return nextSelector
	}
	objcMsgSend = func(object uintptr, selector uintptr, args ...any) uintptr {
		for name, value := range selectors {
			if value == selector {
				return send(object, name, args...)
			}
		}
		t.Fatalf("unknown Objective-C selector %d", selector)
		return 0
	}
	t.Cleanup(func() {
		foundationOnce, foundationErr = oldOnce, oldErr
		objcGetClass, objcRegister, objcMsgSend = oldGetClass, oldRegister, oldMsgSend
	})
}

func TestDarwinHelpers(t *testing.T) {
	installDarwinTestStubs(t, func(object uintptr, selector string, args ...any) uintptr {
		if object != 7 || selector != "testSelector" || len(args) != 1 || args[0] != "argument" {
			t.Fatalf("unexpected Objective-C call: object=%d selector=%q args=%v", object, selector, args)
		}
		return 42
	})
	if got := darwinSend(7, "testSelector", "argument"); got != 42 {
		t.Fatalf("darwinSend() = %d, want 42", got)
	}
	if got := darwinCString(0); got != "" {
		t.Fatalf("darwinCString(nil) = %q, want empty string", got)
	}
	cstring := []byte("native message\x00ignored")
	if got := darwinCString(uintptr(unsafe.Pointer(&cstring[0]))); got != "native message" {
		t.Fatalf("darwinCString() = %q, want native message", got)
	}
}

func TestDarwinMoveToTrashPreflight(t *testing.T) {
	filesystem := NewOSVFS(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := filesystem.MoveToTrash(ctx, "missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled MoveToTrash() = %v, want context.Canceled", err)
	}
	if err := filesystem.MoveToTrash(context.Background(), "missing"); !os.IsNotExist(err) {
		t.Fatalf("missing MoveToTrash() = %v, want not-exist error", err)
	}
}

func TestDarwinMoveToTrashFoundationError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "item"), []byte("item"), 0600); err != nil {
		t.Fatal(err)
	}
	oldOnce, oldErr := foundationOnce, foundationErr
	var initialized sync.Once
	initialized.Do(func() {})
	foundationOnce = initialized
	foundationErr = errors.New("Foundation unavailable")
	t.Cleanup(func() { foundationOnce, foundationErr = oldOnce, oldErr })
	filesystem := NewOSVFS(root)
	err := filesystem.MoveToTrash(context.Background(), "item")
	if err == nil || !strings.Contains(err.Error(), "load Foundation: Foundation unavailable") {
		t.Fatalf("MoveToTrash() = %v, want Foundation loading error", err)
	}
}

func TestDarwinMoveToTrashSuccessForFilesAndDirectories(t *testing.T) {
	fileURLDirectories := make([]uintptr, 0, 2)
	installDarwinTestStubs(t, func(object uintptr, selector string, args ...any) uintptr {
		switch selector {
		case "alloc":
			return 10
		case "init":
			return 11
		case "drain":
			return 0
		case "fileURLWithFileSystemRepresentation:isDirectory:relativeToURL:":
			fileURLDirectories = append(fileURLDirectories, args[1].(uintptr))
			return 20
		case "defaultManager":
			return 30
		case "trashItemAtURL:resultingItemURL:error:":
			return 1
		default:
			t.Fatalf("unexpected Objective-C selector %q", selector)
			return 0
		}
	})
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	filesystem := NewOSVFS(root)
	if err := filesystem.MoveToTrash(context.Background(), "file"); err != nil {
		t.Fatalf("file MoveToTrash() = %v", err)
	}
	if err := filesystem.MoveToTrash(context.Background(), "directory"); err != nil {
		t.Fatalf("directory MoveToTrash() = %v", err)
	}
	if len(fileURLDirectories) != 2 || fileURLDirectories[0] != 0 || fileURLDirectories[1] != 1 {
		t.Fatalf("isDirectory arguments = %v, want [0 1]", fileURLDirectories)
	}
}

func TestDarwinMoveToTrashErrors(t *testing.T) {
	tests := []struct {
		name       string
		selector   string
		want       string
		setNSError bool
	}{
		{name: "file URL", selector: "fileURLWithFileSystemRepresentation:isDirectory:relativeToURL:", want: "cannot create file URL"},
		{name: "manager", selector: "defaultManager", want: "NSFileManager is unavailable"},
		{name: "without description", selector: "trashItemAtURL:resultingItemURL:error:", want: "Trash rejected the item"},
		{name: "with description", selector: "trashItemAtURL:resultingItemURL:error:", want: "macOS Trash: native failure", setNSError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var message = []byte("native failure\x00")
			installDarwinTestStubs(t, func(object uintptr, selector string, args ...any) uintptr {
				switch selector {
				case "alloc":
					return 10
				case "init":
					return 11
				case "drain":
					return 0
				case "fileURLWithFileSystemRepresentation:isDirectory:relativeToURL:":
					if test.selector == selector {
						return 0
					}
					return 20
				case "defaultManager":
					if test.selector == selector {
						return 0
					}
					return 30
				case "trashItemAtURL:resultingItemURL:error:":
					if test.setNSError {
						*(*uintptr)(args[2].(unsafe.Pointer)) = 40
					}
					return 0
				case "localizedDescription":
					return 41
				case "UTF8String":
					return uintptr(unsafe.Pointer(&message[0]))
				default:
					t.Fatalf("unexpected Objective-C selector %q", selector)
					return 0
				}
			})
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "item"), []byte("item"), 0600); err != nil {
				t.Fatal(err)
			}
			err := NewOSVFS(root).MoveToTrash(context.Background(), "item")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("MoveToTrash() = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestDarwinMoveToTrashCancellationAfterURL(t *testing.T) {
	var cancel context.CancelFunc
	installDarwinTestStubs(t, func(object uintptr, selector string, args ...any) uintptr {
		switch selector {
		case "alloc":
			return 10
		case "init":
			return 11
		case "drain":
			return 0
		case "fileURLWithFileSystemRepresentation:isDirectory:relativeToURL:":
			cancel()
			return 20
		case "defaultManager":
			return 30
		case "trashItemAtURL:resultingItemURL:error:":
			return 1
		default:
			t.Fatalf("unexpected Objective-C selector %q", selector)
			return 0
		}
	})
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "item"), []byte("item"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	cancel = cancelFunc
	if err := NewOSVFS(root).MoveToTrash(ctx, "item"); !errors.Is(err, context.Canceled) {
		t.Fatalf("MoveToTrash() = %v, want context.Canceled", err)
	}
}
