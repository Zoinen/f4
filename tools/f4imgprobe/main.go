//go:build windows

// f4imgprobe -- issue #805, and only issue #805.
//
// Two complaints, one shape: something repaints over the picture.
//
//   - classic conhost: the overlay window flickers and disappears. The console
//     window has no WS_CLIPCHILDREN, so every console repaint paints over a
//     child window of it and nothing invalidates the child afterwards
//     (docs/WINCON_805_HANDOVER.md F6). The proposed fix is an overlay that is
//     not a child at all: a top-level layered window that tracks the console
//     (step 3).
//   - Windows Terminal: the same build shows sixel sometimes and not others.
//     A sixel image in a text terminal lives in the text buffer, so the
//     suspicion is symmetrical -- writing text near or over the image erases
//     it, and f4 repaints its whole screen many times a second.
//
// So this probe does not survey the machine. It runs the two overlay
// mechanisms and the sixel path against the events suspected of erasing them,
// and asks the one question a program cannot answer for itself: is the picture
// still on the screen. Every answer is appended to the report immediately, so a
// run that is closed halfway is still worth attaching.
//
// Build: GOOS=windows GOARCH=amd64 go build -o f4imgprobe.exe ./tools/f4imgprobe
package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")
	version  = syscall.NewLazyDLL("version.dll")

	procGetConsoleWindow           = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procFlushConsoleInputBuffer    = kernel32.NewProc("FlushConsoleInputBuffer")
	procReadConsoleInputW          = kernel32.NewProc("ReadConsoleInputW")
	procGetNumberOfConsoleInputEvs = kernel32.NewProc("GetNumberOfConsoleInputEvents")
	procGetCurrentConsoleFontEx    = kernel32.NewProc("GetCurrentConsoleFontEx")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procOpenProcess                = kernel32.NewProc("OpenProcess")

	procGetClassNameW       = user32.NewProc("GetClassNameW")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procClientToScreen      = user32.NewProc("ClientToScreen")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procGetWindow           = user32.NewProc("GetWindow")
	procGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procPeekMessageW        = user32.NewProc("PeekMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procBeginPaint          = user32.NewProc("BeginPaint")
	procEndPaint            = user32.NewProc("EndPaint")
	procFillRect            = user32.NewProc("FillRect")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	procInvalidateRect      = user32.NewProc("InvalidateRect")

	procCreateSolidBrush   = gdi32.NewProc("CreateSolidBrush")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")

	procRtlGetVersion           = ntdll.NewProc("RtlGetVersion")
	procGetFileVersionInfoSizeW = version.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = version.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = version.NewProc("VerQueryValueW")
)

const (
	stdInput  = ^uintptr(9)
	stdOutput = ^uintptr(10)

	enableProcessedInput = 0x0001
	enableLineInput      = 0x0002
	enableEchoInput      = 0x0004
	enableVTProcessing   = 0x0004

	gwlExStyle = ^uintptr(19)
	gwHwndPrev = 3

	wsChild         = 0x40000000
	wsVisible       = 0x10000000
	wsPopup         = 0x80000000
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000
	wsExTopmost     = 0x00000008

	swShowNoActivate = 4
	swHide           = 0
	swpNoMove        = 0x0002
	swpNoSize        = 0x0001
	swpNoActivate    = 0x0010

	wmPaint       = 0x000F
	wmEraseBkgnd  = 0x0014
	wmNCHitTest   = 0x0084
	wmNull        = 0x0000
	htTransparent = ^uintptr(0) // (UINT_PTR)-1, the HTTRANSPARENT hit-test result

	pmRemove     = 0x0001
	acSrcOver    = 0x00
	acSrcAlpha   = 0x01
	ulwAlpha     = 0x0002
	dibRGBColors = 0
)

type point struct{ X, Y int32 }
type sizeT struct{ Cx, Cy int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type smallRect struct{ Left, Top, Right, Bottom int16 }
type coord struct{ X, Y int16 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type paintStruct struct {
	Hdc         uintptr
	Erase       int32
	RcPaint     rect
	Restore     int32
	IncUpdate   int32
	RgbReserved [32]byte
}

type blendFunction struct {
	BlendOp, BlendFlags, SourceConstantAlpha, AlphaFormat byte
}

type bitmapInfoHeader struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
}

type consoleFontInfoEx struct {
	Size       uint32
	Font       uint32
	FontSize   coord
	FontFamily uint32
	FontWeight uint32
	FaceName   [32]uint16
}

type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

type osVersionInfoEx struct {
	OSVersionInfoSize                             uint32
	MajorVersion, MinorVersion, BuildNumber       uint32
	PlatformId                                    uint32
	CSDVersion                                    [128]uint16
	ServicePackMajor, ServicePackMinor, SuiteMask uint16
	ProductType, Reserved                         byte
}

// ---------------------------------------------------------------- reporting --

var reportFile *os.File

// report writes one line to the screen and to the report at the same time, and
// flushes: a run that is killed with the window's close button must still leave
// everything measured so far on disk.
func report(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	fmt.Print(line + "\r\n")
	if reportFile != nil {
		reportFile.WriteString(line + "\r\n")
		reportFile.Sync()
	}
}

func section(title string) {
	report("")
	report("--- %s ---", title)
}

func main() {
	// Every window created here must live on the thread that pumps its
	// messages, and that pump runs between the questions on this thread.
	runtime.LockOSThread()

	exe, _ := os.Executable()
	path := "f4imgprobe-report.txt"
	if exe != "" {
		path = exe[:strings.LastIndexByte(exe, '\\')+1] + "f4imgprobe-report.txt"
	}
	f, err := os.Create(path)
	if err == nil {
		reportFile = f
		defer f.Close()
	}

	report("=== f4imgprobe 1 (issue #805: the picture disappears) ===")
	report("time: %s", time.Now().Format("2006-01-02 15:04:05 -0700"))
	report("report file: %s", path)
	fmt.Print("\r\nThis asks you a few times whether a picture is on the screen.\r\n")
	fmt.Print("Answer Y or N. Nothing is installed and nothing is changed.\r\n")

	describeHost()
	console := consoleWindow()
	class := classNameOf(console)

	if class == "ConsoleWindowClass" && isWindowVisible(console) {
		testConhostOverlays(console)
	} else {
		section("Overlay tests")
		report("skipped: this is not a classic console window (class %q), so an", class)
		report("overlay over the console handle cannot apply here. The picture on")
		report("this host has to travel as sixel, which the next section tests.")
	}

	testSixel()

	section("Done")
	report("Please attach %s to the issue.", path)
	fmt.Print("\r\nPress Enter to close. ")
	var dummy [1]byte
	os.Stdin.Read(dummy[:])
}

// ------------------------------------------------------------------- host ----

func describeHost() {
	section("Where this is running")
	v := osVersion()
	report("windows build: %d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	report("WT_SESSION: %s", envOr("WT_SESSION", "(not set)"))

	hwnd := consoleWindow()
	report("console window: %#x class %q visible=%v", hwnd, classNameOf(hwnd), isWindowVisible(hwnd))
	cr := clientRectOf(hwnd)
	report("console client area: %dx%d px", cr.Right-cr.Left, cr.Bottom-cr.Top)

	// The Windows Terminal build matters here: the complaint is that the same
	// version shows a picture sometimes and not others, so the exact file
	// version of the running terminal has to be in the report, not the name.
	if term := terminalProcessPath(); term != "" {
		report("terminal process: %s (file version %s)", term, fileVersion(term))
	}

	// The cell size decides the picture's geometry. Behind a pseudo console the
	// Win32 answer is a zero width, which is why f4 must ask the terminal
	// instead (handover F16/F17). Both are recorded so the field says which.
	var info consoleFontInfoEx
	info.Size = uint32(unsafe.Sizeof(info))
	if r, _, _ := procGetCurrentConsoleFontEx.Call(uintptr(stdHandle(stdOutput)), 0,
		uintptr(unsafe.Pointer(&info))); r != 0 {
		report("GetCurrentConsoleFontEx cell: %dx%d px", info.FontSize.X, info.FontSize.Y)
		if info.FontSize.X <= 0 {
			report("  (a zero width: this host must not be asked for the cell this way)")
		}
	}
	withVT(func() {
		report("DA1: %s", escapeAnswer(ask("\x1b[c", 'c', 900*time.Millisecond)))
		report("cell size (CSI 16 t): %s", escapeAnswer(ask("\x1b[16t", 't', 900*time.Millisecond)))
		report("text area (CSI 14 t): %s", escapeAnswer(ask("\x1b[14t", 't', 900*time.Millisecond)))
		report("text area in cells (CSI 18 t): %s", escapeAnswer(ask("\x1b[18t", 't', 900*time.Millisecond)))
	})
}

// ------------------------------------------------------- conhost overlays ----

// testConhostOverlays runs both overlay mechanisms against the same event: a
// console repaint under the picture.
//
// The child-window half of this test cannot ask anything while it runs. A
// window whose parent belongs to another process attaches the two threads'
// input queues (handover F7), and the first field run of this probe froze the
// console exactly there -- the probe was asking a question through an input
// queue its own test had just wedged. So the child test now narrates what is
// about to happen, runs unattended, destroys the window, and only then asks.
// Whether the input queue actually froze is measured rather than asked.
func testConhostOverlays(console uintptr) {
	inst, _, _ := procGetModuleHandleW.Call(0)
	x, y, w, h := pictureRect(console)

	section("Overlay test 1: a child window of the console (what f4 does today)")
	report("Expected to fail: the console window has no WS_CLIPCHILDREN, so its")
	report("own repaint paints over the child and nothing redraws the child after.")

	if !registerClass(inst, "f4imgprobeChild", childWndProc) {
		report("could not register a window class; child overlay test skipped")
	} else {
		fmt.Print("\r\nWATCH THE MIDDLE OF THIS WINDOW. For about six seconds a RED square\r\n")
		fmt.Print("will appear there, then lines of text will be printed underneath it.\r\n")
		fmt.Print("Do not type or click during that time -- this test is expected to\r\n")
		fmt.Print("freeze the console, and the probe measures that by itself.\r\n")
		fmt.Print("Starting in three seconds...\r\n")
		pump(3 * time.Second)

		cx, cy := x-clientOriginX(console), y-clientOriginY(console)
		child, _, _ := procCreateWindowExW.Call(0,
			uintptr(unsafe.Pointer(utf16Ptr("f4imgprobeChild"))), 0,
			uintptr(wsChild|wsVisible),
			uintptr(uint32(cx)), uintptr(uint32(cy)), uintptr(w), uintptr(h),
			console, 0, inst, 0)
		if child == 0 {
			report("CreateWindowEx (child of the console) failed")
		} else {
			report("created a child window of the console, %dx%d px at %d,%d in its client area", w, h, cx, cy)
			pump(1500 * time.Millisecond)

			// F7/F8, measured: if the console's window thread has stopped
			// answering while our child window exists, that is the freeze the
			// issue describes, and it is a number rather than an impression.
			// Deliberately not measured from here. A cross-process child
			// attaches this thread's input queue to the console's, so a
			// message sent from this thread is exactly the one call that
			// still succeeds while everything the user does is frozen. The
			// first field run reported "answered: true" while the tester
			// could not type. The tester's answer below is the measurement.
			report("console responsiveness during the child window: asked, not measured")
			report("(a message sent from this thread cannot detect the freeze: F7 attaches")
			report(" this very thread to the console's input queue.)")

			fmt.Print("\r\n")
			for i := 0; i < 12; i++ {
				fmt.Printf("console repaint line %d\r\n", i)
			}
			pump(2 * time.Second)
			procDestroyWindow.Call(child)
			pump(300 * time.Millisecond)

			report("child overlay: %s", askChoice("What happened to the RED square?",
				"it appeared and stayed whole, undamaged by the text",
				"it appeared, then part of it was erased or cut away",
				"it appeared and then vanished completely",
				"it never appeared at all"))
			report("input during the child window: %s", askChoice(
				"While the red square was on screen, did the console stop responding?",
				"no, everything stayed responsive",
				"yes, typing and clicking did nothing until the square went away",
				"cannot tell"))
		}
	}

	section("Overlay test 2: a top-level layered window (the proposed fix)")
	report("No parent and no owner, so no input queue is attached; positioned and")
	report("filled in one UpdateLayeredWindow call, then lifted just above the console.")

	if !registerClass(inst, "f4imgprobeLayered", defaultWndProc) {
		report("could not register the layered window class")
		return
	}
	top, _, _ := procCreateWindowExW.Call(
		uintptr(wsExLayered|wsExTransparent|wsExToolWindow|wsExNoActivate),
		uintptr(unsafe.Pointer(utf16Ptr("f4imgprobeLayered"))), 0,
		uintptr(wsPopup),
		uintptr(uint32(x)), uintptr(uint32(y)), uintptr(w), uintptr(h),
		0, 0, inst, 0)
	if top == 0 {
		report("CreateWindowEx (top-level layered) failed")
		return
	}
	defer procDestroyWindow.Call(top)

	if !drawLayered(top, x, y, w, h) {
		report("UpdateLayeredWindow failed: this mechanism does not work on this build")
		return
	}
	procShowWindow.Call(top, swShowNoActivate)
	// SetWindowPos(hwnd, console, ...) would put the window *behind* the
	// console: the second argument names what to sit after. Directly above the
	// console means after whatever precedes it in the z-order, or HWND_TOP.
	after, _, _ := procGetWindow.Call(console, gwHwndPrev)
	if after != 0 && windowLongOf(after, gwlExStyle)&wsExTopmost != 0 {
		after = 0
	}
	procSetWindowPos.Call(top, after, 0, 0, 0, 0, uintptr(swpNoMove|swpNoSize|swpNoActivate))
	prev, _, _ := procGetWindow.Call(console, gwHwndPrev)
	report("placed directly above the console: %v (console's GW_HWNDPREV %#x, ours %#x)",
		prev == top, prev, top)
	pump(500 * time.Millisecond)

	first := askChoice("Do you see a BLUE square over the console window?",
		"yes, a solid blue square",
		"yes, but it is somewhere unexpected or the wrong shape",
		"no, nothing blue")
	report("layered overlay visible at first: %s", first)
	if strings.HasPrefix(first, "no") {
		report("layered overlay survives a console repaint: not asked (nothing was visible)")
		return
	}

	ok, d := consoleResponds(console, 2*time.Second)
	report("console answered a message while the layered window existed: %v (%v)", ok, d)

	fmt.Print("\r\n")
	for i := 0; i < 12; i++ {
		fmt.Printf("console repaint line %d\r\n", i)
	}
	pump(700 * time.Millisecond)
	report("layered overlay after a console repaint: %s", askChoice(
		"What happened to the BLUE square while that text was printed?",
		"nothing, it stayed whole",
		"part of it was erased or cut away",
		"it vanished completely",
		"it flickered but came back whole"))

	fmt.Print("\r\nNow please MOVE the console window with the mouse, then come back here.\r\n")
	report("layered overlay when the console moves: %s", askChoice(
		"After moving the window, where is the blue square?",
		"it moved with the console window",
		"it stayed where it was on the screen",
		"it disappeared"))
	report("(f4 would run a tracking timer; this probe deliberately does not, so")
	report(" 'it stayed where it was' is the expected answer and means the tracker is required.)")
	procShowWindow.Call(top, swHide)
}

// consoleResponds asks the console's window thread a question that costs it
// nothing and reports whether it answered. A wedged console (F8) is the whole
// second half of the conhost complaint, and this turns it into a measurement.
func consoleResponds(console uintptr, timeout time.Duration) (bool, time.Duration) {
	const smtoAbortIfHung = 0x0002
	start := time.Now()
	var result uintptr
	r, _, _ := procSendMessageTimeoutW.Call(console, wmNull, 0, 0, smtoAbortIfHung,
		uintptr(timeout/time.Millisecond), uintptr(unsafe.Pointer(&result)))
	return r != 0, time.Since(start).Round(time.Millisecond)
}

// pictureRect is a square in the middle of the console's client area, in screen
// coordinates -- the same arithmetic the overlay does.
func pictureRect(console uintptr) (x, y, w, h int32) {
	cr := clientRectOf(console)
	origin := point{}
	procClientToScreen.Call(console, uintptr(unsafe.Pointer(&origin)))
	w, h = 160, 160
	if cr.Right-cr.Left < w*2 {
		w = (cr.Right - cr.Left) / 2
	}
	if cr.Bottom-cr.Top < h*2 {
		h = (cr.Bottom - cr.Top) / 2
	}
	return origin.X + (cr.Right-cr.Left-w)/2, origin.Y + (cr.Bottom-cr.Top-h)/2, w, h
}

var childWndProc = syscall.NewCallback(func(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		brush, _, _ := procCreateSolidBrush.Call(0x000000FF) // COLORREF is BGR: red
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&ps.RcPaint)), brush)
		procDeleteObject.Call(brush)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmEraseBkgnd:
		return 1
	case wmNCHitTest:
		return htTransparent
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wparam, lparam)
	return r
})

var defaultWndProc = syscall.NewCallback(func(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wparam, lparam)
	return r
})

func registerClass(inst uintptr, name string, proc uintptr) bool {
	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		WndProc:   proc,
		Instance:  inst,
		ClassName: utf16Ptr(name),
	}
	atom, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	return atom != 0
}

// drawLayered fills the window with a blue square in one UpdateLayeredWindow
// call: position, size and pixels together, which is what makes the "black
// rectangle before the first paint" impossible.
func drawLayered(hwnd uintptr, x, y, w, h int32) bool {
	screenDC, _, _ := procGetDC.Call(0)
	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	defer func() {
		procDeleteDC.Call(memDC)
		procReleaseDC.Call(0, screenDC)
	}()
	bih := bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    w,
		Height:   -h, // top-down
		Planes:   1,
		BitCount: 32,
	}
	var bits unsafe.Pointer
	dib, _, _ := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bih)), dibRGBColors,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 || bits == nil {
		return false
	}
	defer procDeleteObject.Call(dib)
	px := unsafe.Slice((*byte)(bits), int(w)*int(h)*4)
	for i := 0; i < int(w)*int(h); i++ {
		// Opaque blue, premultiplied (alpha 255 leaves the values unchanged).
		px[i*4+0] = 230 // B
		px[i*4+1] = 40  // G
		px[i*4+2] = 20  // R
		px[i*4+3] = 255 // A
	}
	old, _, _ := procSelectObject.Call(memDC, dib)
	defer procSelectObject.Call(memDC, old)
	pt := point{X: x, Y: y}
	sz := sizeT{Cx: w, Cy: h}
	src := point{}
	bf := blendFunction{BlendOp: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}
	r, _, _ := procUpdateLayeredWindow.Call(hwnd, screenDC,
		uintptr(unsafe.Pointer(&pt)), uintptr(unsafe.Pointer(&sz)),
		memDC, uintptr(unsafe.Pointer(&src)), 0,
		uintptr(unsafe.Pointer(&bf)), ulwAlpha)
	return r != 0
}

// ------------------------------------------------------------------ sixel ----

// testSixel asks what erases a sixel image. The complaint is that the same
// Windows Terminal shows the picture sometimes and not others; if writing text
// erases it, then "sometimes" is simply "whenever f4 repainted next", which is
// a bug with an address rather than a mystery.
func testSixel() {
	section("Sixel test: does the picture survive what f4 does next")
	outMode, ok := consoleMode(stdOutput)
	if !ok {
		report("skipped: stdout is not a console")
		return
	}
	setConsoleMode(stdOutput, outMode|enableVTProcessing)
	defer setConsoleMode(stdOutput, outMode)

	fmt.Print("\x1b[2J\x1b[H")
	fmt.Print("A square should appear below: red on the left, blue on the right.\r\n\r\n")
	os.Stdout.WriteString(sixelSquare())
	fmt.Print("\r\n\r\n\r\n\r\n\r\n\r\n")
	s1 := askChoice("Do you see a coloured area below the text?",
		"yes: red on the left, blue on the right, and it looks square",
		"yes: red and blue, but the shape is not square",
		"yes, but the colours or the layout are different from that",
		"no, nothing coloured appeared")
	report("sixel image appeared at all: %s", s1)
	if strings.HasPrefix(s1, "no,") {
		report("(no image: either this terminal has no sixel support, or it is")
		report(" switched off in its settings -- both belong in the report.)")
		return
	}
	report("(the image is 90x90 pixels: two halves of 45x90 each. If it looks")
	report(" taller or wider than a square, the terminal is scaling it, which is")
	report(" a finding about the cell size and not about the image.)")

	// From here the shape no longer matters; only what survives does. Each
	// question offers "partly erased" as its own answer, because that is what
	// a repaint over an image looks like and it is neither yes nor no.
	survives := func(what string) string {
		return askChoice("What happened to the coloured area after "+what+"?",
			"nothing, it is completely unchanged",
			"part of it was blanked or covered, the rest is still there",
			"it disappeared completely",
			"it is still there but has moved on the screen",
			"it moved up past the top edge and was cut off there")
	}

	// 1. Text written elsewhere: the ordinary case for f4, whose panels
	//    repaint constantly while a picture is shown.
	fmt.Print("text written below the image, nothing near it\r\n")
	pause(600 * time.Millisecond)
	report("survives text written elsewhere: %s", survives("that line of text"))

	// 2. A repaint of the rows the image occupies: cursor moved with CUP and
	//    the line erased, which is what a full-screen redraw does.
	fmt.Print("\x1b[3;1H\x1b[K\x1b[4;1H\x1b[K\x1b[12;1H")
	pause(600 * time.Millisecond)
	report("survives ESC[K over its own top rows: %s", survives("erasing two of its own top rows"))

	// 3. Scrolling: new output at the bottom pushes the image up.
	for i := 0; i < 3; i++ {
		fmt.Printf("scrolling line %d\r\n", i)
	}
	pause(600 * time.Millisecond)
	report("survives scrolling: %s", survives("three lines of scrolling"))

	// 4. The alternate screen. f4 uses it, and an image drawn on one buffer
	//    has no reason to exist on the other.
	fmt.Print("\x1b[?1049h\x1b[2J\x1b[H")
	fmt.Print("This is the alternate screen. The same image is drawn here.\r\n\r\n")
	os.Stdout.WriteString(sixelSquare())
	fmt.Print("\r\n\r\n\r\n\r\n\r\n\r\n")
	s5 := askChoice("Do you see the coloured area on this alternate screen?",
		"yes, the same as before",
		"yes, but damaged or incomplete",
		"no, nothing")
	fmt.Print("\x1b[?1049l")
	report("sixel works on the alternate screen: %s", s5)

	// 5. Redrawn repeatedly, which is what an animation or a repainting file
	//    manager does. Flicker here is the visible form of the complaint.
	fmt.Print("\x1b[2J\x1b[H")
	fmt.Print("The image is now being redrawn 20 times in a row.\r\n\r\n")
	for i := 0; i < 20; i++ {
		fmt.Print("\x1b[3;1H")
		os.Stdout.WriteString(sixelSquare())
		time.Sleep(120 * time.Millisecond)
	}
	fmt.Print("\x1b[14;1H")
	report("steady when redrawn 20 times: %s", askChoice("During those redraws the image was:",
		"steady, no visible change",
		"flickering or blinking",
		"disappearing and coming back",
		"gone after the first few redraws"))

	fmt.Print("\r\nNow please RESIZE the terminal window with the mouse, then come back.\r\n")
	report("survives a window resize: %s", survives("resizing the window"))
}

// sixelSquare is a 90x90 square, red on the left half and blue on the right.
// Deliberately small: the point is whether it is there, not what it is.
func sixelSquare() string {
	var b strings.Builder
	// P1=7 is a 1:1 pixel aspect ratio. Without it the DCS default is 2:1
	// and the terminal draws the image twice as tall as it is wide -- the
	// first field runs reported "not square" and that was this header, not
	// the terminal and not f4.
	b.WriteString("\x1bP7;0;0q#0;2;90;10;10#1;2;10;10;90")
	for band := 0; band < 15; band++ { // 15 bands of 6 pixels = 90 tall
		b.WriteString("#0!45~$#1!45?!45~-") // 45 red + 45 blue = 90 px wide, 90 tall
	}
	b.WriteString("\x1b\\")
	return b.String()
}

// ------------------------------------------------------------- asking you ----

// askChoice asks a question with a numbered list of answers. Every question
// about a picture is asked this way, because a picture has more states than
// yes and no: the first field run met a coloured area that was not the shape
// the question described, and rows that had been blanked while the rest of the
// image survived, and neither could be answered honestly with Y or N. An
// answer nobody can give truthfully is worse than no answer, because it lands
// in the report looking like data.
func askChoice(question string, options ...string) string {
	if len(options) == 0 || len(options) > 9 {
		return "not asked (bad question)"
	}
	inMode, ok := consoleMode(stdInput)
	if !ok {
		return "not asked (no console input)"
	}
	setConsoleMode(stdInput, inMode&^(enableLineInput|enableEchoInput|enableProcessedInput))
	defer setConsoleMode(stdInput, inMode)
	procFlushConsoleInputBuffer.Call(stdHandle(stdInput))

	fmt.Print("\r\n" + question + "\r\n")
	for i, opt := range options {
		fmt.Printf("  %d) %s\r\n", i+1, opt)
	}
	fmt.Printf("  0) something else -- describe it yourself at the top of the report\r\n")
	fmt.Printf("Press 0-%d: ", len(options))
	// The pump keeps running while the question waits: the overlay windows
	// created above must stay alive and painted while they are being judged.

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		pumpOnce()
		ch := readCharNonBlocking()
		switch {
		case ch == '0':
			fmt.Print("0\r\n")
			return "none of the offered answers (see the tester's own note)"
		case ch >= '1' && ch <= rune('0'+len(options)):
			fmt.Printf("%c\r\n", ch)
			return options[ch-'1']
		}
		time.Sleep(15 * time.Millisecond)
	}
	fmt.Print("(no answer)\r\n")
	return "no answer within five minutes"
}

// pause keeps the message pump running instead of sleeping, so a window that
// needs repainting during the wait gets the chance.
func pause(d time.Duration) { pump(d) }

func pump(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		pumpOnce()
		time.Sleep(10 * time.Millisecond)
	}
}

func pumpOnce() {
	var m msg
	for {
		r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
		if r == 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func readCharNonBlocking() rune {
	h := stdHandle(stdInput)
	var pending uint32
	if r, _, _ := procGetNumberOfConsoleInputEvs.Call(h, uintptr(unsafe.Pointer(&pending))); r == 0 || pending == 0 {
		return 0
	}
	var rec struct {
		EventType       uint16
		_               uint16
		KeyDown         int32
		RepeatCount     uint16
		VirtualKeyCode  uint16
		VirtualScanCode uint16
		UnicodeChar     uint16
		ControlKeyState uint32
	}
	var read uint32
	if r, _, _ := procReadConsoleInputW.Call(h, uintptr(unsafe.Pointer(&rec)), 1,
		uintptr(unsafe.Pointer(&read))); r == 0 || read == 0 {
		return 0
	}
	if rec.EventType != 1 || rec.KeyDown == 0 {
		return 0
	}
	return rune(rec.UnicodeChar)
}

// ask sends a VT query and reads the answer, with echo off so the reply does
// not appear on the screen.
func ask(query string, final byte, timeout time.Duration) string {
	procFlushConsoleInputBuffer.Call(stdHandle(stdInput))
	os.Stdout.WriteString(query)
	deadline := time.Now().Add(timeout)
	var sb strings.Builder
	for time.Now().Before(deadline) {
		ch := readCharNonBlocking()
		if ch == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		sb.WriteRune(ch)
		if byte(ch) == final && sb.Len() > 1 {
			return sb.String()
		}
	}
	if sb.Len() == 0 {
		return ""
	}
	return sb.String()
}

// withVT turns on VT output and raw input for the duration of f, then puts both
// modes back exactly as they were.
func withVT(f func()) {
	inMode, inOK := consoleMode(stdInput)
	outMode, outOK := consoleMode(stdOutput)
	if !inOK || !outOK {
		report("VT queries skipped: this needs a console on both stdin and stdout")
		return
	}
	setConsoleMode(stdOutput, outMode|enableVTProcessing)
	setConsoleMode(stdInput, inMode&^(enableLineInput|enableEchoInput|enableProcessedInput))
	defer func() {
		setConsoleMode(stdInput, inMode)
		setConsoleMode(stdOutput, outMode)
	}()
	f()
}

// ----------------------------------------------------------------- plumbing --

func stdHandle(which uintptr) uintptr {
	h, _ := syscall.GetStdHandle(int(int32(uint32(which))))
	return uintptr(h)
}

func consoleMode(which uintptr) (uint32, bool) {
	var m uint32
	r, _, _ := procGetConsoleMode.Call(stdHandle(which), uintptr(unsafe.Pointer(&m)))
	return m, r != 0
}

func setConsoleMode(which uintptr, mode uint32) {
	procSetConsoleMode.Call(stdHandle(which), uintptr(mode))
}

func consoleWindow() uintptr {
	h, _, _ := procGetConsoleWindow.Call()
	return h
}

func classNameOf(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	buf := make([]uint16, 128)
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func isWindowVisible(hwnd uintptr) bool {
	r, _, _ := procIsWindowVisible.Call(hwnd)
	return r != 0
}

func windowLongOf(hwnd, index uintptr) uintptr {
	v, _, _ := procGetWindowLongPtrW.Call(hwnd, index)
	return v
}

func clientOriginX(hwnd uintptr) int32 { p := clientOrigin(hwnd); return p.X }
func clientOriginY(hwnd uintptr) int32 { p := clientOrigin(hwnd); return p.Y }

func clientOrigin(hwnd uintptr) point {
	p := point{}
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&p)))
	return p
}

func clientRectOf(hwnd uintptr) rect {
	var r rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func osVersion() osVersionInfoEx {
	var v osVersionInfoEx
	v.OSVersionInfoSize = uint32(unsafe.Sizeof(v))
	procRtlGetVersion.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

// terminalProcessPath finds the terminal drawing this console: for a pseudo
// console that is the owner window's process, which is Windows Terminal itself.
func terminalProcessPath() string {
	hwnd := consoleWindow()
	if hwnd == 0 {
		return ""
	}
	owner, _, _ := procGetWindow.Call(hwnd, 4 /*GW_OWNER*/)
	target := owner
	if target == 0 {
		target = hwnd
	}
	var pid uint32
	procGetWindowThreadPID.Call(target, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	h, _, _ := procOpenProcess.Call(0x1000 /*QUERY_LIMITED_INFORMATION*/, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if r, _, _ := procQueryFullProcessImageNameW.Call(h, 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

func fileVersion(path string) string {
	p := utf16Ptr(path)
	var ignored uint32
	sz, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&ignored)))
	if sz == 0 {
		return "unknown"
	}
	buf := make([]byte, sz)
	if ok, _, _ := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(p)), 0, sz,
		uintptr(unsafe.Pointer(&buf[0]))); ok == 0 {
		return "unknown"
	}
	var ptr uintptr
	var n uint32
	root := utf16Ptr(`\`)
	if ok, _, _ := procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&ptr)), uintptr(unsafe.Pointer(&n))); ok == 0 || ptr == 0 {
		return "unknown"
	}
	// The pointer VerQueryValue hands back points inside buf; read the fixed
	// info through the slice rather than through the raw address.
	off := int(ptr - uintptr(unsafe.Pointer(&buf[0])))
	if off < 0 || off+16 > len(buf) {
		return "unknown"
	}
	ms := uint32(buf[off+8]) | uint32(buf[off+9])<<8 | uint32(buf[off+10])<<16 | uint32(buf[off+11])<<24
	ls := uint32(buf[off+12]) | uint32(buf[off+13])<<8 | uint32(buf[off+14])<<16 | uint32(buf[off+15])<<24
	return fmt.Sprintf("%d.%d.%d.%d", ms>>16, ms&0xffff, ls>>16, ls&0xffff)
}

func escapeAnswer(s string) string {
	if s == "" {
		return "(no answer)"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 0x1b:
			b.WriteString("ESC")
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, "<%02X>", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

func envOr(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	return fallback
}
