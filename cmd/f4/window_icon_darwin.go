//go:build darwin

package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/unxed/vtui"
	"golang.org/x/sys/unix"
)

//go:embed assets/icon/generated/f4.icns
var darwinIconICNS []byte

// applyDarwinDockIcon gives a bare-binary launch a real icon instead of the
// generic green "exec" tile.
//
// gogpu switches the process to NSApplicationActivationPolicyRegular, which is
// what makes the Dock tile appear at all, but the tile's image comes from the
// app bundle — and a binary launched from a terminal has no bundle. The
// documented runtime override for that case is
// [NSApp setApplicationIconImage:], but on macOS 26 it has no effect for a
// bundle-less process: the Dock and NSRunningApplication.icon both keep
// returning the generic executable icon, whether it is called before or after
// finishLaunching, and whether or not a bundle is present.
//
// What does work is stamping a custom icon onto the executable file itself via
// [NSWorkspace setIcon:forFile:options:]. Launch Services picks that up
// immediately — the running process's Dock tile updates in place — and Finder
// shows the same icon for the binary from then on. The stamp lives in the
// file's com.apple.FinderInfo/com.apple.ResourceFork extended attributes; it
// does not touch the executable's contents or its code signature.
//
// Running from F4.app needs none of this: the bundle's CFBundleIconFile
// already supplies the icon, so the stamp is skipped there.
//
// AppKit calls must happen on the main OS thread: the main goroutine is
// pinned to it by gogpu's darwin platform package init, and RunGui runs on
// the main goroutine before handing it to the Cocoa event loop.
func applyDarwinDockIcon(backend string) {
	if backend != "gogpu" && backend != "ebiten" {
		// x11/wayland windows (XQuartz) are not Cocoa apps; leave AppKit alone.
		return
	}
	if len(darwinIconICNS) == 0 {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		vtui.DebugLog("DOCK_ICON: os.Executable failed: %v", err)
		return
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if strings.Contains(exe, ".app/Contents/MacOS/") {
		// Bundled: Info.plist's CFBundleIconFile already covers Dock and Finder.
		return
	}
	if hasCustomIconFlag(exe) {
		// Already stamped by an earlier run; rewriting the resource fork on
		// every start would be pure disk churn.
		return
	}

	defer func() {
		if r := recover(); r != nil {
			vtui.DebugLog("DOCK_ICON: skipped after panic: %v", r)
		}
	}()

	// gogpu dlopens AppKit inside app.Run(); this runs earlier, so load it
	// here. dlopen of an already-loaded framework just bumps a refcount.
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		vtui.DebugLog("DOCK_ICON: AppKit dlopen failed: %v", err)
		return
	}

	// alloc/init rather than the autoreleasing dataWithBytes:length: — this
	// runs before gogpu sets up an NSAutoreleasePool, so an autoreleased
	// object would never be drained.
	data := objc.ID(objc.GetClass("NSData")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithBytes:length:"),
		// #nosec G103 -- NSData copies this non-empty embedded byte slice during the synchronous initializer call.
		unsafe.Pointer(&darwinIconICNS[0]), len(darwinIconICNS))
	if data == 0 {
		return
	}
	img := objc.ID(objc.GetClass("NSImage")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithData:"), data)
	data.Send(objc.RegisterName("release"))
	if img == 0 {
		vtui.DebugLog("DOCK_ICON: icns decode failed")
		return
	}
	defer img.Send(objc.RegisterName("release"))

	path := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), append([]byte(exe), 0))
	ws := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
	if ws.Send(objc.RegisterName("setIcon:forFile:options:"), img, path, 0) == 0 {
		// Read-only location (/usr/local/bin without write access, a signed
		// and sealed volume, ...). Nothing else to try; the tile stays generic.
		vtui.DebugLog("DOCK_ICON: setIcon:forFile: rejected %s", exe)
	}
}

// hasCustomIconFlag reports whether the file already carries a custom icon.
// Finder stores its flags in bytes 8..9 of com.apple.FinderInfo, big-endian;
// bit 10 (0x0400) is kHasCustomIcon.
func hasCustomIconFlag(path string) bool {
	var info [32]byte
	n, err := unix.Getxattr(path, "com.apple.FinderInfo", info[:])
	if err != nil || n < 10 {
		return false
	}
	return info[8]&0x04 != 0
}
