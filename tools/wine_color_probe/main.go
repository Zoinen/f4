//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                         = syscall.NewLazyDLL("kernel32.dll")
	ntdll                            = syscall.NewLazyDLL("ntdll.dll")
	procWineGetVersion               = ntdll.NewProc("wine_get_version")
	procCreateConsoleScreenBuffer    = kernel32.NewProc("CreateConsoleScreenBuffer")
	procSetConsoleActiveScreenBuffer = kernel32.NewProc("SetConsoleActiveScreenBuffer")
	procGetConsoleScreenBufferInfo   = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procWriteConsoleOutputW          = kernel32.NewProc("WriteConsoleOutputW")
	procSetConsoleCursorPosition     = kernel32.NewProc("SetConsoleCursorPosition")
	procSetConsoleWindowInfo         = kernel32.NewProc("SetConsoleWindowInfo")
)

type coord struct {
	X int16
	Y int16
}

type smallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

type consoleScreenBufferInfo struct {
	dwSize              coord
	dwCursorPosition    coord
	wAttributes         uint16
	srWindow            smallRect
	dwMaximumWindowSize coord
}

type charInfo struct {
	UnicodeChar uint16
	Attributes  uint16
}

const (
	fgBlue      uint16 = 0x0001
	fgGreen     uint16 = 0x0002
	fgRed       uint16 = 0x0004
	fgIntensity uint16 = 0x0008

	bgBlue      uint16 = 0x0010
	bgGreen     uint16 = 0x0020
	bgRed       uint16 = 0x0040
	bgIntensity uint16 = 0x0080

	fgLightCyan uint16 = fgGreen | fgBlue | fgIntensity         // 0x000B (11)
	fgLightGray uint16 = fgRed | fgGreen | fgBlue               // 0x0007 (7)
	fgWhite     uint16 = fgRed | fgGreen | fgBlue | fgIntensity // 0x000F (15)
	fgYellow    uint16 = fgRed | fgGreen | fgIntensity          // 0x000E (14)
	fgDarkGray  uint16 = fgIntensity                            // 0x0008 (8)
	fgBlack     uint16 = 0x0000

	bgDarkBlue  uint16 = bgBlue // 0x0010 (16)
	bgBlack     uint16 = 0x0000
	bgCyan      uint16 = bgGreen | bgBlue         // 0x0030 (48)
	bgLightGray uint16 = bgRed | bgGreen | bgBlue // 0x0070 (112)
	bgBrownDark uint16 = bgRed | bgGreen          // 0x0060 (Radiola-like)
)

type probeReport struct {
	sb strings.Builder
}

func (r *probeReport) Log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)
	r.sb.WriteString(msg + "\n")
}

func getWineVersion() string {
	if procWineGetVersion.Find() != nil {
		return "Native Windows (Not Wine)"
	}
	r1, _, _ := procWineGetVersion.Call()
	if r1 == 0 {
		return "Wine (version unknown)"
	}
	var buf []byte
	ptr := (*byte)(unsafe.Pointer(r1))
	for i := 0; i < 256; i++ {
		b := *(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + uintptr(i)))
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return "Wine " + string(buf)
}

func writeGrid(h syscall.Handle, x, y, w, hCount int16, cells []charInfo) bool {
	bufSize := uintptr(uint32(uint16(w)) | (uint32(uint16(hCount)) << 16))
	bufCoord := uintptr(0)
	region := smallRect{Left: x, Top: y, Right: x + w - 1, Bottom: y + hCount - 1}
	r1, _, _ := procWriteConsoleOutputW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&cells[0])),
		bufSize,
		bufCoord,
		uintptr(unsafe.Pointer(&region)),
	)
	return r1 != 0
}

func makeRow(text string, textAttr uint16, width int, fillAttr uint16) []charInfo {
	row := make([]charInfo, width)
	runes := []rune(text)
	for i := 0; i < width; i++ {
		if i < len(runes) {
			row[i] = charInfo{UnicodeChar: uint16(runes[i]), Attributes: textAttr}
		} else {
			row[i] = charInfo{UnicodeChar: ' ', Attributes: fillAttr}
		}
	}
	return row
}

func main() {
	report := &probeReport{}
	report.Log("=================================================================")
	report.Log("       WINE CONSOLE COLOR & ATTRIBUTE PROBE (Issue #536)        ")
	report.Log("=================================================================")
	report.Log("Environment: %s", getWineVersion())
	report.Log("Timestamp:   %s", time.Now().Format(time.RFC3339))

	hStdOut, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || hStdOut == 0 || hStdOut == syscall.InvalidHandle {
		report.Log("FATAL: Invalid STD_OUTPUT_HANDLE: %v", err)
		_ = os.WriteFile("wine_color_probe_report.txt", []byte(report.sb.String()), 0644)
		return
	}

	var csbi consoleScreenBufferInfo
	procGetConsoleScreenBufferInfo.Call(uintptr(hStdOut), uintptr(unsafe.Pointer(&csbi)))
	cols := csbi.srWindow.Right - csbi.srWindow.Left + 1
	rows := csbi.srWindow.Bottom - csbi.srWindow.Top + 1
	report.Log("Console Buffer: dwSize=%dx%d, srWindow=%dx%d (L%d T%d R%d B%d)",
		csbi.dwSize.X, csbi.dwSize.Y, cols, rows,
		csbi.srWindow.Left, csbi.srWindow.Top, csbi.srWindow.Right, csbi.srWindow.Bottom)

	if cols < 80 {
		cols = 80
	}

	// Create a secondary screen buffer like f4's Win32ConsoleRenderer does
	r1, _, _ := procCreateConsoleScreenBuffer.Call(
		uintptr(0xC0000000), // GENERIC_READ | GENERIC_WRITE
		uintptr(3),          // FILE_SHARE_READ | FILE_SHARE_WRITE
		0,
		uintptr(1), // CONSOLE_TEXTMODE_BUFFER
		0,
	)
	var hTarget syscall.Handle
	createdDedicated := false
	if r1 != 0 && syscall.Handle(r1) != syscall.InvalidHandle {
		hTarget = syscall.Handle(r1)
		procSetConsoleActiveScreenBuffer.Call(uintptr(hTarget))
		createdDedicated = true
		report.Log("Secondary Console Screen Buffer created: Handle=0x%X", hTarget)
	} else {
		hTarget = hStdOut
		report.Log("Secondary buffer creation failed, using STD_OUTPUT_HANDLE")
	}

	defer func() {
		if createdDedicated {
			procSetConsoleActiveScreenBuffer.Call(uintptr(hStdOut))
			syscall.CloseHandle(hTarget)
		}
		_ = os.WriteFile("wine_color_probe_report.txt", []byte(report.sb.String()), 0644)
		fmt.Println("\n[!] Full report saved to wine_color_probe_report.txt")
	}()

	report.Log("\n--- RUNNING 10 HYPOTHESIS TESTS ---\n")

	// -------------------------------------------------------------------------
	// HYPOTHESIS 1: Same BG (Blue 0x10), Different FG transition between lines
	// Row 0 ends with Blue BG (0x1F). Row 1 starts with Blue BG, Cyan FG (0x1B).
	// -------------------------------------------------------------------------
	h1Cells := make([]charInfo, 0, int(cols)*2)
	h1Row0 := makeRow(" [H1 Status Line - White on Blue] ", bgDarkBlue|fgWhite, int(cols), bgDarkBlue|fgWhite)
	h1Row1 := make([]charInfo, cols)
	kw := []rune("package main // Should be Cyan on Blue")
	for i := 0; i < int(cols); i++ {
		if i < len(kw) {
			if i < 7 { // "package"
				h1Row1[i] = charInfo{UnicodeChar: uint16(kw[i]), Attributes: bgDarkBlue | fgLightCyan}
			} else {
				h1Row1[i] = charInfo{UnicodeChar: uint16(kw[i]), Attributes: bgDarkBlue | fgLightGray}
			}
		} else {
			h1Row1[i] = charInfo{UnicodeChar: ' ', Attributes: bgDarkBlue | fgLightGray}
		}
	}
	h1Cells = append(h1Cells, h1Row0...)
	h1Cells = append(h1Cells, h1Row1...)
	ok1 := writeGrid(hTarget, 0, 0, cols, 2, h1Cells)
	report.Log("Hypothesis 1 (Row Transition with Identical BG 0x10, Different FG 0x1B vs 0x1F): Write=%v", ok1)

	// -------------------------------------------------------------------------
	// HYPOTHESIS 2: Row 0 ends with Black FG on Blue BG (0x10) -> Row 1 starts with 0x19
	// -------------------------------------------------------------------------
	h2Cells := make([]charInfo, 0, int(cols)*2)
	h2Row0 := makeRow(" [H2 TopBar Black on Blue 0x10] ", bgDarkBlue|fgBlack, int(cols), bgDarkBlue|fgBlack)
	h2Row1 := makeRow("package main // FG: Light Blue (0x19), BG: Blue (0x10)", bgDarkBlue|(fgBlue|fgIntensity), int(cols), bgDarkBlue|fgLightGray)
	h2Cells = append(h2Cells, h2Row0...)
	h2Cells = append(h2Cells, h2Row1...)
	ok2 := writeGrid(hTarget, 0, 2, cols, 2, h2Cells)
	report.Log("Hypothesis 2 (Row 0 ends with FG:Black/BG:Blue 0x10 -> Row 1 FG:BrightBlue/BG:Blue 0x19): Write=%v", ok2)

	// -------------------------------------------------------------------------
	// HYPOTHESIS 3: Leading Spaces with Editor.Text (0x1F) before Keyword (0x1B)
	// -------------------------------------------------------------------------
	h3Cells := make([]charInfo, 0, int(cols)*2)
	h3Row0 := makeRow(" [H3 Status Line] ", bgDarkBlue|fgWhite, int(cols), bgDarkBlue|fgWhite)
	h3Row1 := make([]charInfo, cols)
	for i := 0; i < int(cols); i++ {
		h3Row1[i] = charInfo{UnicodeChar: ' ', Attributes: bgDarkBlue | fgLightGray}
	}
	h3Text := "    func main() // 4 leading spaces"
	for i, r := range []rune(h3Text) {
		attr := bgDarkBlue | fgLightGray
		if i >= 4 && i < 8 { // "func"
			attr = bgDarkBlue | fgLightCyan
		}
		h3Row1[i] = charInfo{UnicodeChar: uint16(r), Attributes: attr}
	}
	h3Cells = append(h3Cells, h3Row0...)
	h3Cells = append(h3Cells, h3Row1...)
	ok3 := writeGrid(hTarget, 0, 4, cols, 2, h3Cells)
	report.Log("Hypothesis 3 (Leading spaces with default BG before keyword token): Write=%v", ok3)

	// -------------------------------------------------------------------------
	// HYPOTHESIS 4: Extended Win32 attribute bits (COMMON_LVB_* > 0xFF) contamination
	// -------------------------------------------------------------------------
	h4Cells := make([]charInfo, 0, int(cols)*2)
	h4Row0 := makeRow(" [H4 Extended Win32 Bits: 0x011B with LVB_LEADING_BYTE] ", bgDarkBlue|fgWhite, int(cols), bgDarkBlue|fgWhite)
	h4Row1 := makeRow("package main // Attribute has 0x0100 bit set", bgDarkBlue|fgLightCyan|0x0100, int(cols), bgDarkBlue|fgLightGray)
	h4Cells = append(h4Cells, h4Row0...)
	h4Cells = append(h4Cells, h4Row1...)
	ok4 := writeGrid(hTarget, 0, 6, cols, 2, h4Cells)
	report.Log("Hypothesis 4 (Attributes with upper LVB bits 0x0100 set): Write=%v", ok4)

	// -------------------------------------------------------------------------
	// HYPOTHESIS 5: Effect of SetConsoleWindowInfo called immediately before Write
	// -------------------------------------------------------------------------
	rect := smallRect{Left: 0, Top: 0, Right: cols - 1, Bottom: rows - 1}
	procSetConsoleWindowInfo.Call(uintptr(hTarget), 1, uintptr(unsafe.Pointer(&rect)))
	h5Cells := make([]charInfo, 0, int(cols)*2)
	h5Row0 := makeRow(" [H5 SetConsoleWindowInfo reset right before write] ", bgDarkBlue|fgWhite, int(cols), bgDarkBlue|fgWhite)
	h5Row1 := makeRow("package main // Window position was reset prior to this call", bgDarkBlue|fgLightCyan, int(cols), bgDarkBlue|fgLightGray)
	h5Cells = append(h5Cells, h5Row0...)
	h5Cells = append(h5Cells, h5Row1...)
	ok5 := writeGrid(hTarget, 0, 8, cols, 2, h5Cells)
	report.Log("Hypothesis 5 (SetConsoleWindowInfo absolute reset before write): Write=%v", ok5)

	// -------------------------------------------------------------------------
	// HYPOTHESIS 6: Cursor position placement before Write vs after Write
	// -------------------------------------------------------------------------
	procSetConsoleCursorPosition.Call(uintptr(hTarget), uintptr(uint32(0)|(uint32(11)<<16)))
	h6Cells := make([]charInfo, 0, int(cols)*2)
	h6Row0 := makeRow(" [H6 Cursor Set to (0,11) BEFORE Write] ", bgDarkBlue|fgWhite, int(cols), bgDarkBlue|fgWhite)
	h6Row1 := makeRow("package main // Cursor placed at row start prior to WriteConsoleOutputW", bgDarkBlue|fgLightCyan, int(cols), bgDarkBlue|fgLightGray)
	h6Cells = append(h6Cells, h6Row0...)
	h6Cells = append(h6Cells, h6Row1...)
	ok6 := writeGrid(hTarget, 0, 10, cols, 2, h6Cells)
	report.Log("Hypothesis 6 (Cursor position moved to start of line before write): Write=%v", ok6)

	// -------------------------------------------------------------------------
	// HYPOTHESIS 7: Write to STD_OUTPUT_HANDLE directly vs Dedicated Buffer
	// -------------------------------------------------------------------------
	h7Cells := make([]charInfo, 0, int(cols)*2)
	h7Row0 := makeRow(" [H7 Direct Write to STD_OUTPUT_HANDLE] ", bgDarkBlue|fgWhite, int(cols), bgDarkBlue|fgWhite)
	h7Row1 := makeRow("package main // Written directly to STD_OUTPUT_HANDLE", bgDarkBlue|fgLightCyan, int(cols), bgDarkBlue|fgLightGray)
	h7Cells = append(h7Cells, h7Row0...)
	h7Cells = append(h7Cells, h7Row1...)
	ok7 := writeGrid(hStdOut, 0, 12, cols, 2, h7Cells)
	report.Log("Hypothesis 7 (Direct Write to hStdOut vs hFarOut): Write=%v", ok7)

	// -------------------------------------------------------------------------
	// HYPOTHESIS 8: Multi-frame diffing emulation (Frame 1 Panel -> Frame 2 Editor)
	// -------------------------------------------------------------------------
	f1Cells := make([]charInfo, 0, int(cols)*2)
	f1Row0 := makeRow(" [Frame 1: Panel Top] ", bgDarkBlue|fgLightCyan, int(cols), bgDarkBlue|fgLightCyan)
	f1Row1 := makeRow(" File1.txt   File2.txt ", bgDarkBlue|fgYellow, int(cols), bgDarkBlue|fgLightGray)
	f1Cells = append(f1Cells, f1Row0...)
	f1Cells = append(f1Cells, f1Row1...)
	writeGrid(hTarget, 0, 14, cols, 2, f1Cells)

	f2Cells := make([]charInfo, 0, int(cols)*2)
	f2Row0 := makeRow(" [Frame 2: Editor Top] ", bgDarkBlue|fgWhite, int(cols), bgDarkBlue|fgWhite)
	f2Row1 := makeRow("package main // Overwritten frame 2", bgDarkBlue|fgLightCyan, int(cols), bgDarkBlue|fgLightGray)
	f2Cells = append(f2Cells, f2Row0...)
	f2Cells = append(f2Cells, f2Row1...)
	ok8 := writeGrid(hTarget, 0, 14, cols, 2, f2Cells)
	report.Log("Hypothesis 8 (Multi-frame consecutive overwrite on same cells): Write=%v", ok8)

	// -------------------------------------------------------------------------
	// HYPOTHESIS 9: Single 2-row write vs Two separate 1-row writes
	// -------------------------------------------------------------------------
	h9Row0 := makeRow(" [H9 Separate 1-row Write: Row 0] ", bgDarkBlue|fgWhite, int(cols), bgDarkBlue|fgWhite)
	h9Row1 := makeRow("package main // Separate 1-row Write: Row 1", bgDarkBlue|fgLightCyan, int(cols), bgDarkBlue|fgLightGray)
	ok9a := writeGrid(hTarget, 0, 16, cols, 1, h9Row0)
	ok9b := writeGrid(hTarget, 0, 17, cols, 1, h9Row1)
	report.Log("Hypothesis 9 (Two separate single-row WriteConsoleOutputW calls): Row0=%v Row1=%v", ok9a, ok9b)

	// -------------------------------------------------------------------------
	// HYPOTHESIS 10: Comparison of Themes (Classic Blue vs Radiola Dark Brown)
	// -------------------------------------------------------------------------
	h10Cells := make([]charInfo, 0, int(cols)*4)
	tARow0 := makeRow(" [Classic Blue Theme Status] ", bgDarkBlue|fgWhite, int(cols), bgDarkBlue|fgWhite)
	tARow1 := makeRow("package main // Classic Blue BG (0x10)", bgDarkBlue|fgLightCyan, int(cols), bgDarkBlue|fgLightGray)
	tBRow0 := makeRow(" [Radiola Theme Status] ", bgBrownDark|fgWhite, int(cols), bgBrownDark|fgWhite)
	tBRow1 := makeRow("package main // Radiola Dark BG (0x60)", bgBrownDark|fgYellow, int(cols), bgBrownDark|fgLightGray)

	h10Cells = append(h10Cells, tARow0...)
	h10Cells = append(h10Cells, tARow1...)
	h10Cells = append(h10Cells, tBRow0...)
	h10Cells = append(h10Cells, tBRow1...)
	ok10 := writeGrid(hTarget, 0, 18, cols, 4, h10Cells)
	report.Log("Hypothesis 10 (Comparison: Classic Blue 0x10 BG vs Radiola Brown 0x60 BG): Write=%v", ok10)

	report.Log("\n[+] Visual test grid rendered. Holding screen for 4 seconds...")
	time.Sleep(4 * time.Second)
	report.Log("[+] Probe completed successfully.")
}
