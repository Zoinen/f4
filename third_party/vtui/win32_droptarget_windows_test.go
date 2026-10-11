//go:build windows && (amd64 || arm64)

package vtui

import (
	"reflect"
	"testing"
	"unsafe"
)

// lockedGlobal allocates a zeroed HGLOBAL of size bytes and locks it, which
// is the same memory a drop source hands a target: owned by Windows, reached
// only through the address GlobalLock returns.
func lockedGlobal(t *testing.T, size uintptr) unsafe.Pointer {
	t.Helper()
	const gmemMoveable, gmemZeroInit = 0x0002, 0x0040
	h, _, _ := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, size)
	if h == 0 {
		t.Fatal("GlobalAlloc failed")
	}
	t.Cleanup(func() { _, _, _ = procGlobalFree.Call(h) })
	return lockGlobal(t, h)
}

// lockGlobal locks h for the rest of the test and returns its address.
func lockGlobal(t *testing.T, h uintptr) unsafe.Pointer {
	t.Helper()
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		t.Fatal("GlobalLock failed")
	}
	t.Cleanup(func() { _, _, _ = procGlobalUnlock.Call(h) })
	return winPtr(ptr)
}

// The block buildHDROP writes must read back through parseHDROP unchanged:
// both sides reach GlobalLock's memory through winPtr, and every name after
// the first sits at an offset computed with unsafe.Add.
func TestWin32DnD_ParseHDROPRoundTrip(t *testing.T) {
	want := []string{`C:\test1.txt`, `C:\каталог\файл с пробелом.txt`}
	h, err := buildHDROP(want)
	if err != nil {
		t.Fatalf("buildHDROP failed: %v", err)
	}
	t.Cleanup(func() { _, _, _ = procGlobalFree.Call(h) })

	size, _, _ := procGlobalSize.Call(h)
	if got := parseHDROP(lockGlobal(t, h), size); !reflect.DeepEqual(got, want) {
		t.Errorf("parseHDROP = %q, want %q", got, want)
	}
}

// size is the only thing that bounds the walk, so a list whose terminators
// lie past it must stop at it rather than read on.
func TestWin32DnD_ParseHDROPStopsAtSize(t *testing.T) {
	name := []uint16{'a', '.', 't', 'x', 't'}
	const header = unsafe.Sizeof(dropFiles{})
	full := header + uintptr(len(name)+2)*2
	base := lockedGlobal(t, full)

	df := (*dropFiles)(base)
	df.pFiles = uint32(header)
	df.fWide = 1
	copy(unsafe.Slice((*uint16)(unsafe.Add(base, header)), len(name)), name)

	truncated := header + uintptr(len(name))*2
	if got := parseHDROP(base, truncated); !reflect.DeepEqual(got, []string{"a.txt"}) {
		t.Errorf("parseHDROP bounded before the terminator = %q, want [\"a.txt\"]", got)
	}
}

// A source may still hand over the ANSI form of DROPFILES, whose names are
// bytes rather than UTF-16 units.
func TestWin32DnD_ParseHDROPAnsi(t *testing.T) {
	list := "a.txt\x00b\x00\x00"
	const header = unsafe.Sizeof(dropFiles{})
	size := header + uintptr(len(list))
	base := lockedGlobal(t, size)

	df := (*dropFiles)(base)
	df.pFiles = uint32(header)
	copy(unsafe.Slice((*byte)(unsafe.Add(base, header)), len(list)), list)

	if got := parseHDROP(base, size); !reflect.DeepEqual(got, []string{"a.txt", "b"}) {
		t.Errorf("parseHDROP of an ANSI list = %q, want [\"a.txt\" \"b\"]", got)
	}
}
