//go:build windows && (386 || arm)

package main

// See internal/wincon: 32-bit Windows exports GetWindowLongW, not the Ptr
// name, which is a header macro there.
const getWindowLongPtrProc = "GetWindowLongW"
