//go:build windows

package main

import "os"

// Windows has no window size ioctl and no X session behind the console, so
// nothing here measures anything. The overlay is a local X affair and never
// runs on this side; the stubs exist so the portable half compiles.

func hostPixelsFromIoctl(*os.File) (int, int, bool) { return 0, 0, false }

func queryPixels(string, string) (int, int, bool) { return 0, 0, false }
