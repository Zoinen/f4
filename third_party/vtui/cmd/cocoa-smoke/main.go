//go:build darwin && !ios && !vtui_nococoa

// Command cocoa-smoke is the end-to-end check of vtui's Cocoa backend, and
// what the macOS jobs in CI run.
//
// It opens the backend's window with a screen whose every cell it knows,
// and then drives it the way a user would, from the outside: key presses,
// modifier keys, clicks, a drag and a wheel notch go in as NSEvents through
// [NSWindow sendEvent:], the window is resized with setContentSize:, moved,
// retitled and finally closed with performClose:, as the close button does.
// After each step it checks two things: that the event reached the
// application as the vtinput event it should be, and that the frame the
// backend handed to Core Animation -- read back from the view's layer --
// shows what the application drew in answer, down to the colour of known
// cells. It also takes screenshots of the window with screencapture(1) for
// a person to look at; those are reported on but not required, because a
// runner may not grant the screen recording permission they need.
//
// Everything lands in -out: report.txt, the frames as PNG, the screenshots.
// The exit status is zero only when every required check passed.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// NSApplicationTerminateReply, from AppKit -- not exported by vtui, so
// redeclared here to check applicationShouldTerminate:'s return value
// (vtui #1571: it must not always answer NSTerminateCancel, which would
// refuse every logout, restart and shutdown outright).
const (
	nsTerminateCancel = 0
	nsTerminateLater  = 2
)

// The known screen. Cells are given as (column, row).
const (
	colBackground = 0x203050
	colText       = 0xF0F0F0
	colRed        = 0xE02020
	colKey        = 0xE0E020 // painted once the 'x' key press arrived
	colClick      = 0xE020E0 // painted on the cell a click arrived for
	colCorner     = 0x20E020 // always on the bottom-right cell
	paletteIndex  = 2        // an indexed colour, resolved through ThemePalette

	initialCols, initialRows = 60, 20
)

var (
	redCell     = [2]int{3, 1}
	paletteCell = [2]int{12, 1}
	keyCell     = [2]int{22, 1}
	plainCell   = [2]int{40, 10}
	textRow     = 4
	textCol     = 2
	textLabel   = "vtui cocoa smoke"
	caretCell   = [2]int{30, 8}
	clickCell   = [2]int{45, 12}
)

// probe is the application: a desktop that paints the known screen and
// records every event it is given.
type probe struct {
	*vtui.Desktop

	mu      sync.Mutex
	events  []vtinput.InputEvent
	resizes [][2]int

	// UI goroutine only.
	keySeen bool
	click   *[2]int
}

func (p *probe) record(e *vtinput.InputEvent) {
	p.mu.Lock()
	p.events = append(p.events, *e)
	p.mu.Unlock()
}

func (p *probe) Show(scr *vtui.ScreenBuf) {
	w, h := scr.Width(), scr.Height()
	solid := func(rgb uint32) uint64 { return vtui.SetRGBBoth(0, colText, rgb) }
	fill := func(cell [2]int, width, height int, attr uint64) {
		scr.FillRect(cell[0], cell[1], cell[0]+width-1, cell[1]+height-1, ' ', attr)
	}
	scr.FillRect(0, 0, w-1, h-1, ' ', solid(colBackground))
	fill([2]int{redCell[0] - 1, redCell[1]}, 6, 2, solid(colRed))
	fill([2]int{paletteCell[0] - 2, paletteCell[1]}, 6, 2, vtui.SetIndexBoth(0, 15, paletteIndex))
	scr.Write(textCol, textRow, vtui.StringToCharInfo(textLabel, solid(colBackground)))
	if p.keySeen {
		fill([2]int{keyCell[0] - 2, keyCell[1]}, 6, 2, solid(colKey))
	}
	if p.click != nil {
		fill(*p.click, 1, 1, solid(colClick))
	}
	fill([2]int{w - 1, h - 1}, 1, 1, solid(colCorner))
	scr.SetCursorShape(vtui.CursorShapeBlock)
	scr.SetCursorPos(caretCell[0], caretCell[1])
	scr.SetCursorVisible(true)
}

func (p *probe) ProcessKey(e *vtinput.InputEvent) bool {
	p.record(e)
	if e.Type == vtinput.KeyEventType && e.KeyDown && e.Char == 'x' {
		p.keySeen = true
	}
	return true
}

func (p *probe) ProcessMouse(e *vtinput.InputEvent) bool {
	p.record(e)
	if e.KeyDown && e.ButtonState&vtinput.FromLeft1stButtonPressed != 0 && e.MouseEventFlags&vtinput.MouseMoved == 0 {
		p.click = &[2]int{int(e.MouseX), int(e.MouseY)}
	}
	return true
}

func (p *probe) ResizeConsole(w, h int) {
	p.mu.Lock()
	p.resizes = append(p.resizes, [2]int{w, h})
	p.mu.Unlock()
}

// findEvent returns the first recorded event after index from that
// satisfies match, and the index just past it.
func (p *probe) findEvent(from int, match func(e vtinput.InputEvent) bool) (vtinput.InputEvent, int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := from; i < len(p.events); i++ {
		if match(p.events[i]) {
			return p.events[i], i + 1, true
		}
	}
	return vtinput.InputEvent{}, from, false
}

func (p *probe) eventCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

func (p *probe) lastResize() ([2]int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.resizes) == 0 {
		return [2]int{}, false
	}
	return p.resizes[len(p.resizes)-1], true
}

// Objective-C and Core Graphics, for driving the window from outside.
var (
	sel   = map[string]objc.SEL{}
	selMu sync.Mutex

	cgEventCreateKeyboardEvent      func(source uintptr, keyCode uint16, keyDown bool) uintptr
	cgEventSetFlags                 func(event uintptr, flags uint64)
	cgEventSetType                  func(event uintptr, eventType uint32)
	cgEventKeyboardSetUnicodeString func(event uintptr, length uintptr, str *uint16)
	cgEventCreateScrollWheelEvent2  func(source uintptr, units uint32, wheelCount uint32, wheel1, wheel2, wheel3 int32) uintptr
	cgEventSetLocation              func(event uintptr, location point)
	cgImageGetWidth                 func(image uintptr) uintptr
	cgImageGetHeight                func(image uintptr) uintptr
	cgImageGetBytesPerRow           func(image uintptr) uintptr
	cgImageGetBitsPerPixel          func(image uintptr) uintptr
	cgImageGetBitmapInfo            func(image uintptr) uint32
	cgImageGetDataProvider          func(image uintptr) uintptr
	cgDataProviderCopyData          func(provider uintptr) uintptr
	cfDataGetBytePtr                func(data uintptr) unsafe.Pointer
	cfDataGetLength                 func(data uintptr) int
	cfRelease                       func(ref uintptr)

	runnerClass objc.Class
	runner      objc.ID
	mainQueue   struct {
		sync.Mutex
		fns []func()
	}
)

// s returns the selector for name. The driver and the main thread both
// ask for selectors, hence the lock.
func s(name string) objc.SEL {
	selMu.Lock()
	defer selMu.Unlock()
	if v, ok := sel[name]; ok {
		return v
	}
	v := objc.RegisterName(name)
	sel[name] = v
	return v
}

func class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

// initObjC runs on the main thread, before the window exists.
func initObjC() error {
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		return err
	}
	purego.RegisterLibFunc(&cgEventCreateKeyboardEvent, cg, "CGEventCreateKeyboardEvent")
	purego.RegisterLibFunc(&cgEventSetFlags, cg, "CGEventSetFlags")
	purego.RegisterLibFunc(&cgEventSetType, cg, "CGEventSetType")
	purego.RegisterLibFunc(&cgEventKeyboardSetUnicodeString, cg, "CGEventKeyboardSetUnicodeString")
	purego.RegisterLibFunc(&cgEventCreateScrollWheelEvent2, cg, "CGEventCreateScrollWheelEvent2")
	purego.RegisterLibFunc(&cgEventSetLocation, cg, "CGEventSetLocation")
	purego.RegisterLibFunc(&cgImageGetWidth, cg, "CGImageGetWidth")
	purego.RegisterLibFunc(&cgImageGetHeight, cg, "CGImageGetHeight")
	purego.RegisterLibFunc(&cgImageGetBytesPerRow, cg, "CGImageGetBytesPerRow")
	purego.RegisterLibFunc(&cgImageGetBitsPerPixel, cg, "CGImageGetBitsPerPixel")
	purego.RegisterLibFunc(&cgImageGetBitmapInfo, cg, "CGImageGetBitmapInfo")
	purego.RegisterLibFunc(&cgImageGetDataProvider, cg, "CGImageGetDataProvider")
	purego.RegisterLibFunc(&cgDataProviderCopyData, cg, "CGDataProviderCopyData")
	purego.RegisterLibFunc(&cfDataGetBytePtr, cf, "CFDataGetBytePtr")
	purego.RegisterLibFunc(&cfDataGetLength, cf, "CFDataGetLength")
	purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")

	runnerClass, err = objc.RegisterClass("CocoaSmokeRunner", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{{
		Cmd: s("run:"),
		Fn: func(objc.ID, objc.SEL, objc.ID) {
			mainQueue.Lock()
			fns := mainQueue.fns
			mainQueue.fns = nil
			mainQueue.Unlock()
			for _, fn := range fns {
				fn()
			}
		},
	}})
	if err != nil {
		return err
	}
	runner = objc.ID(runnerClass).Send(s("alloc")).Send(s("init"))
	return nil
}

// onMain runs fn on the main thread and waits for it. A panic in fn is
// returned as an error instead of unwinding through AppKit.
func onMain(fn func()) (err error) {
	done := make(chan error, 1)
	mainQueue.Lock()
	mainQueue.fns = append(mainQueue.fns, func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic on the main thread: %v\n%s", r, debug.Stack())
				return
			}
			done <- nil
		}()
		pool := class("NSAutoreleasePool").Send(s("new"))
		defer pool.Send(s("release"))
		fn()
	})
	mainQueue.Unlock()

	runtime.LockOSThread()
	pool := class("NSAutoreleasePool").Send(s("new"))
	runner.Send(s("performSelectorOnMainThread:withObject:waitUntilDone:"), s("run:"), objc.ID(0), true)
	pool.Send(s("release"))
	runtime.UnlockOSThread()

	select {
	case err = <-done:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("the main thread did not run the call within 10s")
	}
}

// onMainAsync runs fn on the main thread without waiting for it. Unlike
// onMain, this never blocks: a termination request already under way (vtui
// #1571's applicationShouldTerminate: path, which checkApplicationTerminate
// exercises directly ahead of closeWindow's defer) can win the race and
// stop the run loop before this dispatch is drained, and waitUntilDone:true
// would then hang for onMain's own 10s timeout -- longer than the 5s the
// "shutdown" check in main allows the driver to return in. If the loop is
// already gone the dispatch is simply never drained, same as a queued
// runOnMain call on the real host once loopDone is set.
func onMainAsync(fn func()) {
	mainQueue.Lock()
	mainQueue.fns = append(mainQueue.fns, func() {
		defer func() { recover() }()
		pool := class("NSAutoreleasePool").Send(s("new"))
		defer pool.Send(s("release"))
		fn()
	})
	mainQueue.Unlock()

	runtime.LockOSThread()
	pool := class("NSAutoreleasePool").Send(s("new"))
	runner.Send(s("performSelectorOnMainThread:withObject:waitUntilDone:"), s("run:"), objc.ID(0), false)
	pool.Send(s("release"))
	runtime.UnlockOSThread()
}

type point struct{ X, Y float64 }
type size struct{ Width, Height float64 }
type rect struct {
	Origin point
	Size   size
}

// smoke holds the state of one run.
type smoke struct {
	out   string
	probe *probe

	mu     sync.Mutex
	report []string
	failed bool

	window, view objc.ID
	windowID     int
	cellW, cellH int
	pointScale   float64
	shots        int
}

func (sm *smoke) logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	fmt.Println(line)
	sm.mu.Lock()
	sm.report = append(sm.report, line)
	sm.mu.Unlock()
}

func (sm *smoke) check(ok bool, name, format string, args ...any) bool {
	detail := fmt.Sprintf(format, args...)
	if ok {
		sm.logf("PASS  %s: %s", name, detail)
	} else {
		sm.logf("FAIL  %s: %s", name, detail)
		sm.mu.Lock()
		sm.failed = true
		sm.mu.Unlock()
	}
	return ok
}

func (sm *smoke) writeReport() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	status := "PASSED"
	if sm.failed {
		status = "FAILED"
	}
	body := strings.Join(sm.report, "\n") + "\n\n" + status + "\n"
	if err := os.WriteFile(filepath.Join(sm.out, "report.txt"), []byte(body), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "writing report:", err)
	}
}

func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// frame reads back the CGImage the backend last made the layer's contents.
func (sm *smoke) frame() (*image.RGBA, error) {
	var img *image.RGBA
	var ferr error
	err := onMain(func() {
		layer := sm.view.Send(s("layer"))
		if layer == 0 {
			ferr = fmt.Errorf("the view has no layer")
			return
		}
		contents := layer.Send(s("contents"))
		if contents == 0 {
			ferr = fmt.Errorf("the layer has no contents yet")
			return
		}
		img, ferr = decodeCGImage(uintptr(contents))
	})
	if err != nil {
		return nil, err
	}
	return img, ferr
}

func decodeCGImage(ref uintptr) (*image.RGBA, error) {
	w, h := int(cgImageGetWidth(ref)), int(cgImageGetHeight(ref))
	stride := int(cgImageGetBytesPerRow(ref))
	if bpp := cgImageGetBitsPerPixel(ref); bpp != 32 {
		return nil, fmt.Errorf("%d bits per pixel, want 32", bpp)
	}
	info := cgImageGetBitmapInfo(ref)
	alpha, order := info&0x1f, info&0x7000
	alphaFirst := alpha == 2 || alpha == 4 || alpha == 6
	little := order == 0x2000
	if order != 0 && order != 0x2000 && order != 0x4000 {
		return nil, fmt.Errorf("bitmap info 0x%x: unexpected byte order", info)
	}
	data := cgDataProviderCopyData(cgImageGetDataProvider(ref))
	if data == 0 {
		return nil, fmt.Errorf("no pixel data")
	}
	defer cfRelease(data)
	n := cfDataGetLength(data)
	if n < stride*h {
		return nil, fmt.Errorf("%d bytes of pixels for %dx%d at stride %d", n, w, h, stride)
	}
	src := unsafe.Slice((*byte)(cfDataGetBytePtr(data)), n)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := src[y*stride+x*4 : y*stride+x*4+4]
			var r, g, b byte
			switch {
			case !little && !alphaFirst:
				r, g, b = p[0], p[1], p[2]
			case !little && alphaFirst:
				r, g, b = p[1], p[2], p[3]
			case little && alphaFirst:
				r, g, b = p[2], p[1], p[0]
			default:
				r, g, b = p[3], p[2], p[1]
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img, nil
}

func (sm *smoke) cellCenter(img *image.RGBA, cell [2]int) (color.RGBA, bool) {
	x, y := cell[0]*sm.cellW+sm.cellW/2, cell[1]*sm.cellH+sm.cellH/2
	if !(image.Point{X: x, Y: y}.In(img.Rect)) {
		return color.RGBA{}, false
	}
	return img.RGBAAt(x, y), true
}

func rgbOf(c color.RGBA) uint32 { return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) }

// waitCellColor waits for the presented frame to show want in the centre
// of cell.
func (sm *smoke) waitCellColor(name string, cell [2]int, want uint32, timeout time.Duration) (*image.RGBA, bool) {
	var last *image.RGBA
	var got uint32
	var lastErr error
	ok := waitFor(timeout, func() bool {
		img, err := sm.frame()
		if err != nil {
			lastErr = err
			return false
		}
		last = img
		c, in := sm.cellCenter(img, cell)
		got = rgbOf(c)
		return in && got == want
	})
	if lastErr != nil && last == nil {
		sm.check(false, name, "no frame to read: %v", lastErr)
		return nil, false
	}
	sm.check(ok, name, "cell %v shows %06X (want %06X)", cell, got, want)
	return last, ok
}

func (sm *smoke) savePNG(name string, img image.Image) {
	if img == nil {
		return
	}
	f, err := os.Create(filepath.Join(sm.out, name))
	if err != nil {
		sm.logf("note  saving %s: %v", name, err)
		return
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		sm.logf("note  encoding %s: %v", name, err)
	}
}

// screenshot captures the window as the window server composites it. It is
// reported, not required: capturing another window's contents needs the
// screen recording permission, which a runner may not have granted.
func (sm *smoke) screenshot(name string) {
	sm.shots++
	path := filepath.Join(sm.out, fmt.Sprintf("screen-%02d-%s.png", sm.shots, name))
	//nolint:gosec // G204: a fixed binary; the arguments are a window number and a file name made here
	out, err := exec.Command("screencapture", "-x", "-o", "-l", strconv.Itoa(sm.windowID), path).CombinedOutput()
	if err != nil {
		sm.logf("note  screencapture %s: %v %s", name, err, strings.TrimSpace(string(out)))
		return
	}
	f, err := os.Open(path)
	if err != nil {
		sm.logf("note  screenshot %s: %v", name, err)
		return
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		sm.logf("note  screenshot %s: %v", name, err)
		return
	}
	// Look for the red block, allowing for colour management.
	found := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 > 0xB0 && g>>8 < 0x60 && bl>>8 < 0x60 {
				found++
			}
		}
	}
	sm.logf("note  screenshot %s: %dx%d, red block %s (%d samples)", filepath.Base(path), b.Dx(), b.Dy(),
		map[bool]string{true: "visible", false: "NOT visible"}[found > 20], found)
}

// Events, built on the main thread. Keyboard events come from Quartz event
// services rather than from +[NSEvent keyEventWithType:...]: that method
// takes a BOOL and an unsigned short on the stack on arm64, where the FFI
// layer does not pack them the way Apple's ABI does.
const (
	nsEventTypeLeftMouseDown    = 1
	nsEventTypeLeftMouseUp      = 2
	nsEventTypeRightMouseDown   = 3
	nsEventTypeRightMouseUp     = 4
	nsEventTypeLeftMouseDragged = 6

	kCGEventFlagsChanged = 12

	flagShift   = 1 << 17
	flagControl = 1 << 18
	flagOption  = 1 << 19
	flagCommand = 1 << 20
	flagNumPad  = 1 << 21
	flagFn      = 1 << 23
)

func keyEvent(keyCode uint16, down bool, flags uint64, text string) objc.ID {
	ev := cgEventCreateKeyboardEvent(0, keyCode, down)
	cgEventSetFlags(ev, flags)
	if text != "" {
		u := utf16.Encode([]rune(text))
		cgEventKeyboardSetUnicodeString(ev, uintptr(len(u)), &u[0])
	}
	ns := class("NSEvent").Send(s("eventWithCGEvent:"), ev)
	cfRelease(ev)
	return ns
}

func flagsEvent(keyCode uint16, flags uint64) objc.ID {
	ev := cgEventCreateKeyboardEvent(0, keyCode, true)
	cgEventSetType(ev, kCGEventFlagsChanged)
	cgEventSetFlags(ev, flags)
	ns := class("NSEvent").Send(s("eventWithCGEvent:"), ev)
	cfRelease(ev)
	return ns
}

func (sm *smoke) mouseEvent(kind uint, cell [2]int, clicks int) objc.ID {
	bounds := objc.Send[rect](sm.view, s("bounds"))
	loc := point{
		X: (float64(cell[0]*sm.cellW) + float64(sm.cellW)/2) / sm.pointScale,
		Y: bounds.Size.Height - (float64(cell[1]*sm.cellH)+float64(sm.cellH)/2)/sm.pointScale,
	}
	number := objc.Send[int](sm.window, s("windowNumber"))
	return objc.Send[objc.ID](class("NSEvent"), s("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
		kind, loc, uint(0), float64(0), number, objc.ID(0), 0, clicks, float32(1))
}

// scroll hands the view a wheel event over cell, lines notches up (or
// down, negative). Quartz makes scroll events, and one made here belongs
// to no window: it is given the cell's position on screen and delivered to
// the view directly, as the window would deliver it to the view under the
// pointer.
func (sm *smoke) scroll(cell [2]int, lines int32) {
	bounds := objc.Send[rect](sm.view, s("bounds"))
	local := point{
		X: (float64(cell[0]*sm.cellW) + float64(sm.cellW)/2) / sm.pointScale,
		Y: bounds.Size.Height - (float64(cell[1]*sm.cellH)+float64(sm.cellH)/2)/sm.pointScale,
	}
	inWindow := objc.Send[point](sm.view, s("convertPoint:toView:"), local, objc.ID(0))
	onScreen := objc.Send[rect](sm.window, s("convertRectToScreen:"), rect{Origin: inWindow}).Origin
	screens := class("NSScreen").Send(s("screens"))
	primary := objc.Send[rect](screens.Send(s("objectAtIndex:"), uint(0)), s("frame"))
	// Quartz measures from the top-left corner of the main display, AppKit
	// from its bottom-left corner.
	global := point{X: onScreen.X, Y: primary.Origin.Y + primary.Size.Height - onScreen.Y}

	ev := cgEventCreateScrollWheelEvent2(0, 1 /* kCGScrollEventUnitLine */, 1, lines, 0, 0)
	cgEventSetLocation(ev, global)
	ns := class("NSEvent").Send(s("eventWithCGEvent:"), ev)
	cfRelease(ev)
	sm.view.Send(s("scrollWheel:"), ns)
}

// send builds events and hands them to the window in one main-thread
// call. The events are autoreleased, and the pool around each call drains
// at its end: an event built in one call and sent in the next is gone.
func (sm *smoke) send(build func() []objc.ID) error {
	return onMain(func() {
		for _, ev := range build() {
			sm.window.Send(s("sendEvent:"), ev)
		}
	})
}

// run is the driver, on its own goroutine while the main thread runs the
// AppKit loop.
func (sm *smoke) run() {
	defer func() {
		if r := recover(); r != nil {
			sm.check(false, "driver", "panic: %v\n%s", r, debug.Stack())
		}
		sm.closeWindow()
	}()

	// 1. The window comes up and shows its first frame.
	viewClass := objc.GetClass("VtuiCocoaView")
	found := waitFor(20*time.Second, func() bool {
		_ = onMain(func() {
			windows := class("NSApplication").Send(s("sharedApplication")).Send(s("windows"))
			for i := 0; i < objc.Send[int](windows, s("count")); i++ {
				w := windows.Send(s("objectAtIndex:"), uint(i))
				v := w.Send(s("contentView"))
				if v != 0 && objc.Send[bool](v, s("isKindOfClass:"), viewClass) {
					sm.window, sm.view = w, v
					sm.windowID = objc.Send[int](w, s("windowNumber"))
					sm.pointScale = objc.Send[float64](w, s("backingScaleFactor"))
					return
				}
			}
		})
		return sm.window != 0
	})
	if !sm.check(found, "window", "a window with a VtuiCocoaView content view exists") {
		return
	}
	if sm.pointScale <= 0 {
		sm.pointScale = 1
	}
	sm.cellW, sm.cellH = vtui.FrameManager.Screen().Graphics().CellSize()
	sm.logf("info  backend %q, %s; window %d, backing scale %v, cell %dx%d px",
		vtui.ActiveBackend(), strings.Join(vtui.BackendDetails(), "; "), sm.windowID, sm.pointScale, sm.cellW, sm.cellH)
	sm.check(vtui.ActiveBackend() == "cocoa", "backend", "active backend is %q", vtui.ActiveBackend())
	if !sm.check(sm.cellW > 0 && sm.cellH > 0, "cell size", "%dx%d", sm.cellW, sm.cellH) {
		return
	}
	sm.checkAppActive()

	img, ok := sm.waitCellColor("first frame", redCell, colRed, 15*time.Second)
	if !ok {
		sm.savePNG("frame-01-initial.png", img)
		return
	}
	sm.checkInitialFrame(img)
	sm.savePNG("frame-01-initial.png", img)
	sm.screenshot("initial")
	sm.checkCaret()

	// 2. Title and position, through vtui's public API, from the UI
	// goroutine as an application would call it.
	vtui.FrameManager.PostTask(func() { vtui.SetWindowTitle("cocoa smoke") })
	var title string
	sm.check(waitFor(5*time.Second, func() bool {
		_ = onMain(func() {
			title = cocoaString(sm.window.Send(s("title")))
		})
		return title == "cocoa smoke [cocoa]"
	}), "title", "window title is %q", title)

	vtui.FrameManager.PostTask(func() { vtui.SetWindowPosition(120, 140) })
	var x, y int
	var posOK bool
	sm.check(waitFor(5*time.Second, func() bool {
		x, y, posOK = vtui.GetWindowPosition()
		return posOK && x == 120 && y == 140
	}), "position", "window position reads back as (%d,%d) ok=%v after moving it to (120,140)", x, y, posOK)

	// 3. Keys.
	sm.checkKeys()
	img, _ = sm.waitCellColor("key reached the app and was drawn", keyCell, colKey, 5*time.Second)
	sm.savePNG("frame-02-after-keys.png", img)

	// 4. Mouse.
	sm.checkMouse()
	img, _ = sm.waitCellColor("click reached the app and was drawn", clickCell, colClick, 5*time.Second)
	sm.savePNG("frame-03-after-click.png", img)
	sm.screenshot("after-input")

	// 5. Resize by hand: the window edge is dragged, and updateGridSize
	// recomputes the grid from it.
	sm.checkResize()

	// 6. Resize the other way: the application asks for a grid size and
	// ResizeGrid picks the window size for it (vtui #1571's ResizeGrid fix
	// -- the window used to grow upward instead of downward).
	sm.checkResizeGrid()

	// 7. A title that is not valid UTF-8 must not be dropped or crash the
	// window (vtui #1571's SetTitle fix).
	sm.checkInvalidUTF8Title()

	// 8. A logout, restart or shutdown must not be refused outright (vtui
	// #1571's applicationShouldTerminate: fix). This starts the real quit,
	// so it runs last; closeWindow below becomes a harmless no-op once the
	// window is already on its way down.
	sm.checkApplicationTerminate()
}

func cocoaString(str objc.ID) string {
	if str == 0 {
		return ""
	}
	n := objc.Send[uint](str, s("lengthOfBytesUsingEncoding:"), uint(4))
	buf := make([]byte, n+1)
	if !objc.Send[bool](str, s("getCString:maxLength:encoding:"), &buf[0], uint(len(buf)), uint(4)) {
		return ""
	}
	return string(buf[:n])
}

func (sm *smoke) checkInitialFrame(img *image.RGBA) {
	cols, rows := vtui.FrameManager.Screen().Width(), vtui.FrameManager.Screen().Height()
	sm.check(img.Rect.Dx() >= cols*sm.cellW && img.Rect.Dy() >= rows*sm.cellH, "frame size",
		"%dx%d px for a %dx%d grid of %dx%d cells", img.Rect.Dx(), img.Rect.Dy(), cols, rows, sm.cellW, sm.cellH)
	sm.check(cols == initialCols && rows == initialRows, "grid size", "%dx%d cells, asked for %dx%d", cols, rows, initialCols, initialRows)

	c, _ := sm.cellCenter(img, plainCell)
	sm.check(rgbOf(c) == colBackground, "background", "cell %v shows %06X (want %06X)", plainCell, rgbOf(c), colBackground)
	c, _ = sm.cellCenter(img, paletteCell)
	want := vtui.ThemePalette[paletteIndex] & 0xFFFFFF
	sm.check(rgbOf(c) == want, "palette", "cell %v shows %06X, ThemePalette[%d] is %06X", paletteCell, rgbOf(c), paletteIndex, want)
	c, _ = sm.cellCenter(img, [2]int{cols - 1, rows - 1})
	sm.check(rgbOf(c) == colCorner, "bottom-right cell", "cell (%d,%d) shows %06X (want %06X)", cols-1, rows-1, rgbOf(c), colCorner)

	// Glyphs: the label's cells carry pixels of the text colour.
	ink := 0
	for x := textCol * sm.cellW; x < (textCol+len(textLabel))*sm.cellW; x++ {
		for y := textRow * sm.cellH; y < (textRow+1)*sm.cellH; y++ {
			if rgbOf(img.RGBAAt(x, y)) != colBackground {
				ink++
			}
		}
	}
	sm.check(ink > len(textLabel)*2, "text", "%d pixels of glyph ink in %q", ink, textLabel)

	// Everything right of and below the grid is black.
	if img.Rect.Dx() > cols*sm.cellW {
		c := img.RGBAAt(img.Rect.Dx()-1, 0)
		sm.check(rgbOf(c) == 0, "right margin", "pixel (%d,0) is %06X (want black)", img.Rect.Dx()-1, rgbOf(c))
	}
}

// checkCaret watches the caret's cell for a while: the block caret inverts
// the cell, and blinks.
func (sm *smoke) checkCaret() {
	inverted, plain := 0, 0
	for i := 0; i < 16; i++ {
		img, err := sm.frame()
		if err == nil {
			c, _ := sm.cellCenter(img, caretCell)
			switch rgbOf(c) {
			case colBackground ^ 0xFFFFFF:
				inverted++
			case colBackground:
				plain++
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	sm.check(inverted > 0, "caret", "cell %v inverted in %d of 16 frames, plain in %d (it blinks)", caretCell, inverted, plain)
}

func (sm *smoke) checkKeys() {
	type keyCase struct {
		name    string
		events  func() []objc.ID
		match   func(e vtinput.InputEvent) bool
		explain string
	}
	isKey := func(e vtinput.InputEvent, down bool, vk uint16) bool {
		return e.Type == vtinput.KeyEventType && e.KeyDown == down && e.VirtualKeyCode == vk
	}
	cases := []keyCase{
		{
			name:    "typed x",
			events:  func() []objc.ID { return []objc.ID{keyEvent(0x07, true, 0, "x"), keyEvent(0x07, false, 0, "x")} },
			match:   func(e vtinput.InputEvent) bool { return isKey(e, true, vtinput.VK_X) && e.Char == 'x' },
			explain: "KeyDown VK_X with Char 'x'",
		},
		{
			name:    "key up",
			events:  func() []objc.ID { return nil },
			match:   func(e vtinput.InputEvent) bool { return isKey(e, false, vtinput.VK_X) },
			explain: "KeyUp VK_X",
		},
		{
			name:   "shifted A",
			events: func() []objc.ID { return []objc.ID{keyEvent(0x00, true, flagShift|0x2, "A")} },
			match: func(e vtinput.InputEvent) bool {
				return isKey(e, true, vtinput.VK_A) && e.Char == 'A' && e.ControlKeyState&vtinput.ShiftPressed != 0
			},
			explain: "KeyDown VK_A, Char 'A', Shift",
		},
		{
			name:    "F5",
			events:  func() []objc.ID { return []objc.ID{keyEvent(0x60, true, flagFn, "")} },
			match:   func(e vtinput.InputEvent) bool { return isKey(e, true, vtinput.VK_F5) && e.Char == 0 },
			explain: "KeyDown VK_F5, no Char",
		},
		{
			name:    "left arrow",
			events:  func() []objc.ID { return []objc.ID{keyEvent(0x7B, true, flagFn|flagNumPad, "")} },
			match:   func(e vtinput.InputEvent) bool { return isKey(e, true, vtinput.VK_LEFT) },
			explain: "KeyDown VK_LEFT",
		},
		{
			name:   "Cmd+C",
			events: func() []objc.ID { return []objc.ID{keyEvent(0x08, true, flagCommand|0x8, "c")} },
			match: func(e vtinput.InputEvent) bool {
				return isKey(e, true, vtinput.VK_C) && e.ControlKeyState&vtinput.LeftCtrlPressed != 0 && e.Char == 0
			},
			explain: "KeyDown VK_C with LeftCtrlPressed (Command), no Char",
		},
		{
			name:   "Control+C",
			events: func() []objc.ID { return []objc.ID{keyEvent(0x08, true, flagControl|0x1, "\x03")} },
			match: func(e vtinput.InputEvent) bool {
				return isKey(e, true, vtinput.VK_C) && e.ControlKeyState&vtinput.RightCtrlPressed != 0
			},
			explain: "KeyDown VK_C with RightCtrlPressed (Control)",
		},
		{
			name:   "Option+T",
			events: func() []objc.ID { return []objc.ID{keyEvent(0x11, true, flagOption|0x20, "")} },
			match: func(e vtinput.InputEvent) bool {
				return isKey(e, true, vtinput.VK_T) && e.ControlKeyState&vtinput.LeftAltPressed != 0 && e.Char == 't'
			},
			explain: "KeyDown VK_T with LeftAltPressed and Char 't'",
		},
		{
			name:    "Shift down",
			events:  func() []objc.ID { return []objc.ID{flagsEvent(0x38, flagShift|0x2)} },
			match:   func(e vtinput.InputEvent) bool { return isKey(e, true, vtinput.VK_LSHIFT) },
			explain: "KeyDown VK_LSHIFT from flagsChanged:",
		},
		{
			name:    "Shift up",
			events:  func() []objc.ID { return []objc.ID{flagsEvent(0x38, 0)} },
			match:   func(e vtinput.InputEvent) bool { return isKey(e, false, vtinput.VK_LSHIFT) },
			explain: "KeyUp VK_LSHIFT from flagsChanged:",
		},
		{
			name:    "Command down",
			events:  func() []objc.ID { return []objc.ID{flagsEvent(0x37, flagCommand|0x8)} },
			match:   func(e vtinput.InputEvent) bool { return isKey(e, true, vtinput.VK_LCONTROL) },
			explain: "KeyDown VK_LCONTROL from the Command key",
		},
		{
			name:    "Command up",
			events:  func() []objc.ID { return []objc.ID{flagsEvent(0x37, 0)} },
			match:   func(e vtinput.InputEvent) bool { return isKey(e, false, vtinput.VK_LCONTROL) },
			explain: "KeyUp VK_LCONTROL from the Command key",
		},
	}
	next := sm.probe.eventCount()
	for _, kc := range cases {
		if err := sm.send(kc.events); err != nil {
			sm.check(false, kc.name, "sending: %v", err)
			continue
		}
		var got vtinput.InputEvent
		var at int
		ok := waitFor(3*time.Second, func() bool {
			var found bool
			got, at, found = sm.probe.findEvent(next, kc.match)
			return found
		})
		if ok {
			next = at
			sm.check(true, kc.name, "%s (vk 0x%02X char %q mods %v)", kc.explain, got.VirtualKeyCode, got.Char, got.ControlKeyState)
		} else {
			sm.check(false, kc.name, "no %s among: %s", kc.explain, sm.recentEvents(next))
		}
	}
}

func (sm *smoke) recentEvents(from int) string {
	sm.probe.mu.Lock()
	defer sm.probe.mu.Unlock()
	var parts []string
	for i := from; i < len(sm.probe.events); i++ {
		e := sm.probe.events[i]
		parts = append(parts, fmt.Sprintf("{type %d down %v vk 0x%02X char %q mods %v mouse (%d,%d) buttons %x flags %x wheel %d}",
			e.Type, e.KeyDown, e.VirtualKeyCode, e.Char, e.ControlKeyState, e.MouseX, e.MouseY, e.ButtonState, e.MouseEventFlags, e.WheelDirection))
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
}

func (sm *smoke) checkMouse() {
	isMouse := func(e vtinput.InputEvent) bool { return e.Type == vtinput.MouseEventType }
	at := func(e vtinput.InputEvent, cell [2]int) bool {
		return int(e.MouseX) == cell[0] && int(e.MouseY) == cell[1]
	}
	left := uint32(vtinput.FromLeft1stButtonPressed)
	right := uint32(vtinput.RightmostButtonPressed)
	dragFrom, dragTo := [2]int{10, 14}, [2]int{14, 14}
	rightCell := [2]int{5, 15}
	wheelCell := [2]int{33, 6}

	steps := []struct {
		name    string
		events  func() []objc.ID
		match   func(e vtinput.InputEvent) bool
		explain string
	}{
		{
			name: "left press",
			events: func() []objc.ID {
				return []objc.ID{sm.mouseEvent(nsEventTypeLeftMouseDown, clickCell, 1)}
			},
			match: func(e vtinput.InputEvent) bool {
				return isMouse(e) && e.KeyDown && e.ButtonState&left != 0 && at(e, clickCell)
			},
			explain: fmt.Sprintf("a left button press at cell %v", clickCell),
		},
		{
			name:   "left release",
			events: func() []objc.ID { return []objc.ID{sm.mouseEvent(nsEventTypeLeftMouseUp, clickCell, 1)} },
			match: func(e vtinput.InputEvent) bool {
				return isMouse(e) && !e.KeyDown && e.ButtonState == 0 && at(e, clickCell)
			},
			explain: fmt.Sprintf("the release at cell %v", clickCell),
		},
		{
			name: "double click",
			events: func() []objc.ID {
				return []objc.ID{sm.mouseEvent(nsEventTypeLeftMouseDown, clickCell, 2), sm.mouseEvent(nsEventTypeLeftMouseUp, clickCell, 2)}
			},
			match: func(e vtinput.InputEvent) bool {
				return isMouse(e) && e.KeyDown && e.MouseEventFlags&vtinput.DoubleClick != 0
			},
			explain: "a press flagged DoubleClick",
		},
		{
			name: "right click",
			events: func() []objc.ID {
				return []objc.ID{sm.mouseEvent(nsEventTypeRightMouseDown, rightCell, 1), sm.mouseEvent(nsEventTypeRightMouseUp, rightCell, 1)}
			},
			match: func(e vtinput.InputEvent) bool {
				return isMouse(e) && e.KeyDown && e.ButtonState&right != 0 && at(e, rightCell)
			},
			explain: fmt.Sprintf("a right button press at cell %v", rightCell),
		},
		{
			name: "drag",
			events: func() []objc.ID {
				return []objc.ID{
					sm.mouseEvent(nsEventTypeLeftMouseDown, dragFrom, 1),
					sm.mouseEvent(nsEventTypeLeftMouseDragged, dragTo, 1),
					sm.mouseEvent(nsEventTypeLeftMouseUp, dragTo, 1),
				}
			},
			match: func(e vtinput.InputEvent) bool {
				return isMouse(e) && e.MouseEventFlags&vtinput.MouseMoved != 0 && e.ButtonState&left != 0 && at(e, dragTo)
			},
			explain: fmt.Sprintf("a move with the left button held, at cell %v", dragTo),
		},
		{
			name: "click after drag",
			events: func() []objc.ID {
				return []objc.ID{sm.mouseEvent(nsEventTypeLeftMouseDown, clickCell, 1), sm.mouseEvent(nsEventTypeLeftMouseUp, clickCell, 1)}
			},
			match: func(e vtinput.InputEvent) bool {
				return isMouse(e) && e.KeyDown && e.ButtonState&left != 0 && at(e, clickCell)
			},
			explain: fmt.Sprintf("a left button press at cell %v", clickCell),
		},
		{
			name:    "wheel up",
			events:  func() []objc.ID { sm.scroll(wheelCell, 3); return nil },
			match:   func(e vtinput.InputEvent) bool { return isMouse(e) && e.WheelDirection == 1 && at(e, wheelCell) },
			explain: fmt.Sprintf("a wheel event scrolling up (direction 1) at cell %v", wheelCell),
		},
		{
			name:    "wheel down",
			events:  func() []objc.ID { sm.scroll(wheelCell, -3); return nil },
			match:   func(e vtinput.InputEvent) bool { return isMouse(e) && e.WheelDirection == -1 && at(e, wheelCell) },
			explain: fmt.Sprintf("a wheel event scrolling down (direction -1) at cell %v", wheelCell),
		},
	}
	next := sm.probe.eventCount()
	for _, st := range steps {
		if err := sm.send(st.events); err != nil {
			sm.check(false, st.name, "sending: %v", err)
			continue
		}
		var got vtinput.InputEvent
		var idx int
		ok := waitFor(3*time.Second, func() bool {
			var found bool
			got, idx, found = sm.probe.findEvent(next, st.match)
			return found
		})
		if ok {
			next = idx
			sm.check(true, st.name, "%s (cell (%d,%d) buttons %x flags %x wheel %d)", st.explain, got.MouseX, got.MouseY, got.ButtonState, got.MouseEventFlags, got.WheelDirection)
		} else {
			sm.check(false, st.name, "no %s among: %s", st.explain, sm.recentEvents(next))
		}
	}
}

func (sm *smoke) checkResize() {
	scr := vtui.FrameManager.Screen()
	cols, rows := scr.Width()+10, scr.Height()+4
	// Half a cell more than the grid needs, so there is a margin to check.
	w := (float64(cols*sm.cellW) + float64(sm.cellW)/2) / sm.pointScale
	h := (float64(rows*sm.cellH) + float64(sm.cellH)/2) / sm.pointScale
	if err := onMain(func() {
		sm.window.Send(s("setContentSize:"), size{Width: w, Height: h})
	}); err != nil {
		sm.check(false, "resize", "setContentSize: %v", err)
		return
	}
	var last [2]int
	sm.check(waitFor(5*time.Second, func() bool {
		var ok bool
		last, ok = sm.probe.lastResize()
		return ok && last == [2]int{cols, rows}
	}), "resize reached the app", "ResizeConsole(%d, %d), want (%d, %d)", last[0], last[1], cols, rows)

	img, ok := sm.waitCellColor("resized frame", [2]int{cols - 1, rows - 1}, colCorner, 5*time.Second)
	if ok {
		sm.check(img.Rect.Dx() > cols*sm.cellW && img.Rect.Dy() > rows*sm.cellH, "resized frame size",
			"%dx%d px for %dx%d cells of %dx%d", img.Rect.Dx(), img.Rect.Dy(), cols, rows, sm.cellW, sm.cellH)
		c := img.RGBAAt(img.Rect.Dx()-1, img.Rect.Dy()-1)
		sm.check(rgbOf(c) == 0, "margin after resize", "bottom-right pixel is %06X (want black)", rgbOf(c))
		c, _ = sm.cellCenter(img, redCell)
		sm.check(rgbOf(c) == colRed, "content after resize", "cell %v shows %06X (want %06X)", redCell, rgbOf(c), colRed)
	}
	sm.savePNG("frame-04-resized.png", img)
	sm.screenshot("resized")
}

// checkResizeGrid drives host.ResizeGrid the way FrameManager.ResizeWindow
// does when the application itself asks for a different grid size --
// unlike checkResize, which simulates the user dragging the window's edge
// by hand -- and checks that the window's top edge stays put. The old bug
// kept the bottom-left corner fixed instead, so the window grew upward on
// screen rather than downward, as X11 and Win32 do (vtui #1571).
func (sm *smoke) checkResizeGrid() {
	scr := vtui.FrameManager.Screen()
	cols, rows := scr.Width()+6, scr.Height()+3
	var before, after rect
	if err := onMain(func() { before = objc.Send[rect](sm.window, s("frame")) }); err != nil {
		sm.check(false, "resize-grid frame before", "%v", err)
		return
	}

	vtui.FrameManager.ResizeWindow(cols, rows)

	var last [2]int
	sm.check(waitFor(5*time.Second, func() bool {
		var ok bool
		last, ok = sm.probe.lastResize()
		return ok && last == [2]int{cols, rows}
	}), "resize-grid reached the app", "ResizeConsole(%d, %d), want (%d, %d)", last[0], last[1], cols, rows)

	if err := onMain(func() { after = objc.Send[rect](sm.window, s("frame")) }); err != nil {
		sm.check(false, "resize-grid frame after", "%v", err)
		return
	}
	sm.check(after.Size.Width > before.Size.Width && after.Size.Height > before.Size.Height, "resize-grid grew",
		"window frame %+v grew to %+v for a %dx%d grid", before, after, cols, rows)

	beforeTop := before.Origin.Y + before.Size.Height
	afterTop := after.Origin.Y + after.Size.Height
	sm.check(math.Abs(afterTop-beforeTop) < 1, "resize-grid top edge",
		"top edge stayed at %.1f (was %.1f): ResizeGrid grows the window downward, not upward", afterTop, beforeTop)

	img, ok := sm.waitCellColor("resize-grid frame", [2]int{cols - 1, rows - 1}, colCorner, 5*time.Second)
	if ok {
		c, _ := sm.cellCenter(img, redCell)
		sm.check(rgbOf(c) == colRed, "resize-grid content", "cell %v shows %06X (want %06X)", redCell, rgbOf(c), colRed)
	}
	sm.savePNG("frame-05-resize-grid.png", img)
}

// checkAppActive waits for NSApp to report itself active, the way it must
// once the window is up. activateIgnoringOtherApps: used to run before
// -run's own finishLaunching (vtui #1571): moved into
// applicationDidFinishLaunching: instead, it now runs at the point AppKit
// itself considers the app launched, rather than a moment before the Window
// Server had necessarily caught up with setActivationPolicy: promoting this
// bundle-less executable to a regular, Dock-visible app.
func (sm *smoke) checkAppActive() {
	var active bool
	ok := waitFor(5*time.Second, func() bool {
		_ = onMain(func() { active = objc.Send[bool](class("NSApplication").Send(s("sharedApplication")), s("isActive")) })
		return active
	})
	sm.check(ok && active, "app active", "NSApp.isActive is %v once the window is up (must become active through applicationDidFinishLaunching:)", active)
}

// checkInvalidUTF8Title sets a title that is not valid UTF-8, the way an
// OSC title from a terminal session or a non-UTF-8 directory name off
// SMB/NFS/FUSE can be, and checks that the window still gets a readable
// title instead of AppKit's nonnull setTitle: receiving nil (vtui #1571).
func (sm *smoke) checkInvalidUTF8Title() {
	vtui.FrameManager.PostTask(func() { vtui.SetWindowTitle("bad\xff\xfeutf8") })
	var title string
	ok := waitFor(5*time.Second, func() bool {
		_ = onMain(func() { title = cocoaString(sm.window.Send(s("title"))) })
		return strings.Contains(title, "utf8")
	})
	sm.check(ok && utf8.ValidString(title) && strings.Contains(title, "bad") && strings.Contains(title, "utf8"),
		"invalid UTF-8 title", "window title after an invalid-UTF-8 SetWindowTitle is %q (valid UTF-8, not dropped)", title)
}

// checkApplicationTerminate sends applicationShouldTerminate: to NSApp's
// delegate directly, the way -terminate: (Cmd+Q, or a logout, restart or
// shutdown) does, and checks two things: that it answers NSTerminateLater
// rather than refusing outright (vtui #1571 -- an outright refusal would
// block every logout, restart and shutdown for good), and that the
// application still quits normally afterward, i.e. that the later reply
// this triggers is not left hanging. It also exercises the app quitting by
// a path other than the close button, ahead of the after-shutdown checks in
// main.
func (sm *smoke) checkApplicationTerminate() {
	var delegate objc.ID
	var reply uint
	if err := onMain(func() {
		delegate = class("NSApplication").Send(s("sharedApplication")).Send(s("delegate"))
		if delegate != 0 {
			reply = objc.Send[uint](delegate, s("applicationShouldTerminate:"), objc.ID(0))
		}
	}); err != nil {
		sm.check(false, "terminate reply", "applicationShouldTerminate: %v", err)
		return
	}
	if !sm.check(delegate != 0, "terminate delegate", "NSApp has an application delegate") {
		return
	}
	sm.check(reply == nsTerminateLater && reply != nsTerminateCancel, "terminate reply",
		"applicationShouldTerminate: returned %d, want NSTerminateLater (%d), not an immediate NSTerminateCancel (%d)",
		reply, nsTerminateLater, nsTerminateCancel)
}

// closeWindow closes the window as its close button does. The
// application quits in answer, and RunInGUIWindow returns. It is also the
// driver's safety net on a panic, and a harmless no-op if the window is
// already on its way down by the time it runs (checkApplicationTerminate
// already started that): performClose: on a closing window, or
// windowShouldClose: seeing the quit that is already under way, just does
// nothing further.
func (sm *smoke) closeWindow() {
	if sm.window == 0 {
		vtui.FrameManager.PostTask(func() { vtui.FrameManager.EmitCommand(vtui.CmQuit, nil) })
		return
	}
	sm.logf("info  closing the window with performClose:")
	// onMainAsync, not onMain: checkApplicationTerminate above may already
	// have the app quitting by the time this defer runs (that is the point
	// of it being a safety net), and onMain's waitUntilDone:true would then
	// wait out its own 10s timeout for a run loop that has already stopped.
	onMainAsync(func() { sm.window.Send(s("performClose:"), objc.ID(0)) })
}

// checkAfterShutdown calls the host methods f4's shutdown path calls once
// RunGui/RunInGUIWindow has returned -- the deferred SaveSession's
// GetWindowPosition and SetWindowPosition among them -- plus SetWindowTitle,
// ResizeWindow, ToggleWindowMaximized and a Flush, which take the same
// route. The window, view and delegate are already released by then; before
// vtui #1571 was fixed, close() left them non-zero and the "already on the
// main thread" shortcut in runOnMain/runOnMainAndWait skipped the loopDone
// check that would otherwise have caught it, so these went to freed AppKit
// objects instead of quietly doing nothing.
func (sm *smoke) checkAfterShutdown() {
	defer func() {
		if r := recover(); r != nil {
			sm.check(false, "after-shutdown calls", "panic: %v\n%s", r, debug.Stack())
		}
	}()
	vtui.SetWindowTitle("after shutdown")
	vtui.SetWindowPosition(5, 5)
	_, _, posOK := vtui.GetWindowPosition()
	vtui.FrameManager.ResizeWindow(70, 25)
	vtui.FrameManager.ToggleWindowMaximized()
	if scr := vtui.FrameManager.Screen(); scr != nil {
		scr.Flush()
	}
	sm.check(!posOK, "after-shutdown calls",
		"SetWindowTitle/SetWindowPosition/GetWindowPosition/ResizeWindow/ToggleWindowMaximized/Flush returned without crashing once RunInGUIWindow had returned (GetWindowPosition correctly reports ok=%v with no window left)", posOK)
}

func main() {
	out := flag.String("out", "cocoa-smoke-out", "directory for the report, frames and screenshots")
	timeout := flag.Duration("timeout", 2*time.Minute, "give up after this long")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	sm := &smoke{out: *out, probe: &probe{Desktop: vtui.NewDesktop()}}
	time.AfterFunc(*timeout, func() {
		sm.check(false, "timeout", "the run took longer than %v", *timeout)
		sm.writeReport()
		os.Exit(3)
	})

	if err := initObjC(); err != nil {
		sm.check(false, "setup", "%v", err)
		sm.writeReport()
		os.Exit(1)
	}

	start := time.Now()
	driverDone := make(chan struct{})
	err := vtui.RunInGUIWindow(initialCols, initialRows, "cocoa", "", 16, func() {
		vtui.FrameManager.Push(sm.probe)
		go func() {
			defer close(driverDone)
			sm.run()
		}()
	})
	sm.check(err == nil, "RunInGUIWindow", "returned %v after %v", err, time.Since(start).Round(time.Millisecond))
	// This runs on the same goroutine that called RunInGUIWindow, exactly
	// where f4's deferred SaveSession calls GetWindowPosition today: the
	// window, view and delegate are already released by now (vtui #1571).
	// A background goroutine would take the already-guarded queued path
	// instead of the one this is meant to catch a regression in, so this
	// has to stay right here, synchronous, not spawned off.
	sm.checkAfterShutdown()
	select {
	case <-driverDone:
		sm.check(true, "shutdown", "closing the window ended the event loop and RunInGUIWindow returned")
	case <-time.After(5 * time.Second):
		sm.check(false, "shutdown", "RunInGUIWindow returned while the driver was still running")
	}
	sm.writeReport()
	if sm.failed {
		os.Exit(1)
	}
}
