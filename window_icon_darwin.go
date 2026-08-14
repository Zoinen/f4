//go:build darwin

package main

import (
	_ "embed"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/unxed/vtui"
)

//go:embed assets/icon/generated/f4-512.png
var darwinDockIconPNG []byte

// applyDarwinDockIcon gives the Dock tile an image. gogpu switches the
// process to NSApplicationActivationPolicyRegular, which is what makes the
// tile appear at all, but the tile's image comes from the app bundle's
// Info.plist — and a bare binary launched from a terminal has no bundle, so
// the Dock shows the generic executable icon. [NSApp setApplicationIconImage:]
// is the runtime override for exactly that case; when running from F4.app it
// simply matches the bundled icns.
//
// AppKit calls must happen on the main OS thread: the main goroutine is
// pinned to it by gogpu's darwin platform package init, and RunGui runs on
// the main goroutine before handing it to the Cocoa event loop.
func applyDarwinDockIcon(backend string) {
	if backend != "gogpu" && backend != "ebiten" {
		// x11/wayland windows (XQuartz) are not Cocoa apps; leave AppKit alone.
		return
	}
	if len(darwinDockIconPNG) == 0 {
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

	nsApp := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	if nsApp == 0 {
		return
	}
	// alloc/init rather than the autoreleasing dataWithBytes:length: — this
	// runs before gogpu sets up an NSAutoreleasePool, so an autoreleased
	// object would never be drained.
	data := objc.ID(objc.GetClass("NSData")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithBytes:length:"),
		unsafe.Pointer(&darwinDockIconPNG[0]), len(darwinDockIconPNG))
	if data == 0 {
		return
	}
	img := objc.ID(objc.GetClass("NSImage")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithData:"), data)
	data.Send(objc.RegisterName("release"))
	if img == 0 {
		return
	}
	nsApp.Send(objc.RegisterName("setApplicationIconImage:"), img)
	img.Send(objc.RegisterName("release"))
}
