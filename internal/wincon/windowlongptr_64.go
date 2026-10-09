//go:build windows && !386 && !arm

package wincon

// On 64-bit Windows the pointer-sized accessor is a real export and is the
// one to use: GetWindowLongW truncates a LONG_PTR value to 32 bits there.
const getWindowLongPtrProc = "GetWindowLongPtrW"
