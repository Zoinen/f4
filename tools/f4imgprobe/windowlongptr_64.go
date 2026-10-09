//go:build windows && !386 && !arm

package main

const getWindowLongPtrProc = "GetWindowLongPtrW"
