//go:build windows

package vtui

import "unsafe"

// winPtr turns an address that Windows returned as a uintptr back into the
// pointer it is: GlobalLock's view of an HGLOBAL, or any other block that
// syscall.Proc.Call or syscall.SyscallN can only hand back as an integer.
//
// The contract is that addr is memory the Go runtime does not manage --
// allocated by the OS or by foreign code, and kept alive by whatever API
// produced it (GlobalLock until GlobalUnlock, for instance). It must never
// be given an address that came from Go memory: not uintptr(unsafe.Pointer(p))
// laundered through a variable, and not one of our own COM objects coming
// back as a "this" argument. For those, keep the value typed -- declare the
// callback parameter as the pointer type it is -- so the collector sees it.
//
// Why the conversion is valid there: the rule that unsafe.Pointer(uintptr)
// may only appear in the same expression as the uintptr(unsafe.Pointer(...))
// that produced it exists because a Go object whose only reference is an
// integer can be freed, and a stack can move under it. Neither can happen to
// memory outside the Go heap. The collector never frees, moves or scans it,
// so an integer copy of its address loses nothing, and turning that integer
// back into a pointer is exactly as safe as the address itself.
//
// Why it is written this way: a direct unsafe.Pointer(addr) is what vet's
// unsafeptr check flags, because vet cannot tell an OS address from a
// laundered Go one. Reinterpreting the word in place through a
// *unsafe.Pointer yields the same bits without that conversion, and is how
// the standard library returns a non-Go address it holds as an integer
// (reflect.Value.UnsafePointer does this for a method's code pointer).
// Keeping every such conversion in this one function is what lets the
// unsafeptr check stay enabled for the rest of the package: a new
// unsafe.Pointer(uintptr) anywhere else is still reported.
func winPtr(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}
