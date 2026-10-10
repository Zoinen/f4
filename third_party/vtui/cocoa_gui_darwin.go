//go:build darwin && !ios && !vtui_nococoa

package vtui

import (
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/unxed/vtinput"
)

// The Cocoa backend: an NSApplication with one NSWindow whose content view
// is a class of our own, registered with the Objective-C runtime through
// purego, so no cgo is involved anywhere. The grid is composed on the CPU by
// the raster the Win32 backend uses and handed to Core Animation as the
// contents of the view's layer, one CGImage per displayed frame.
//
// AppKit is single-threaded: windows, views, layers and the event loop all
// belong to the main thread. The rules this file keeps are therefore:
//
//   - RunCocoaGuiHost runs on the main thread (see init below) and ends up in
//     [NSApp run]; every Objective-C callback into this file arrives there.
//   - FrameManager renders on its own goroutine into the renderer's bitmap,
//     under the renderer's lock, exactly as it does for the Win32 backend.
//   - Anything that goroutine needs done to the window -- a display pass, a
//     new title, a resize, the final stop -- is queued and handed to the main
//     thread with performSelectorOnMainThread:, never done in place.
//   - Input handlers on the main thread never block: they post into the
//     vtinput channel without waiting.

func init() {
	// AppKit runs on the process's main thread and nowhere else, and the main
	// goroutine only stays on that thread if it is locked there before main
	// starts. gogpu and Ebitengine do the same in their own darwin packages;
	// this is what keeps it true in a build that leaves both of them out.
	runtime.LockOSThread()
}

type cocoaPoint struct{ X, Y float64 }
type cocoaSize struct{ Width, Height float64 }
type cocoaRect struct {
	Origin cocoaPoint
	Size   cocoaSize
}

const (
	nsWindowStyleMaskTitled         = 1 << 0
	nsWindowStyleMaskClosable       = 1 << 1
	nsWindowStyleMaskMiniaturizable = 1 << 2
	nsWindowStyleMaskResizable      = 1 << 3

	nsBackingStoreBuffered                 = 2
	nsApplicationActivationPolicyRegular   = 0
	nsWindowTabbingModeDisallowed          = 2
	nsViewLayerContentsRedrawOnSetNeedsDsp = 1
	nsViewLayerContentsPlacementTopLeft    = 11
	nsTerminateCancel                      = 0
	nsTerminateNow                         = 1
	nsTerminateLater                       = 2
	nsUTF8StringEncoding                   = 4

	nsEventTypeKeyDown            = 10
	nsEventTypeApplicationDefined = 15

	// RGBX, eight bits a channel, in the byte order image.RGBA keeps.
	cgImageAlphaNoneSkipLast = 5
)

// The Objective-C classes and selectors this file uses, resolved once.
var cocoaRT struct {
	once sync.Once
	err  error

	classNSApplication       objc.Class
	classNSWindow            objc.Class
	classNSEvent             objc.Class
	classNSString            objc.Class
	classNSScreen            objc.Class
	classNSMenu              objc.Class
	classNSMenuItem          objc.Class
	classNSColor             objc.Class
	classView                objc.Class
	classWindowDelegate      objc.Class
	classApplicationDelegate objc.Class

	colorSpace uintptr
}

var cocoaSel struct {
	alloc, init, release                     objc.SEL
	sharedApplication, setActivationPolicy   objc.SEL
	activateIgnoringOtherApps, run, stop     objc.SEL
	postEventAtStart, mainMenu, setMainMenu  objc.SEL
	delegate, setDelegate                    objc.SEL
	mainScreen, screens, objectAtIndex       objc.SEL
	backingScaleFactor, frame                objc.SEL
	initWithContentRect, setTitle            objc.SEL
	setContentView, makeFirstResponder       objc.SEL
	setAcceptsMouseMovedEvents               objc.SEL
	setReleasedWhenClosed, setRestorable     objc.SEL
	center, makeKeyAndOrderFront, orderOut   objc.SEL
	setContentSize, setContentMinSize        objc.SEL
	setContentResizeIncrements, zoom, close  objc.SEL
	setFrame, setFrameTopLeftPoint           objc.SEL
	setBackgroundColor                       objc.SEL
	blackColor, respondsToSelector           objc.SEL
	setTabbingMode                           objc.SEL
	convertRectFromScreen, window            objc.SEL
	initWithFrame, setWantsLayer, layer      objc.SEL
	setLayerContentsRedrawPolicy             objc.SEL
	setLayerContentsPlacement                objc.SEL
	setNeedsDisplay, bounds, convertPoint    objc.SEL
	performOnMain, setContents               objc.SEL
	setContentsScale                         objc.SEL
	keyCode, modifierFlags, characters       objc.SEL
	charactersIgnoringModifiers, eventType   objc.SEL
	locationInWindow, clickCount             objc.SEL
	buttonNumber, scrollingDeltaY            objc.SEL
	hasPreciseScrollingDeltas, otherEvent    objc.SEL
	initWithUTF8String, lengthOfBytes        objc.SEL
	getCString                               objc.SEL
	initWithTitle, addItem, addItemWithTitle objc.SEL
	separatorItem, setSubmenu                objc.SEL
	hide, terminate, performKeyEquivalent    objc.SEL
	runQueued                                objc.SEL
	replyToApplicationShouldTerminate        objc.SEL
	vtuiReplyTerminate                       objc.SEL
}

// Plain C functions: Core Graphics for the frame, libobjc for autorelease
// pools on goroutines, libSystem to tell the main thread from the others.
var (
	cgColorSpaceCreateDeviceRGB   func() uintptr
	cgColorSpaceCreateWithName    func(name uintptr) uintptr
	cgBitmapContextCreate         func(data unsafe.Pointer, width, height, bitsPerComponent, bytesPerRow uintptr, space uintptr, bitmapInfo uint32) uintptr
	cgBitmapContextGetData        func(ctx uintptr) unsafe.Pointer
	cgBitmapContextGetBytesPerRow func(ctx uintptr) uintptr
	cgBitmapContextCreateImage    func(ctx uintptr) uintptr
	cgContextRelease              func(ctx uintptr)
	cgImageRelease                func(image uintptr)
	objcAutoreleasePoolPush       func() uintptr
	objcAutoreleasePoolPop        func(pool uintptr)
	pthreadMainNP                 func() int32
)

// cocoaHosts maps the view and the window delegate to the host they belong
// to; cocoaActiveHost is the host of the running loop, for the application
// delegate, which belongs to no window. cocoaAppDelegate is NSApp's
// delegate, set once in RunCocoaGuiHost's setup and read from any thread
// after that (see cocoaAppDelegate's use in cocoaReplyOnceClosed): the `go`
// statement that starts a goroutine after that point happens-after the
// assignment, so this needs no lock.
var (
	cocoaHosts       sync.Map
	cocoaActiveHost  atomic.Pointer[CocoaGuiHost]
	cocoaAppDelegate objc.ID
)

func cocoaHostFor(id objc.ID) *CocoaGuiHost {
	if v, ok := cocoaHosts.Load(id); ok {
		return v.(*CocoaGuiHost)
	}
	return nil
}

// loadCocoa loads AppKit and Core Graphics, resolves what this file calls
// and registers the classes, once per process.
func loadCocoa() error {
	cocoaRT.once.Do(func() {
		cocoaRT.err = loadCocoaOnce()
	})
	return cocoaRT.err
}

func loadCocoaOnce() error {
	// With cgo off nothing links the frameworks in, so the classes do not
	// exist until the frameworks are loaded by hand.
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		return fmt.Errorf("cocoa: loading AppKit: %w", err)
	}
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("cocoa: loading CoreGraphics: %w", err)
	}
	libobjc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("cocoa: loading libobjc: %w", err)
	}
	libSystem, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("cocoa: loading libSystem: %w", err)
	}
	for _, fn := range []struct {
		lib  uintptr
		name string
		ptr  any
	}{
		{cg, "CGColorSpaceCreateDeviceRGB", &cgColorSpaceCreateDeviceRGB},
		{cg, "CGColorSpaceCreateWithName", &cgColorSpaceCreateWithName},
		{cg, "CGBitmapContextCreate", &cgBitmapContextCreate},
		{cg, "CGBitmapContextGetData", &cgBitmapContextGetData},
		{cg, "CGBitmapContextGetBytesPerRow", &cgBitmapContextGetBytesPerRow},
		{cg, "CGBitmapContextCreateImage", &cgBitmapContextCreateImage},
		{cg, "CGContextRelease", &cgContextRelease},
		{cg, "CGImageRelease", &cgImageRelease},
		{libobjc, "objc_autoreleasePoolPush", &objcAutoreleasePoolPush},
		{libobjc, "objc_autoreleasePoolPop", &objcAutoreleasePoolPop},
		{libSystem, "pthread_main_np", &pthreadMainNP},
	} {
		sym, err := purego.Dlsym(fn.lib, fn.name)
		if err != nil {
			return fmt.Errorf("cocoa: resolving %s: %w", fn.name, err)
		}
		purego.RegisterFunc(fn.ptr, sym)
	}

	s := &cocoaSel
	for _, sel := range []struct {
		dst  *objc.SEL
		name string
	}{
		{&s.alloc, "alloc"}, {&s.init, "init"}, {&s.release, "release"},
		{&s.sharedApplication, "sharedApplication"},
		{&s.setActivationPolicy, "setActivationPolicy:"},
		{&s.activateIgnoringOtherApps, "activateIgnoringOtherApps:"},
		{&s.run, "run"}, {&s.stop, "stop:"},
		{&s.postEventAtStart, "postEvent:atStart:"},
		{&s.mainMenu, "mainMenu"}, {&s.setMainMenu, "setMainMenu:"},
		{&s.delegate, "delegate"}, {&s.setDelegate, "setDelegate:"},
		{&s.mainScreen, "mainScreen"}, {&s.screens, "screens"},
		{&s.objectAtIndex, "objectAtIndex:"},
		{&s.backingScaleFactor, "backingScaleFactor"}, {&s.frame, "frame"},
		{&s.initWithContentRect, "initWithContentRect:styleMask:backing:defer:"},
		{&s.setTitle, "setTitle:"}, {&s.setContentView, "setContentView:"},
		{&s.makeFirstResponder, "makeFirstResponder:"},
		{&s.setAcceptsMouseMovedEvents, "setAcceptsMouseMovedEvents:"},
		{&s.setReleasedWhenClosed, "setReleasedWhenClosed:"},
		{&s.setRestorable, "setRestorable:"},
		{&s.center, "center"}, {&s.makeKeyAndOrderFront, "makeKeyAndOrderFront:"},
		{&s.orderOut, "orderOut:"},
		{&s.setContentSize, "setContentSize:"},
		{&s.setContentMinSize, "setContentMinSize:"},
		{&s.setContentResizeIncrements, "setContentResizeIncrements:"},
		{&s.zoom, "zoom:"}, {&s.close, "close"},
		{&s.setFrame, "setFrame:display:"},
		{&s.setFrameTopLeftPoint, "setFrameTopLeftPoint:"},
		{&s.setBackgroundColor, "setBackgroundColor:"},
		{&s.blackColor, "blackColor"},
		{&s.respondsToSelector, "respondsToSelector:"},
		{&s.setTabbingMode, "setTabbingMode:"},
		{&s.convertRectFromScreen, "convertRectFromScreen:"},
		{&s.window, "window"},
		{&s.initWithFrame, "initWithFrame:"},
		{&s.setWantsLayer, "setWantsLayer:"}, {&s.layer, "layer"},
		{&s.setLayerContentsRedrawPolicy, "setLayerContentsRedrawPolicy:"},
		{&s.setLayerContentsPlacement, "setLayerContentsPlacement:"},
		{&s.setNeedsDisplay, "setNeedsDisplay:"}, {&s.bounds, "bounds"},
		{&s.convertPoint, "convertPoint:fromView:"},
		{&s.performOnMain, "performSelectorOnMainThread:withObject:waitUntilDone:"},
		{&s.setContents, "setContents:"},
		{&s.setContentsScale, "setContentsScale:"},
		{&s.keyCode, "keyCode"}, {&s.modifierFlags, "modifierFlags"},
		{&s.characters, "characters"},
		{&s.charactersIgnoringModifiers, "charactersIgnoringModifiers"},
		{&s.eventType, "type"},
		{&s.locationInWindow, "locationInWindow"},
		{&s.clickCount, "clickCount"}, {&s.buttonNumber, "buttonNumber"},
		{&s.scrollingDeltaY, "scrollingDeltaY"},
		{&s.hasPreciseScrollingDeltas, "hasPreciseScrollingDeltas"},
		{&s.otherEvent, "otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"},
		{&s.initWithUTF8String, "initWithUTF8String:"},
		{&s.lengthOfBytes, "lengthOfBytesUsingEncoding:"},
		{&s.getCString, "getCString:maxLength:encoding:"},
		{&s.initWithTitle, "initWithTitle:"}, {&s.addItem, "addItem:"},
		{&s.addItemWithTitle, "addItemWithTitle:action:keyEquivalent:"},
		{&s.separatorItem, "separatorItem"}, {&s.setSubmenu, "setSubmenu:"},
		{&s.hide, "hide:"}, {&s.terminate, "terminate:"},
		{&s.performKeyEquivalent, "performKeyEquivalent:"},
		{&s.runQueued, "vtuiRunQueued:"},
		{&s.replyToApplicationShouldTerminate, "replyToApplicationShouldTerminate:"},
		{&s.vtuiReplyTerminate, "vtuiReplyTerminate:"},
	} {
		*sel.dst = objc.RegisterName(sel.name)
	}

	for _, c := range []struct {
		dst  *objc.Class
		name string
	}{
		{&cocoaRT.classNSApplication, "NSApplication"},
		{&cocoaRT.classNSWindow, "NSWindow"},
		{&cocoaRT.classNSEvent, "NSEvent"},
		{&cocoaRT.classNSString, "NSString"},
		{&cocoaRT.classNSScreen, "NSScreen"},
		{&cocoaRT.classNSMenu, "NSMenu"},
		{&cocoaRT.classNSMenuItem, "NSMenuItem"},
		{&cocoaRT.classNSColor, "NSColor"},
	} {
		if *c.dst = objc.GetClass(c.name); *c.dst == 0 {
			return fmt.Errorf("cocoa: class %s not found", c.name)
		}
	}

	if err := registerCocoaClasses(); err != nil {
		return err
	}

	// sRGB, so a palette colour means on screen what it means in every
	// terminal; the name is compared by value, so an NSString of our own
	// stands in for the kCGColorSpaceSRGB constant.
	name := cocoaNSString("kCGColorSpaceSRGB")
	cocoaRT.colorSpace = cgColorSpaceCreateWithName(uintptr(name))
	name.Send(cocoaSel.release)
	if cocoaRT.colorSpace == 0 {
		cocoaRT.colorSpace = cgColorSpaceCreateDeviceRGB()
	}
	if cocoaRT.colorSpace == 0 {
		return fmt.Errorf("cocoa: no RGB colour space")
	}
	return nil
}

// registerCocoaClasses registers the content view, the window delegate and
// the application delegate. Every method looks its host up by the receiver
// and does nothing when there is none: a callback can still be queued for a
// window whose loop has ended.
func registerCocoaClasses() error {
	mouse := func(name string, btn uint32, down bool) objc.MethodDef {
		return objc.MethodDef{
			Cmd: objc.RegisterName(name),
			Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
				defer cocoaCallbackRecover(name)
				if h := cocoaHostFor(self); h != nil {
					h.handleMouseButton(self, event, btn, down)
				}
			},
		}
	}
	motion := func(name string) objc.MethodDef {
		return objc.MethodDef{
			Cmd: objc.RegisterName(name),
			Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
				defer cocoaCallbackRecover(name)
				if h := cocoaHostFor(self); h != nil {
					h.handleMouseMotion(self, event)
				}
			},
		}
	}
	yes := func(name string) objc.MethodDef {
		return objc.MethodDef{Cmd: objc.RegisterName(name), Fn: func(objc.ID, objc.SEL) bool { return true }}
	}

	var err error
	cocoaRT.classView, err = objc.RegisterClass("VtuiCocoaView", objc.GetClass("NSView"), nil, nil, []objc.MethodDef{
		yes("acceptsFirstResponder"),
		yes("canBecomeKeyView"),
		yes("isOpaque"),
		yes("wantsUpdateLayer"),
		{Cmd: objc.RegisterName("acceptsFirstMouse:"), Fn: func(objc.ID, objc.SEL, objc.ID) bool { return true }},
		{Cmd: objc.RegisterName("updateLayer"), Fn: func(self objc.ID, _ objc.SEL) {
			defer cocoaCallbackRecover("updateLayer")
			if h := cocoaHostFor(self); h != nil {
				h.presentFrame(self)
			}
		}},
		{Cmd: objc.RegisterName("keyDown:"), Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
			defer cocoaCallbackRecover("keyDown:")
			if h := cocoaHostFor(self); h != nil {
				h.handleKeyDown(event)
			}
		}},
		{Cmd: objc.RegisterName("keyUp:"), Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
			defer cocoaCallbackRecover("keyUp:")
			if h := cocoaHostFor(self); h != nil {
				h.handleKeyUp(event)
			}
		}},
		{Cmd: objc.RegisterName("flagsChanged:"), Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
			defer cocoaCallbackRecover("flagsChanged:")
			if h := cocoaHostFor(self); h != nil {
				h.handleFlagsChanged(event)
			}
		}},
		{Cmd: cocoaSel.performKeyEquivalent, Fn: func(self objc.ID, _ objc.SEL, event objc.ID) bool {
			defer cocoaCallbackRecover("performKeyEquivalent:")
			if h := cocoaHostFor(self); h != nil && h.claimsKeyEquivalent(event) {
				h.handleKeyDown(event)
				return true
			}
			// objc.SendSuper resolves the super class from self's dynamic
			// class (object_getClass(self)), which is exactly what
			// objc_msgSendSuper2 itself expects. That is safe as long as
			// self's dynamic class is really VtuiCocoaView: the runtime
			// then starts the search one level up, at NSView. But if this
			// particular view instance ever gets isa-swizzled -- most
			// commonly by Cocoa's KVO machinery, which installs a private
			// NSKVONotifying_VtuiCocoaView subclass the moment an observer
			// is added to it -- self's dynamic class becomes that
			// subclass, and the same lookup would start one level up from
			// *there*, landing back on VtuiCocoaView itself and calling
			// straight back into this method: unbounded recursion. Guard
			// against that by only forwarding to super while self is
			// still exactly our own registered class; otherwise fall back
			// to NSView's real default, which is simply to not handle the
			// key equivalent.
			if self.Class() == cocoaRT.classView {
				return objc.SendSuper[bool](self, cocoaSel.performKeyEquivalent, event)
			}
			return false
		}},
		mouse("mouseDown:", uint32(vtinput.FromLeft1stButtonPressed), true),
		mouse("mouseUp:", uint32(vtinput.FromLeft1stButtonPressed), false),
		mouse("rightMouseDown:", uint32(vtinput.RightmostButtonPressed), true),
		mouse("rightMouseUp:", uint32(vtinput.RightmostButtonPressed), false),
		mouse("otherMouseDown:", 0, true),
		mouse("otherMouseUp:", 0, false),
		motion("mouseMoved:"),
		motion("mouseDragged:"),
		motion("rightMouseDragged:"),
		motion("otherMouseDragged:"),
		{Cmd: objc.RegisterName("scrollWheel:"), Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
			defer cocoaCallbackRecover("scrollWheel:")
			if h := cocoaHostFor(self); h != nil {
				h.handleScroll(self, event)
			}
		}},

		// Text that arrives without a key press of its own: the Character
		// Viewer and the Services menu hand it to the first responder this
		// way.
		{Cmd: objc.RegisterName("insertText:"), Fn: func(self objc.ID, _ objc.SEL, text objc.ID) {
			defer cocoaCallbackRecover("insertText:")
			if h := cocoaHostFor(self); h != nil {
				h.insertText(cocoaGoString(text))
			}
		}},
		{Cmd: cocoaSel.runQueued, Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			defer cocoaCallbackRecover("vtuiRunQueued:")
			if h := cocoaHostFor(self); h != nil {
				h.runQueued()
			}
		}},
	})
	if err != nil {
		return fmt.Errorf("cocoa: registering the view class: %w", err)
	}

	cocoaRT.classWindowDelegate, err = objc.RegisterClass("VtuiCocoaWindowDelegate", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
		{Cmd: objc.RegisterName("windowShouldClose:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) bool {
			defer cocoaCallbackRecover("windowShouldClose:")
			h := cocoaHostFor(self)
			if h == nil || h.isClosed() || FrameManager.IsShutdown() {
				return true
			}
			// The application decides: it may ask about unsaved work
			// first, and it quits through the same command a key would.
			postQuitCommand()
			return false
		}},
		{Cmd: objc.RegisterName("windowWillClose:"), Fn: func(objc.ID, objc.SEL, objc.ID) {
			defer cocoaCallbackRecover("windowWillClose:")
			cocoaStopApp()
		}},
		{Cmd: objc.RegisterName("windowDidResize:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			defer cocoaCallbackRecover("windowDidResize:")
			if h := cocoaHostFor(self); h != nil {
				h.updateGridSize()
			}
		}},
		{Cmd: objc.RegisterName("windowDidBecomeKey:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			defer cocoaCallbackRecover("windowDidBecomeKey:")
			if h := cocoaHostFor(self); h != nil {
				h.sendEvent(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
			}
		}},
		{Cmd: objc.RegisterName("windowDidResignKey:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			defer cocoaCallbackRecover("windowDidResignKey:")
			if h := cocoaHostFor(self); h != nil {
				// A button released over another window never reaches
				// this one.
				h.mouseBtn = 0
				h.keys.reset()
				h.sendEvent(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
			}
		}},
		{Cmd: objc.RegisterName("windowDidChangeBackingProperties:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			defer cocoaCallbackRecover("windowDidChangeBackingProperties:")
			if h := cocoaHostFor(self); h != nil && h.window != 0 {
				// The font stays rasterised for the display the window
				// opened on; Core Animation scales the frame for this one.
				DebugLog("COCOA: backing scale factor is now %v (frames stay at %v)",
					objc.Send[float64](h.window, cocoaSel.backingScaleFactor), h.pointScale)
				h.view.Send(cocoaSel.setNeedsDisplay, true)
			}
		}},
	})
	if err != nil {
		return fmt.Errorf("cocoa: registering the window delegate class: %w", err)
	}

	// No NSApplicationDelegate protocol is declared: on macOS 14 declaring
	// it has been seen to crash AppKit inside nextEventMatchingMask:
	// (Ebitengine issue #3451), and AppKit finds the methods through
	// respondsToSelector: regardless.
	cocoaRT.classApplicationDelegate, err = objc.RegisterClass("VtuiCocoaApplicationDelegate", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
		{Cmd: objc.RegisterName("applicationShouldTerminate:"), Fn: func(objc.ID, objc.SEL, objc.ID) uint {
			defer cocoaCallbackRecover("applicationShouldTerminate:")
			// Quit from the menu, or a logout, restart or shutdown. Never let
			// AppKit call exit() under the application: ask it to quit as the
			// close button does, or end the loop if it has already gone.
			// AppKit is told to wait rather than refused outright (returning
			// NSTerminateCancel here would otherwise refuse every logout,
			// restart and shutdown for good); cocoaReplyOnceClosed answers it
			// once that quit has actually finished.
			h := cocoaActiveHost.Load()
			if h == nil {
				// Nothing is open: there is nothing left to shut down, so
				// there is nothing to wait for either.
				return nsTerminateNow
			}
			if h.isClosed() || FrameManager.IsShutdown() {
				cocoaStopApp()
			} else {
				postQuitCommand()
			}
			go cocoaReplyOnceClosed(h)
			return nsTerminateLater
		}},
		{Cmd: cocoaSel.vtuiReplyTerminate, Fn: func(objc.ID, objc.SEL, objc.ID) {
			defer cocoaCallbackRecover("vtuiReplyTerminate:")
			app := objc.ID(cocoaRT.classNSApplication).Send(cocoaSel.sharedApplication)
			app.Send(cocoaSel.replyToApplicationShouldTerminate, true)
		}},
		{Cmd: objc.RegisterName("applicationShouldTerminateAfterLastWindowClosed:"), Fn: func(objc.ID, objc.SEL, objc.ID) bool {
			return false
		}},
		{Cmd: objc.RegisterName("applicationDidFinishLaunching:"), Fn: func(objc.ID, objc.SEL, objc.ID) {
			defer cocoaCallbackRecover("applicationDidFinishLaunching:")
			// -run calls finishLaunching itself, the first time it runs, before
			// it starts pumping events; this delegate method is where AppKit
			// considers the app fully launched. Activating any earlier --
			// right after setActivationPolicy: promotes this ordinary
			// executable (no bundle, so no Info.plist to say it upfront) to a
			// regular, Dock-visible app, and before -run had even been sent --
			// asked the Window Server to bring a not-yet-fully-registered app
			// to the front (vtui #1571).
			app := objc.ID(cocoaRT.classNSApplication).Send(cocoaSel.sharedApplication)
			app.Send(cocoaSel.activateIgnoringOtherApps, true)
		}},
	})
	if err != nil {
		return fmt.Errorf("cocoa: registering the application delegate class: %w", err)
	}
	return nil
}

// cocoaCallbackRecover ends the process on a panic in a method AppKit
// called. The panic must not unwind into Objective-C frames, which is
// undefined, so it is recorded the way FrameManager.Run records one and the
// process exits.
func cocoaCallbackRecover(where string) {
	r := recover()
	if r == nil {
		return
	}
	stack := debug.Stack()
	DebugLog("FATAL PANIC in Cocoa callback %s: %v\n%s", where, r, stack)
	crashPath := RecordCrash(r, nil)
	fmt.Fprintf(os.Stderr, "\n[%s] FATAL PANIC in Cocoa callback %s: %v\n%s\n", AppName, where, r, stack)
	if crashPath != "" {
		fmt.Fprintf(os.Stderr, "[%s] Crash report saved to: %s\n", AppName, crashPath)
	}
	os.Exit(2)
}

// cocoaNSString returns a new NSString the caller releases.
func cocoaNSString(s string) objc.ID {
	return objc.ID(cocoaRT.classNSString).Send(cocoaSel.alloc).Send(cocoaSel.initWithUTF8String, s)
}

// cocoaGoString copies an NSString into a Go string.
func cocoaGoString(str objc.ID) string {
	if str == 0 {
		return ""
	}
	n := objc.Send[uint](str, cocoaSel.lengthOfBytes, uint(nsUTF8StringEncoding))
	if n == 0 {
		return ""
	}
	buf := make([]byte, n+1)
	if !objc.Send[bool](str, cocoaSel.getCString, &buf[0], uint(len(buf)), uint(nsUTF8StringEncoding)) {
		return ""
	}
	return string(buf[:n])
}

func cocoaOnMainThread() bool {
	return pthreadMainNP != nil && pthreadMainNP() != 0
}

// cocoaPerformOnMain sends performSelectorOnMainThread:withObject:
// waitUntilDone: from any goroutine. The goroutine stays on its thread for
// the duration, because the autorelease pool around the call belongs to the
// thread.
func cocoaPerformOnMain(target objc.ID, sel objc.SEL, wait bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objcAutoreleasePoolPush()
	defer objcAutoreleasePoolPop(pool)
	target.Send(cocoaSel.performOnMain, sel, objc.ID(0), wait)
}

// cocoaStopApp ends [NSApp run]. stop: only takes effect once the loop has
// dispatched another event, so one is posted behind it.
func cocoaStopApp() {
	app := objc.ID(cocoaRT.classNSApplication).Send(cocoaSel.sharedApplication)
	app.Send(cocoaSel.stop, objc.ID(0))
	event := objc.Send[objc.ID](objc.ID(cocoaRT.classNSEvent), cocoaSel.otherEvent,
		uint(nsEventTypeApplicationDefined), cocoaPoint{}, uint(0), float64(0), 0, objc.ID(0), uintptr(0), 0, 0)
	if event != 0 {
		app.Send(cocoaSel.postEventAtStart, event, true)
	}
}

// cocoaReplyOnceClosed waits for h's own quit sequence -- the one the close
// button and Cmd+Q trigger -- to reach the end of close(), and then answers
// applicationShouldTerminate:'s NSTerminateLater so a logout, restart or
// shutdown that was waiting on it can proceed. AppKit will not act on the
// reply until this arrives, so a wedged close() would otherwise hang the
// logout instead of merely refusing it; the timeout answers anyway rather
// than letting that happen.
func cocoaReplyOnceClosed(h *CocoaGuiHost) {
	select {
	case <-h.hostClosed:
	case <-time.After(10 * time.Second):
		DebugLog("COCOA: close() did not finish within 10s of a termination request; replying anyway")
	}
	if cocoaAppDelegate != 0 {
		cocoaPerformOnMain(cocoaAppDelegate, cocoaSel.vtuiReplyTerminate, false)
	}
}

// cocoaPrimaryScreenTop is the top edge of the main display in AppKit's
// screen coordinates, which grow upwards from its bottom edge.
func cocoaPrimaryScreenTop() float64 {
	screens := objc.ID(cocoaRT.classNSScreen).Send(cocoaSel.screens)
	if screens == 0 {
		return 0
	}
	f := objc.Send[cocoaRect](screens.Send(cocoaSel.objectAtIndex, uint(0)), cocoaSel.frame)
	return f.Origin.Y + f.Size.Height
}

// CocoaGuiHost owns the window. The fields after the mainOnly mark are
// touched on the main thread only and need no lock.
type CocoaGuiHost struct {
	mu       sync.Mutex
	renderer *CocoaGuiRenderer
	reader   *vtinput.Reader
	scr      *ScreenBuf
	cols     int
	rows     int
	closed   bool

	// cellW and cellH are the cell size in pixels. scale is the integer
	// line-thickness factor the raster takes, pointScale the backing scale
	// factor the window opened with: pixels per point. None of them change
	// once the window exists.
	cellW, cellH int
	scale        int
	pointScale   float64

	app, window, view, windowDelegate objc.ID

	// mainMu guards window, view and windowDelegate above, not just
	// mainQueue and loopDone: close() zeroes all three under it once the
	// loop has ended, so runOnMain's gate also keeps every other method from
	// reading them once they are stale (vtui #1571).
	mainMu    sync.Mutex
	mainQueue []func()
	loopDone  bool
	// displayQueued keeps at most one display request in flight.
	displayQueued atomic.Bool
	// hostClosed is closed once close() has finished releasing the window,
	// for applicationShouldTerminate: to wait on before it may tell AppKit a
	// pending logout, restart or shutdown can proceed.
	hostClosed chan struct{}

	// mainOnly
	bitmap         uintptr
	bitmapW        int
	bitmapH        int
	mouseBtn       uint32
	lastMouseCellX int16
	lastMouseCellY int16
	mouseCellKnown bool
	wheelAcc       float64
	keys           cocoaKeyTranslator
}

func (h *CocoaGuiHost) isClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

// runOnMain runs fn on the main thread: at once when called there, else
// queued for the main loop. Work queued after the loop has ended is
// dropped; there is no window left for it to act on. This includes the
// caller already being on the main thread: f4's deferred SaveSession, and
// anything else called from the same goroutine that ran RunInGUIWindow,
// keeps running there once RunInGUIWindow has returned, which is exactly
// when window, view and windowDelegate have already been released (vtui
// #1571) -- so that path needs the same loopDone check as the queued one.
func (h *CocoaGuiHost) runOnMain(fn func()) {
	if cocoaOnMainThread() {
		h.mainMu.Lock()
		done := h.loopDone
		h.mainMu.Unlock()
		if done {
			return
		}
		fn()
		return
	}
	h.mainMu.Lock()
	view := h.view
	if view == 0 || h.loopDone {
		h.mainMu.Unlock()
		return
	}
	h.mainQueue = append(h.mainQueue, fn)
	h.mainMu.Unlock()
	cocoaPerformOnMain(view, cocoaSel.runQueued, false)
}

// runOnMainAndWait is runOnMain for work whose result the caller needs. It
// reports whether fn ran; see runOnMain for why the already-on-the-main-
// thread path has to check loopDone too (vtui #1571).
func (h *CocoaGuiHost) runOnMainAndWait(fn func()) bool {
	if cocoaOnMainThread() {
		h.mainMu.Lock()
		done := h.loopDone
		h.mainMu.Unlock()
		if done {
			return false
		}
		fn()
		return true
	}
	done := make(chan struct{}, 1)
	h.mainMu.Lock()
	view := h.view
	if view == 0 || h.loopDone {
		h.mainMu.Unlock()
		return false
	}
	h.mainQueue = append(h.mainQueue, func() {
		fn()
		done <- struct{}{}
	})
	h.mainMu.Unlock()
	// Not waitUntilDone:YES: should the loop end before it gets to this,
	// the caller would wait for ever.
	cocoaPerformOnMain(view, cocoaSel.runQueued, false)
	select {
	case <-done:
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

// runQueued is vtuiRunQueued:, on the main thread.
func (h *CocoaGuiHost) runQueued() {
	h.mainMu.Lock()
	queue := h.mainQueue
	h.mainQueue = nil
	h.mainMu.Unlock()
	for _, fn := range queue {
		fn()
	}
}

// Invalidate asks for a display pass, from any goroutine. The pass runs
// updateLayer on the main thread, which presents whatever frame is newest
// by then, so requests made while one is pending fold into it.
func (h *CocoaGuiHost) Invalidate() {
	if !h.displayQueued.CompareAndSwap(false, true) {
		return
	}
	h.runOnMain(func() {
		h.displayQueued.Store(false)
		if h.view != 0 {
			h.view.Send(cocoaSel.setNeedsDisplay, true)
		}
	})
}

func (h *CocoaGuiHost) SetTitle(title string) {
	full := WindowTitleWithBackend(title)
	h.runOnMain(func() {
		if h.window == 0 {
			return
		}
		// A directory name off SMB/NFS/FUSE, or an OSC title set from a
		// terminal session, is not guaranteed to be valid UTF-8;
		// initWithUTF8String: returns nil for one that is not, and nil is
		// not a title -setTitle: can take (vtui #1571).
		s := cocoaNSString(strings.ToValidUTF8(full, "\uFFFD"))
		if s == 0 {
			return
		}
		h.window.Send(cocoaSel.setTitle, s)
		s.Send(cocoaSel.release)
	})
}

// ResizeGrid resizes the window to hold cols x rows cells. The new grid
// size reaches FrameManager the way a resize by hand does, through
// windowDidResize:.
//
// setContentSize: alone keeps the bottom-left corner of the window fixed,
// which would grow the window upward on screen; X11 and Win32 keep the top
// edge fixed instead, so the frame is recomputed by hand here to match
// (vtui #1571).
func (h *CocoaGuiHost) ResizeGrid(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	h.runOnMain(func() {
		if h.window == 0 || h.view == 0 {
			return
		}
		oldFrame := objc.Send[cocoaRect](h.window, cocoaSel.frame)
		oldContent := objc.Send[cocoaRect](h.view, cocoaSel.bounds).Size
		// The chrome (title bar, border) around the content view keeps its
		// own size; only the content grows or shrinks.
		chromeW := oldFrame.Size.Width - oldContent.Width
		chromeH := oldFrame.Size.Height - oldContent.Height
		newSize := cocoaSize{
			Width:  float64(cols*h.cellW)/h.pointScale + chromeW,
			Height: float64(rows*h.cellH)/h.pointScale + chromeH,
		}
		newFrame := cocoaRect{
			Origin: cocoaPoint{
				X: oldFrame.Origin.X,
				Y: oldFrame.Origin.Y + oldFrame.Size.Height - newSize.Height,
			},
			Size: newSize,
		}
		h.window.Send(cocoaSel.setFrame, newFrame, true)
	})
}

// ToggleMaximized zooms the window: to the largest size the screen allows,
// or back to the size it had before.
func (h *CocoaGuiHost) ToggleMaximized() bool {
	h.mainMu.Lock()
	done, window := h.loopDone, h.window
	h.mainMu.Unlock()
	if done || window == 0 {
		return false
	}
	h.runOnMain(func() {
		if h.window != 0 {
			h.window.Send(cocoaSel.zoom, objc.ID(0))
		}
	})
	return true
}

// WindowPosition returns the top-left corner of the window frame in points,
// measured from the top-left corner of the main display.
func (h *CocoaGuiHost) WindowPosition() (x, y int, ok bool) {
	type position struct{ x, y int }
	result := make(chan position, 1)
	h.runOnMainAndWait(func() {
		if h.window == 0 {
			return
		}
		f := objc.Send[cocoaRect](h.window, cocoaSel.frame)
		top := cocoaPrimaryScreenTop()
		result <- position{int(math.Round(f.Origin.X)), int(math.Round(top - (f.Origin.Y + f.Size.Height)))}
	})
	select {
	case p := <-result:
		return p.x, p.y, true
	default:
		return 0, 0, false
	}
}

// SetWindowPosition moves the window, in the coordinates WindowPosition
// reports. Called during setup, it runs before the window is first shown.
func (h *CocoaGuiHost) SetWindowPosition(x, y int) {
	h.runOnMain(func() {
		if h.window == 0 {
			return
		}
		h.window.Send(cocoaSel.setFrameTopLeftPoint, cocoaPoint{X: float64(x), Y: cocoaPrimaryScreenTop() - float64(y)})
	})
}

// PostQuit ends the main loop once FrameManager has finished.
func (h *CocoaGuiHost) PostQuit() {
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
	h.runOnMain(cocoaStopApp)
}

// sendEvent hands an event to FrameManager without ever blocking the main
// thread. A pointer move that finds the queue full is dropped, the next one
// carries the position anyway.
func (h *CocoaGuiHost) sendEvent(ev *vtinput.InputEvent) {
	h.mu.Lock()
	var ch chan *vtinput.InputEvent
	if !h.closed && h.reader != nil {
		ch = h.reader.EventChan
	}
	h.mu.Unlock()
	if ch == nil {
		return
	}
	// The reader closes its channel as FrameManager.Run returns, and a
	// callback can still be on its way here at that moment.
	defer func() { _ = recover() }()
	select {
	case ch <- ev:
	default:
		if ev.Type == vtinput.MouseEventType && ev.MouseEventFlags&vtinput.MouseMoved != 0 {
			return
		}
		DebugLog("COCOA: input queue full, dropped %s", ev.String())
	}
}

// updateGridSize recomputes the grid from the size of the view, after a
// resize by hand, a zoom or ResizeGrid.
func (h *CocoaGuiHost) updateGridSize() {
	if h.view == 0 {
		return
	}
	b := objc.Send[cocoaRect](h.view, cocoaSel.bounds)
	cols, rows := cocoaGridForPixels(
		int(math.Round(b.Size.Width*h.pointScale)), int(math.Round(b.Size.Height*h.pointScale)),
		h.cellW, h.cellH)
	h.mu.Lock()
	changed := cols != h.cols || rows != h.rows
	h.cols, h.rows = cols, rows
	h.mu.Unlock()
	if changed {
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType})
	}
	// The canvas follows the view even when the grid does not: the margin
	// right of and below the grid has to be repainted.
	h.view.Send(cocoaSel.setNeedsDisplay, true)
}

// presentFrame is updateLayer. It lays the newest frame onto a bitmap the
// size of the view in pixels, black where the grid does not reach, and
// makes a CGImage of it the layer's contents. The bitmap is kept between
// frames; CGBitmapContextCreateImage copies it (copy-on-write), so the
// image the layer holds is not disturbed by the next frame being written.
func (h *CocoaGuiHost) presentFrame(view objc.ID) {
	layer := view.Send(cocoaSel.layer)
	if layer == 0 {
		return
	}
	b := objc.Send[cocoaRect](view, cocoaSel.bounds)
	w := int(math.Round(b.Size.Width * h.pointScale))
	ht := int(math.Round(b.Size.Height * h.pointScale))
	if w <= 0 || ht <= 0 {
		return
	}
	if h.bitmap == 0 || h.bitmapW != w || h.bitmapH != ht {
		if h.bitmap != 0 {
			cgContextRelease(h.bitmap)
		}
		h.bitmap = cgBitmapContextCreate(nil, uintptr(w), uintptr(ht), 8, 0, cocoaRT.colorSpace, cgImageAlphaNoneSkipLast)
		h.bitmapW, h.bitmapH = w, ht
		if h.bitmap == 0 {
			DebugLog("COCOA: CGBitmapContextCreate failed for %dx%d", w, ht)
			return
		}
	}
	data := cgBitmapContextGetData(h.bitmap)
	stride := int(cgBitmapContextGetBytesPerRow(h.bitmap))
	if data == nil || stride < w*4 {
		return
	}
	pix := unsafe.Slice((*byte)(data), stride*ht)
	if h.renderer != nil {
		h.renderer.composeCanvas(pix, w, ht, stride)
	} else {
		composeCocoaCanvas(pix, w, ht, stride, nil)
	}
	img := cgBitmapContextCreateImage(h.bitmap)
	if img == 0 {
		return
	}
	layer.Send(cocoaSel.setContentsScale, h.pointScale)
	layer.Send(cocoaSel.setContents, objc.ID(img))
	cgImageRelease(img)
}

// eventCell is the cell under a mouse event.
func (h *CocoaGuiHost) eventCell(view, event objc.ID) (int16, int16) {
	p := objc.Send[cocoaPoint](event, cocoaSel.locationInWindow)
	if h.window != 0 && event.Send(cocoaSel.window) != h.window {
		// An event that belongs to no window carries screen coordinates
		// in locationInWindow.
		p = objc.Send[cocoaRect](h.window, cocoaSel.convertRectFromScreen, cocoaRect{Origin: p}).Origin
	}
	p = objc.Send[cocoaPoint](view, cocoaSel.convertPoint, p, objc.ID(0))
	b := objc.Send[cocoaRect](view, cocoaSel.bounds)
	// The view is not flipped: AppKit counts y upwards from the bottom.
	return cocoaCellAt(p.X, b.Size.Height-p.Y, h.pointScale, h.cellW, h.cellH)
}

func cocoaEventModifiers(event objc.ID) vtinput.ControlKeyState {
	return cocoaControlKeyState(objc.Send[uint64](event, cocoaSel.modifierFlags))
}

func (h *CocoaGuiHost) handleMouseButton(view, event objc.ID, btn uint32, down bool) {
	if btn == 0 {
		// otherMouseDown: covers every button past the second; the
		// third is the middle one.
		switch objc.Send[int](event, cocoaSel.buttonNumber) {
		case 2:
			btn = uint32(vtinput.FromLeft2ndButtonPressed)
		case 3:
			btn = uint32(vtinput.FromLeft3rdButtonPressed)
		case 4:
			btn = uint32(vtinput.FromLeft4thButtonPressed)
		default:
			return
		}
	}
	x, y := h.eventCell(view, event)
	if down {
		h.mouseBtn |= btn
	} else {
		h.mouseBtn &^= btn
	}
	ev := &vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          x,
		MouseY:          y,
		KeyDown:         down,
		ButtonState:     h.mouseBtn,
		ControlKeyState: cocoaEventModifiers(event),
	}
	// Every second click of a series is a double click, as Windows counts
	// them.
	if clicks := objc.Send[int](event, cocoaSel.clickCount); down && clicks >= 2 && clicks%2 == 0 {
		ev.MouseEventFlags = vtinput.DoubleClick
	}
	h.lastMouseCellX, h.lastMouseCellY, h.mouseCellKnown = x, y, true
	h.sendEvent(ev)
}

// handleMouseMotion reports the pointer when it enters another cell, with
// or without a button held: text views underline the URL under the pointer
// (f4 #459) and need hover motion for that.
func (h *CocoaGuiHost) handleMouseMotion(view, event objc.ID) {
	x, y := h.eventCell(view, event)
	if h.mouseCellKnown && x == h.lastMouseCellX && y == h.lastMouseCellY {
		return
	}
	h.lastMouseCellX, h.lastMouseCellY, h.mouseCellKnown = x, y, true
	h.sendEvent(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          x,
		MouseY:          y,
		MouseEventFlags: vtinput.MouseMoved,
		ButtonState:     h.mouseBtn,
		ControlKeyState: cocoaEventModifiers(event),
	})
}

func (h *CocoaGuiHost) handleScroll(view, event objc.ID) {
	delta := objc.Send[float64](event, cocoaSel.scrollingDeltaY)
	precise := objc.Send[bool](event, cocoaSel.hasPreciseScrollingDeltas)
	// A trackpad scrolls a notch's worth of lines per that many lines of
	// finger travel.
	notch := float64(h.cellH) / h.pointScale * float64(WheelLinesPerNotch())
	count, dir := cocoaWheelNotches(&h.wheelAcc, delta, precise, notch)
	if count == 0 {
		return
	}
	x, y := h.eventCell(view, event)
	mods := cocoaEventModifiers(event)
	for i := 0; i < count; i++ {
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          x,
			MouseY:          y,
			WheelDirection:  dir,
			ControlKeyState: mods,
		})
	}
}

// claimsKeyEquivalent picks out the chords AppKit offers as key
// equivalents and would otherwise consume before keyDown: -- Ctrl+Tab and
// Ctrl+Esc go to keyboard navigation, Cmd+. to cancelling -- and which an
// application wants as keys.
func (h *CocoaGuiHost) claimsKeyEquivalent(event objc.ID) bool {
	if objc.Send[uint](event, cocoaSel.eventType) != nsEventTypeKeyDown {
		return false
	}
	flags := objc.Send[uint64](event, cocoaSel.modifierFlags)
	vk := cocoaVKForKeyCode(objc.Send[uint16](event, cocoaSel.keyCode))
	if flags&nsEventModifierFlagControl != 0 && (vk == vtinput.VK_TAB || vk == vtinput.VK_ESCAPE) {
		return true
	}
	return flags&nsEventModifierFlagCommand != 0 && vk == vtinput.VK_OEM_PERIOD
}

// handleKeyDown delivers a key press.
//
// Keys that do not type -- navigation, function keys, Escape, Return, Tab,
// Backspace -- and every Ctrl, Cmd or Option chord go out at once as the
// virtual key. An Option chord carries the key's own character, the way the
// gogpu backend fills it in: Option composes on macOS ("†" for Option+T),
// and an accelerator reading that would look for the wrong letter. Ctrl and
// Cmd chords carry none.
//
// Everything else is typing. The text is what the current keyboard layout
// makes of the key (see cocoaKeyTranslator), paired with the key's virtual
// key in one event, as the X11 backend pairs them.
func (h *CocoaGuiHost) handleKeyDown(event objc.ID) {
	keyCode := objc.Send[uint16](event, cocoaSel.keyCode)
	flags := objc.Send[uint64](event, cocoaSel.modifierFlags)
	vk := cocoaVKForKeyCode(keyCode)
	mods := cocoaControlKeyState(flags)
	if isCocoaEnhancedNavKey(vk) {
		mods |= vtinput.EnhancedKey
	}

	if isSpecialOrModifiedKey(vk, mods) {
		// A dead key waiting for its letter is dropped, as Escape or an
		// arrow drops it in a Mac text field.
		h.keys.reset()
		ev := &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			VirtualKeyCode:  vk,
			ControlKeyState: mods,
		}
		if mods&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 {
			ev.Char = cocoaTextRune(cocoaGoString(event.Send(cocoaSel.charactersIgnoringModifiers)))
			if ev.Char == 0 {
				ev.Char = defaultRuneForVK(vk)
			}
		}
		h.sendEvent(ev)
		return
	}

	text, ok := h.keys.translate(keyCode, flags)
	if !ok {
		text = cocoaGoString(event.Send(cocoaSel.characters))
	}
	sent := false
	for _, r := range text {
		if !cocoaIsTypedRune(r) {
			continue
		}
		ev := &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			Char:            r,
			ControlKeyState: mods,
		}
		// The key goes with the first character; a dead key that did not
		// combine types its accent and then the letter.
		if !sent {
			ev.VirtualKeyCode = vk
		}
		h.sendEvent(ev)
		sent = true
	}
	// A key that types nothing is still a key, unless it is a dead key
	// waiting for the next one: Windows reports no key for that either.
	if !sent && vk != 0 && !h.keys.pending() {
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			VirtualKeyCode:  vk,
			ControlKeyState: mods,
		})
	}
}

// insertText types text that came without a key: characters with no
// virtual key, as the Win32 backend's WM_CHAR delivers them.
func (h *CocoaGuiHost) insertText(text string) {
	mods := cocoaControlKeyState(objc.Send[uint64](objc.ID(cocoaRT.classNSEvent), cocoaSel.modifierFlags))
	for _, r := range text {
		if cocoaIsTypedRune(r) {
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				Char:            r,
				ControlKeyState: mods,
			})
		}
	}
}

func (h *CocoaGuiHost) handleKeyUp(event objc.ID) {
	vk := cocoaVKForKeyCode(objc.Send[uint16](event, cocoaSel.keyCode))
	if vk == 0 {
		return
	}
	mods := cocoaEventModifiers(event)
	if isCocoaEnhancedNavKey(vk) {
		mods |= vtinput.EnhancedKey
	}
	h.sendEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         false,
		VirtualKeyCode:  vk,
		ControlKeyState: mods,
	})
}

// handleFlagsChanged turns a modifier key going down or up into its key
// event.
func (h *CocoaGuiHost) handleFlagsChanged(event objc.ID) {
	key, ok := cocoaModifierKeys[objc.Send[uint16](event, cocoaSel.keyCode)]
	if !ok {
		return
	}
	flags := objc.Send[uint64](event, cocoaSel.modifierFlags)
	mods := cocoaControlKeyState(flags)
	if key.vk == vtinput.VK_CAPITAL {
		// Caps Lock reports its lock state, not the key; each change is
		// one press of it.
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key.vk, ControlKeyState: mods})
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: key.vk, ControlKeyState: mods})
		return
	}
	down := cocoaModifierKeyDown(key, flags)
	// FrameManager reads a Ctrl key-up as Ctrl released, whatever the
	// flags say, so a Ctrl channel letting go while the other still holds
	// Ctrl -- Cmd released with Control held -- is not reported; the
	// Switcher would commit its selection on it. The gogpu backend
	// withholds the same key-up.
	if !down && (key.vk == vtinput.VK_LCONTROL || key.vk == vtinput.VK_RCONTROL) &&
		mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0 {
		return
	}
	h.sendEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         down,
		VirtualKeyCode:  key.vk,
		ControlKeyState: mods,
	})
}

// cocoaInstallMenu gives an application without a menu bar the minimal
// one every Mac application has, so Cmd+Q and Cmd+H do what they do
// everywhere. An application that has a menu of its own keeps it.
func cocoaInstallMenu(app objc.ID) {
	if app.Send(cocoaSel.mainMenu) != 0 {
		return
	}
	name := AppName
	if name == "" {
		name = "vtui"
	}
	newString := func(s string) objc.ID { return cocoaNSString(s) }
	menuBar := objc.ID(cocoaRT.classNSMenu).Send(cocoaSel.alloc).Send(cocoaSel.init)
	appItem := objc.ID(cocoaRT.classNSMenuItem).Send(cocoaSel.alloc).Send(cocoaSel.init)
	menuBar.Send(cocoaSel.addItem, appItem)

	title := newString(name)
	appMenu := objc.ID(cocoaRT.classNSMenu).Send(cocoaSel.alloc).Send(cocoaSel.initWithTitle, title)
	title.Send(cocoaSel.release)
	for _, item := range []struct {
		title  string
		action objc.SEL
		key    string
	}{
		{"Hide " + name, cocoaSel.hide, "h"},
		{"", 0, ""},
		{"Quit " + name, cocoaSel.terminate, "q"},
	} {
		if item.action == 0 {
			appMenu.Send(cocoaSel.addItem, objc.ID(cocoaRT.classNSMenuItem).Send(cocoaSel.separatorItem))
			continue
		}
		t, k := newString(item.title), newString(item.key)
		appMenu.Send(cocoaSel.addItemWithTitle, t, item.action, k)
		t.Send(cocoaSel.release)
		k.Send(cocoaSel.release)
	}
	appItem.Send(cocoaSel.setSubmenu, appMenu)
	app.Send(cocoaSel.setMainMenu, menuBar)
	appMenu.Send(cocoaSel.release)
	appItem.Send(cocoaSel.release)
	menuBar.Send(cocoaSel.release)
}

// RunCocoaGuiHost opens a Cocoa window of cols x rows cells, calls setupApp
// to build the interface in it, and runs the AppKit event loop until the
// application quits or the window is closed. It has to be called on the
// main goroutine.
func RunCocoaGuiHost(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := loadCocoa(); err != nil {
		return err
	}
	if !cocoaOnMainThread() {
		return fmt.Errorf("cocoa: RunInGUIWindow has to be called from the main goroutine; AppKit runs on the main thread only")
	}

	pool := objcAutoreleasePoolPush()
	defer objcAutoreleasePoolPop(pool)

	app := objc.ID(cocoaRT.classNSApplication).Send(cocoaSel.sharedApplication)
	// A plain executable has no bundle to say it is an application with
	// windows and a Dock icon; this says it instead.
	app.Send(cocoaSel.setActivationPolicy, nsApplicationActivationPolicyRegular)
	cocoaInstallMenu(app)
	if app.Send(cocoaSel.delegate) == 0 {
		app.Send(cocoaSel.setDelegate, objc.ID(cocoaRT.classApplicationDelegate).Send(cocoaSel.alloc).Send(cocoaSel.init))
	}
	// Kept for cocoaReplyOnceClosed, which runs on its own goroutine and
	// otherwise has no way to reach the delegate without messaging AppKit
	// from off the main thread (vtui #1571).
	cocoaAppDelegate = app.Send(cocoaSel.delegate)

	if fontSize <= 0 {
		fontSize = 18.0
	}
	pointScale := 1.0
	if screen := objc.ID(cocoaRT.classNSScreen).Send(cocoaSel.mainScreen); screen != 0 {
		if s := objc.Send[float64](screen, cocoaSel.backingScaleFactor); s >= 1 {
			pointScale = s
		}
	}
	// The font is rasterised for the display's pixels, as the Win32
	// backend rasterises it for the monitor's DPI; the frame is then shown
	// at pointScale pixels per point.
	face, cellW, cellH := loadBestFont(fontName, fontSize, 72.0*pointScale)
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = int(8*pointScale+0.5), int(16*pointScale+0.5)
	}
	scale := max(int(pointScale+0.5), 1)

	host := &CocoaGuiHost{
		cols:       cols,
		rows:       rows,
		cellW:      cellW,
		cellH:      cellH,
		scale:      scale,
		pointScale: pointScale,
		app:        app,
		hostClosed: make(chan struct{}),
	}

	cellPt := cocoaSize{Width: float64(cellW) / pointScale, Height: float64(cellH) / pointScale}
	content := cocoaRect{Size: cocoaSize{Width: float64(cols) * cellPt.Width, Height: float64(rows) * cellPt.Height}}
	style := uint(nsWindowStyleMaskTitled | nsWindowStyleMaskClosable | nsWindowStyleMaskMiniaturizable | nsWindowStyleMaskResizable)
	window := objc.ID(cocoaRT.classNSWindow).Send(cocoaSel.alloc).Send(cocoaSel.initWithContentRect,
		content, style, uint(nsBackingStoreBuffered), false)
	if window == 0 {
		return fmt.Errorf("cocoa: could not create the window")
	}
	window.Send(cocoaSel.setReleasedWhenClosed, false)
	window.Send(cocoaSel.setRestorable, false)
	if objc.Send[bool](window, cocoaSel.respondsToSelector, cocoaSel.setTabbingMode) {
		window.Send(cocoaSel.setTabbingMode, nsWindowTabbingModeDisallowed)
	}
	window.Send(cocoaSel.setBackgroundColor, objc.ID(cocoaRT.classNSColor).Send(cocoaSel.blackColor))
	// A resize by hand moves in whole cells, as in a terminal window.
	window.Send(cocoaSel.setContentResizeIncrements, cellPt)
	window.Send(cocoaSel.setContentMinSize, cocoaSize{Width: 10 * cellPt.Width, Height: 4 * cellPt.Height})
	// AppName is compiled in and always valid UTF-8, but going through the
	// same ToValidUTF8 guard as SetTitle keeps this call safe too, and keeps
	// the two in one place if that ever changes (vtui #1571).
	title := cocoaNSString(strings.ToValidUTF8(WindowTitleWithBackend(AppName), "\uFFFD"))
	if title != 0 {
		window.Send(cocoaSel.setTitle, title)
		title.Send(cocoaSel.release)
	}

	view := objc.ID(cocoaRT.classView).Send(cocoaSel.alloc).Send(cocoaSel.initWithFrame, content)
	view.Send(cocoaSel.setWantsLayer, true)
	view.Send(cocoaSel.setLayerContentsRedrawPolicy, nsViewLayerContentsRedrawOnSetNeedsDsp)
	view.Send(cocoaSel.setLayerContentsPlacement, nsViewLayerContentsPlacementTopLeft)
	delegate := objc.ID(cocoaRT.classWindowDelegate).Send(cocoaSel.alloc).Send(cocoaSel.init)

	host.window, host.view, host.windowDelegate = window, view, delegate
	cocoaHosts.Store(view, host)
	cocoaHosts.Store(delegate, host)
	cocoaActiveHost.Store(host)
	defer host.close()

	window.Send(cocoaSel.setContentView, view)
	window.Send(cocoaSel.makeFirstResponder, view)
	window.Send(cocoaSel.setDelegate, delegate)
	window.Send(cocoaSel.setAcceptsMouseMovedEvents, true)
	window.Send(cocoaSel.center)

	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	renderer := NewCocoaGuiRenderer(host, face, cellW, cellH)
	scr.Renderer = renderer
	scr.Graphics().SetProtocol(GraphicsNative)
	scr.Graphics().SetCellSize(cellW, cellH)
	host.renderer = renderer
	host.scr = scr

	FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.mu.Lock()
	host.reader = reader
	host.mu.Unlock()

	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return min(host.cols, 500), min(host.rows, 300), nil
	}

	UseWindowClipboard()
	setupApp()
	SetActiveBackend("cocoa",
		fmt.Sprintf("cell %dx%d px, font %q, backing scale %v", cellW, cellH, fontName, pointScale),
		"AppKit window, CoreGraphics bitmap as the layer contents")
	setWheelNotchLines(getSystemScrollLines())

	// Compose the first frame before the window is shown, so it does not
	// open black and fill in a moment later.
	FrameManager.renderPhase()
	scr.Flush()
	view.Send(cocoaSel.setNeedsDisplay, true)

	window.Send(cocoaSel.makeKeyAndOrderFront, objc.ID(0))
	// Not activateIgnoringOtherApps here: -run below sends finishLaunching
	// the first time it runs, and applicationDidFinishLaunching: above does
	// the activation once AppKit is done with it (vtui #1571).

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		defer LogAndRepanic("cocoa FrameManager")
		FrameManager.Run(reader)
		host.PostQuit()
	}()

	app.Send(cocoaSel.run)

	// The loop has ended: FrameManager quit, or the window went away under
	// it. In the second case FrameManager is still running and is told to
	// stop; either way it gets a moment to finish before the window is torn
	// down. Nothing is queued for the main thread from here on, since
	// nothing would run it.
	host.mainMu.Lock()
	host.loopDone = true
	host.mainQueue = nil
	host.mainMu.Unlock()
	host.mu.Lock()
	host.closed = true
	host.mu.Unlock()
	select {
	case <-runDone:
	default:
		FrameManager.Stop()
		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			DebugLog("COCOA: FrameManager did not stop within 2s of the window closing")
		}
	}
	return nil
}

// close tears the window down once the loop has ended.
//
// window, view and windowDelegate are zeroed here, under mainMu, rather than
// merely released: every host method reads them under the same lock (or
// through runOnMain/runOnMainAndWait, which gate on loopDone under it), so a
// call made after RunInGUIWindow has returned -- f4's deferred SaveSession,
// among others -- sees them gone instead of dereferencing the objects this
// releases (vtui #1571).
func (h *CocoaGuiHost) close() {
	h.mainMu.Lock()
	h.loopDone = true
	h.mainQueue = nil
	window, view, windowDelegate := h.window, h.view, h.windowDelegate
	h.window, h.view, h.windowDelegate = 0, 0, 0
	h.mainMu.Unlock()

	cocoaHosts.Delete(view)
	cocoaHosts.Delete(windowDelegate)
	cocoaActiveHost.CompareAndSwap(h, nil)

	if window != 0 {
		window.Send(cocoaSel.setDelegate, objc.ID(0))
		window.Send(cocoaSel.orderOut, objc.ID(0))
		window.Send(cocoaSel.close)
		window.Send(cocoaSel.release)
	}
	if view != 0 {
		view.Send(cocoaSel.release)
	}
	if windowDelegate != 0 {
		windowDelegate.Send(cocoaSel.release)
	}
	if h.bitmap != 0 {
		cgContextRelease(h.bitmap)
		h.bitmap = 0
	}
	close(h.hostClosed)
}

func runInCocoaWindow(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return RunCocoaGuiHost(cols, rows, fontName, fontSize, setupApp)
}
