//go:build windows && (386 || arm)

package wincon

// GetWindowLongPtrW does not exist on 32-bit Windows. The Win32 headers
// define GetWindowLongPtr as a macro that expands to GetWindowLong there --
// LONG and LONG_PTR are the same width -- so user32.dll exports only
// GetWindowLongA/W, and asking for the Ptr name gives a LazyProc that panics
// the first time it is called ("Failed to find GetWindowLongPtrW procedure
// in user32.dll"). Measured on ReactOS 0.4.16 (NT 5.2, x86); 32-bit Windows
// XP through 10 export the same set.
const getWindowLongPtrProc = "GetWindowLongW"
