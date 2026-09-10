//go:build windows && (amd64 || arm64)

package gui

import (
	"syscall"
	"testing"
	"unsafe"
)

func TestWinePathStringHelpers(t *testing.T) {
	raw := []byte("unix/path\x00ignored")
	if got := stringFromCString(uintptr(unsafe.Pointer(&raw[0]))); got != "unix/path" {
		t.Fatalf("stringFromCString() = %q, want unix/path", got)
	}

	wide := []uint16{'D', ':', '\\', 't', 'm', 'p', 0, 'x'}
	if got := stringFromWideCString(uintptr(unsafe.Pointer(&wide[0]))); got != "D:\\tmp" {
		t.Fatalf("stringFromWideCString() = %q, want D:\\tmp", got)
	}
}

func TestWinePathUnavailableInputs(t *testing.T) {
	freeProcessHeap(0)

	if got, ok := HostUnixPath(""); got != "" || ok {
		t.Fatalf("HostUnixPath(empty) = %q, %v; want empty, false", got, ok)
	}
	if got, ok := HostDosPath(""); got != "" || ok {
		t.Fatalf("HostDosPath(empty) = %q, %v; want empty, false", got, ok)
	}

	if got, ok := HostUnixPath("contains\x00nul"); got != "" || ok {
		t.Fatalf("HostUnixPath(NUL) = %q, %v; want empty, false", got, ok)
	}
	if got, ok := HostDosPath("contains\x00nul"); got != "" || ok {
		t.Fatalf("HostDosPath(NUL) = %q, %v; want empty, false", got, ok)
	}
}

func TestWinePathUTF16Error(t *testing.T) {
	if _, err := syscall.UTF16PtrFromString("contains\x00nul"); err == nil {
		t.Fatal("UTF16PtrFromString accepted an embedded NUL")
	}
}
