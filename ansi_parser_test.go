package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"
)

func init() {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
}

// mockPty captures writes to the PTY for testing parser responses
type mockPty struct {
	mu      sync.Mutex
	written []byte
	closed  atomic.Bool
}

func (m *mockPty) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.written = append(m.written, b...)
	return len(b), nil
}

func (m *mockPty) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return string(m.written)
}

func (m *mockPty) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.written = nil
}
func (m *mockPty) Read(b []byte) (int, error) {
	for !m.closed.Load() {
		time.Sleep(10 * time.Millisecond)
	}
	return 0, io.EOF
}
func (m *mockPty) Close() error {
	m.closed.Store(true)
	return nil
}
func (m *mockPty) SetSize(cols, rows int)                {}
func (m *mockPty) Wait() error                           { return nil }
func (m *mockPty) Run(name string, args ...string) error { return nil }
func (m *mockPty) IsBusy() bool                          { return false }

func TestAnsiParser_CPR(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	pty := &mockPty{}
	p := NewAnsiParser(tv, pty)

	// 0-based coordinates in TerminalView: X=10, Y=5
	tv.SetCursor(10, 5)

	// Send Cursor Position Report (CPR) request
	p.Process([]byte("\x1b[6n"))

	// Expected response: 1-based coordinates \x1b[row;colR
	expected := "\x1b[6;11R"
	if string(pty.written) != expected {
		t.Errorf("Expected CPR response %q, got %q", expected, string(pty.written))
	}
}
func TestAnsiParser_SGR_Advanced(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// 1. Test TrueColor Foreground (38;2;R;G;B)
	p.Process([]byte("\x1b[38;2;255;128;64m"))
	expectedRGB := uint32(0xFF8040)
	if vtui.GetRGBFore(p.Attr) != expectedRGB {
		t.Errorf("TrueColor Fore: expected %06X, got %06X", expectedRGB, vtui.GetRGBFore(p.Attr))
	}
	if (p.Attr & vtui.IsFgRGB) == 0 {
		t.Error("TrueColor Fore: IsFgRGB flag not set")
	}

	// 2. Test 256-color Background (48;5;Index)
	p.Process([]byte("\x1b[48;5;208m"))
	if vtui.GetIndexBack(p.Attr) != 208 {
		t.Errorf("256-color Back: expected 208, got %d", vtui.GetIndexBack(p.Attr))
	}

	// 3. Test Styles: Bold (1) and Underline (4)
	p.Process([]byte("\x1b[1;4m"))
	if (p.Attr & vtui.ForegroundIntensity) == 0 {
		t.Error("Style: Bold flag not set")
	}
	if (p.Attr & vtui.CommonLvbUnderscore) == 0 {
		t.Error("Style: Underline flag not set")
	}

	// 4. Test Reset (0)
	p.Process([]byte("\x1b[0m"))
	if p.Attr != DefaultTermAttr {
		t.Errorf("Reset: expected %v, got %v", DefaultTermAttr, p.Attr)
	}
}
func TestAnsiParser_DynamicPalette(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// 1. Change Palette index 1 (ANSI Red) to Pure Purple #FF00FF
	// Format: OSC 4 ; index ; color BEL
	p.Process([]byte("\x1b]4;1;#FF00FF\x07"))

	// 2. Set foreground to ANSI 31 (Red)
	p.Process([]byte("\x1b[31m"))

	gotColor := tv.Palette[vtui.GetIndexFore(p.Attr)]
	if gotColor != 0xFF00FF {
		t.Errorf("Dynamic Palette: expected Purple #FF00FF, got %06X", gotColor)
	}

	// 3. Test rgb:RR/GG/BB format (used by some versions of far2l)
	// Change index 4 (ANSI Blue) to #112233
	p.Process([]byte("\x1b]4;4;rgb:11/22/33\x07"))
	p.Process([]byte("\x1b[34m")) // SGR 34 is ANSI Blue
	gotColor = tv.Palette[vtui.GetIndexFore(p.Attr)]
	if gotColor != 0x112233 {
		t.Errorf("Dynamic Palette (rgb format): expected #112233, got %06X", gotColor)
	}
}

func TestAnsiParser_SaveRestoreCursor_ESC(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.SetCursor(15, 8)

	// ESC 7 saves the cursor
	p.Process([]byte("\x1b7"))

	// Move away
	tv.SetCursor(0, 0)

	// ESC 8 restores the cursor
	p.Process([]byte("\x1b8"))

	if tv.CursorX != 15 || tv.CursorY != 8 {
		t.Errorf("Expected cursor at (15, 8) after restore, got (%d, %d)", tv.CursorX, tv.CursorY)
	}
}

func TestAnsiParser_SaveRestoreCursor_CSI(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.SetCursor(22, 11)

	// CSI s saves the cursor
	p.Process([]byte("\x1b[s"))

	// Move away
	tv.SetCursor(0, 0)

	// CSI u restores the cursor
	p.Process([]byte("\x1b[u"))

	if tv.CursorX != 22 || tv.CursorY != 11 {
		t.Errorf("Expected cursor at (22, 11) after restore, got (%d, %d)", tv.CursorX, tv.CursorY)
	}
}

func TestAnsiParser_StringTerminator(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// Trigger APC state (Application Program Command)
	p.Process([]byte("\x1b_"))
	if p.State != StateAPC {
		t.Fatalf("Expected state to be StateAPC, got %v", p.State)
	}

	// Send ESC \ (String Terminator)
	p.Process([]byte("\x1b\\"))

	// Parser should return to ground state
	if p.State != StateGround {
		t.Errorf("Expected state to return to StateGround after ST, got %v", p.State)
	}
}

func TestAnsiParser_DSR_Status(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	pty := &mockPty{}
	p := NewAnsiParser(tv, pty)

	// Request terminal status
	p.Process([]byte("\x1b[5n"))

	// Expected response: "Ready, no malfunction"
	expected := "\x1b[0n"
	if string(pty.written) != expected {
		t.Errorf("Expected DSR status response %q, got %q", expected, string(pty.written))
	}
}

func TestAnsiParser_OSC4_Palette(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// ANSI Color 1 — Red. By default in f4 palette it's 0xA00000.
	// Change it via OSC 4 to bright green #00FF00
	// Format: ESC ] 4 ; index ; color BEL
	oscSeq := "\x1b]4;1;#00FF00\x07"
	p.Process([]byte(oscSeq))

	if tv.Palette[1] != 0x00FF00 {
		t.Errorf("OSC 4 palette update failed. Expected #00FF00, got %06X", tv.Palette[1])
	}
}
func TestAnsiParser_REP_ECH(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// 1. Test REP (Repeat last char): write 'A' and repeat 5 times
	p.Process([]byte("A\x1b[5b"))
	line := tv.Lines[tv.CursorY]
	for i := 0; i < 6; i++ {
		if line[i].Char != 'A' {
			t.Errorf("REP failed at pos %d: expected 'A', got %c", i, rune(line[i].Char))
		}
	}

	// 2. Test ECH (Erase characters): erase 3 characters from position 0
	tv.SetCursor(0, tv.CursorY)
	p.Process([]byte("\x1b[3X"))
	for i := 0; i < 3; i++ {
		if line[i].Char != ' ' {
			t.Errorf("ECH failed at pos %d: expected space, got %c", i, rune(line[i].Char))
		}
	}
}
func TestAnsiParser_SplitUTF8(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// Symbol 'П' (0xD0 0x9F) sent in parts
	p.Process([]byte{0xD0})
	if tv.Lines[tv.CursorY][0].Char == 0xD0 {
		t.Error("Parser should not put incomplete UTF-8 byte on screen")
	}

	p.Process([]byte{0x9F})
	if tv.Lines[tv.CursorY][0].Char != 'П' {
		t.Errorf("Parser failed to assemble split UTF-8: expected 'П', got %c", rune(tv.Lines[tv.CursorY][0].Char))
	}
}
func TestAnsiParser_MovementAndErase(t *testing.T) {
	tv := NewTerminalView(10, 5)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// 1. Test CUP (H) - Cursor Position
	p.Process([]byte("\x1b[3;4H")) // 1-based, so should be 2,3
	if tv.CursorY != 2 || tv.CursorX != 3 {
		t.Errorf("CUP failed: expected (3,2), got (%d,%d)", tv.CursorX, tv.CursorY)
	}

	// 2. Test relative movements (A, B, C, D)
	p.Process([]byte("\x1b[2A")) // Up 2
	if tv.CursorY != 0 {
		t.Errorf("CUU failed: expected Y=0, got %d", tv.CursorY)
	}
	p.Process([]byte("\x1b[3B")) // Down 3
	if tv.CursorY != 3 {
		t.Errorf("CUD failed: expected Y=3, got %d", tv.CursorY)
	}
	p.Process([]byte("\x1b[5C")) // Forward 5
	if tv.CursorX != 8 {         // 3 + 5 = 8
		t.Errorf("CUF failed: expected X=8, got %d", tv.CursorX)
	}
	p.Process([]byte("\x1b[4D")) // Backward 4
	if tv.CursorX != 4 {         // 8 - 4 = 4
		t.Errorf("CUB failed: expected X=4, got %d", tv.CursorX)
	}

	// 3. Test ED (Erase Display) and EL (Erase Line)
	tv.PutChar('X', DefaultTermAttr)
	p.Process([]byte("\x1b[2J")) // Erase entire screen
	if tv.Lines[3][5].Char != ' ' {
		t.Error("ED(2) failed to clear screen")
	}
	tv.SetCursor(0, 0)

	// 4. Test Alternate Screen Buffer
	p.Process([]byte("Main"))
	p.Process([]byte("\x1b[?1049h")) // Switch to alt
	if !tv.UseAltScreen {
		t.Fatal("Failed to switch to alternate screen")
	}
	if tv.Lines[0][0].Char != 'M' {
		t.Error("Main screen content was affected by alt screen switch")
	}
	p.Process([]byte("Alt")) // Write to alt screen
	if tv.AltLines[0][0].Char != 'A' {
		t.Error("Failed to write to alt screen")
	}
	p.Process([]byte("\x1b[?1049l")) // Switch back to main
	if tv.UseAltScreen {
		t.Fatal("Failed to switch back to main screen")
	}
	if tv.Lines[0][0].Char != 'M' {
		t.Error("Main screen content was lost")
	}
}
func TestAnsiParser_Win32PasteModes(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// Enable modes
	p.Process([]byte("\x1b[?9001h\x1b[?2004h"))
	if !tv.Win32InputMode || !tv.BracketedPasteMode {
		t.Error("Failed to enable Win32InputMode or BracketedPasteMode")
	}

	// Disable modes
	p.Process([]byte("\x1b[?9001l\x1b[?2004l"))
	if tv.Win32InputMode || tv.BracketedPasteMode {
		t.Error("Failed to disable Win32InputMode or BracketedPasteMode")
	}
}

func TestAnsiParser_AdvancedCSI(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// Ensure we are at the top-left
	tv.SetCursor(0, 0)

	// Test Delete Characters (P)
	p.Process([]byte("12345"))   // Write at (0,0). Cursor moves to (5,0)
	tv.SetCursor(1, 0)           // Move to '2'
	p.Process([]byte("\x1b[2P")) // Delete 2 characters ('2' and '3')
	// Result should be "145" at index 0, 1, 2 of line 0
	if tv.Lines[0][1].Char != '4' || tv.Lines[0][2].Char != '5' {
		t.Errorf("Delete characters failed. Found %c (U+%04X) at [0][1]", rune(tv.Lines[0][1].Char), tv.Lines[0][1].Char)
	}

	// Test Insert Blank Characters (@)
	tv.SetCursor(1, 0)
	p.Process([]byte("\x1b[2@")) // Insert 2 blanks at pos 1
	// Result should be "1  45"
	if tv.Lines[0][1].Char != ' ' || tv.Lines[0][2].Char != ' ' || tv.Lines[0][3].Char != '4' {
		t.Errorf("Insert blank characters failed. Found %c at [0][3]", rune(tv.Lines[0][3].Char))
	}
}

func TestAnsiParser_OSC_Advanced(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// Test window title OSC 2
	p.Process([]byte("\x1b]2;far2l console\x07"))
	if tv.Title != "far2l console" {
		t.Errorf("Window title failed: expected 'far2l console', got '%s'", tv.Title)
	}
}
func TestAnsiParser_SGR_IntensityPersistence(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// 1. Set Bold (Intensity)
	p.Process([]byte("\x1b[1m"))
	if (p.Attr & vtui.ForegroundIntensity) == 0 {
		t.Fatal("Intensity flag not set")
	}

	// 2. Set "Bright Red" using 90-range code
	// HYPOTHESIS: This should either clear the manual Intensity flag OR we must
	// ensure that Flush doesn't produce double-brightening.
	p.Process([]byte("\x1b[91m"))

	if vtui.GetIndexFore(p.Attr) != 9 {
		t.Errorf("Expected index 9, got %d", vtui.GetIndexFore(p.Attr))
	}

	// If Intensity flag is still there, attributesToANSI will produce "\x1b[1;38;5;9m"
	// which is "Bold + Bright Red".
	if (p.Attr & vtui.ForegroundIntensity) != 0 {
		t.Log("Note: Intensity flag persists after 90-range SGR. Check if this causes 'dirty' colors on host.")
	}
}

func TestAnsiParser_DefaultColorRestoration(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// Set some non-default colors
	p.Process([]byte("\x1b[32;44m")) // Green on Blue

	// Restore default foreground (39)
	p.Process([]byte("\x1b[39m"))
	if vtui.GetIndexFore(p.Attr) != vtui.GetIndexFore(DefaultTermAttr) {
		t.Errorf("SGR 39 failed to restore default index. Expected %d, got %d",
			vtui.GetIndexFore(DefaultTermAttr), vtui.GetIndexFore(p.Attr))
	}

	// Check if background is still blue
	if vtui.GetIndexBack(p.Attr) != 4 {
		t.Errorf("SGR 39 corrupted background. Expected 4, got %d", vtui.GetIndexBack(p.Attr))
	}
}
func TestAnsiParser_Robustness(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// 1. Truncated CSI: should stay in StateCSI
	p.Process([]byte("\x1b["))
	if p.State != StateCSI {
		t.Errorf("Expected state StateCSI, got %v", p.State)
	}

	// 2. Garbage inside CSI: should return to ground without crashing
	p.Process([]byte("1;?#@")) // '@' is a valid terminator but parameters are junk
	if p.State != StateGround {
		t.Errorf("Expected return to StateGround after junk CSI, got %v", p.State)
	}

	// 3. Truncated OSC
	p.Process([]byte("\x1b]"))
	if p.State != StateOSC {
		t.Errorf("Expected state StateOSC, got %v", p.State)
	}

	// 4. OSC terminated by ESC instead of BEL
	p.Process([]byte("2;Title\x1b"))
	// The handleOSC is called, then StateEsc is entered
	if p.State != StateEsc {
		t.Errorf("Expected transition from OSC to ESC, got %v", p.State)
	}
	if tv.Title != "Title" {
		t.Error("OSC title failed with ESC terminator")
	}
}

func TestAnsiParser_OSC52_Malformed(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// 1. Malformed Base64 (should not panic or crash)
	// OSC 52 ; c ; <invalid_base64> BEL
	p.Process([]byte("\x1b]52;c;!!!\x07"))

	// 2. Incomplete OSC 52
	p.Process([]byte("\x1b]52;c;"))
	p.Process([]byte{0x07}) // Just BEL

	// 3. Very large OSC 52 (Buffer overflow protection check)
	largeSeq := "\x1b]52;c;" + strings.Repeat("A", 10000) + "\x07"
	p.Process([]byte(largeSeq))

	// If we are here without panic, the test is passed.
}

func TestAnsiParser_UnrecognizedCSI(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// CSI ? 999 z is unrecognized.
	// The parser must consume it and return to Ground state without side effects.
	p.Process([]byte("\x1b[?999z"))

	if p.State != StateGround {
		t.Errorf("Parser stuck in state %v after unrecognized CSI", p.State)
	}
}
func TestAnsiParser_APC_Reset(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)
	p.CurParam.WriteString("old_garbage")
	p.Process([]byte("\x1b_")) // Enter StateAPC
	if p.CurParam.Len() != 0 {
		t.Error("CurParam was not reset when entering APC state")
	}
}
func TestAnsiParser_DECRQM(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	pty := &mockPty{}
	p := NewAnsiParser(tv, pty)

	// --- 1. DEC Private Modes ---

	// Unknown Private Mode
	p.Process([]byte("\x1b[?7777$p"))
	if string(pty.written) != "\x1b[?7777;0$y" {
		t.Errorf("Expected not recognized (0) for ?7777, got %q", string(pty.written))
	}
	pty.written = nil

	// Mode 1: Application Cursor Keys
	tv.ApplicationCursorKeys = false
	p.Process([]byte("\x1b[?1$p"))
	if string(pty.written) != "\x1b[?1;2$y" {
		t.Errorf("Mode 1 Reset fail: %q", string(pty.written))
	}
	pty.written = nil
	tv.ApplicationCursorKeys = true
	p.Process([]byte("\x1b[?1$p"))
	if string(pty.written) != "\x1b[?1;1$y" {
		t.Errorf("Mode 1 Set fail: %q", string(pty.written))
	}
	pty.written = nil

	// Mode 47 & 1049: Alt Screen
	tv.UseAltScreen = false
	p.Process([]byte("\x1b[?47$p"))
	if string(pty.written) != "\x1b[?47;2$y" {
		t.Errorf("Mode 47 Reset fail")
	}
	pty.written = nil
	tv.UseAltScreen = true
	p.Process([]byte("\x1b[?1049$p"))
	if string(pty.written) != "\x1b[?1049;1$y" {
		t.Errorf("Mode 1049 Set fail")
	}
	pty.written = nil

	// Mode 2004: Bracketed Paste
	tv.BracketedPasteMode = false
	p.Process([]byte("\x1b[?2004$p"))
	if string(pty.written) != "\x1b[?2004;2$y" {
		t.Errorf("Mode 2004 Reset fail")
	}
	pty.written = nil
	tv.BracketedPasteMode = true
	p.Process([]byte("\x1b[?2004$p"))
	if string(pty.written) != "\x1b[?2004;1$y" {
		t.Errorf("Mode 2004 Set fail")
	}
	pty.written = nil

	// Mode 9001: Win32 Input
	tv.Win32InputMode = false
	p.Process([]byte("\x1b[?9001$p"))
	if string(pty.written) != "\x1b[?9001;2$y" {
		t.Errorf("Mode 9001 Reset fail")
	}
	pty.written = nil
	tv.Win32InputMode = true
	p.Process([]byte("\x1b[?9001$p"))
	if string(pty.written) != "\x1b[?9001;1$y" {
		t.Errorf("Mode 9001 Set fail")
	}
	pty.written = nil

	// --- 2. Standard Modes ---

	// Standard mode (always 0/not recognized in our current implementation)
	p.Process([]byte("\x1b[20$p"))
	if string(pty.written) != "\x1b[20;0$y" {
		t.Errorf("Expected not recognized for standard mode 20, got %q", string(pty.written))
	}
	pty.written = nil

	// --- 3. Edge Cases & Negative Tests ---

	// Wrong intermediate byte (e.g. # instead of $)
	p.Process([]byte("\x1b[?1#p"))
	if len(pty.written) > 0 {
		t.Error("Should not respond to DECRQM with wrong intermediate byte")
	}
	pty.written = nil

	// Missing parameters
	p.Process([]byte("\x1b[$p"))
	if len(pty.written) > 0 {
		t.Error("Should not respond to DECRQM without parameters")
	}
}
func TestAnsiParser_TechnicalCommandFilter(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.BracketedPasteMode = true // Изменим стейт, чтобы убедиться, что trailingANSI корректно отработает

	// Имитация технической команды, которую генерирует f4 для Unix (set +H...).
	// Обратите внимание: она содержит эхо самой команды и trailing ANSI.
	techCmd := "set +H; cd '/tmp' && { printf \"\\033]133;C\\007\"; ./script.sh ; printf \"\\033]133;D\\007\"; }\r\n"
	trailingANSI := "\x1b[?2004l" // Отключение bracketed paste и т.д.

	p.Process([]byte(techCmd + trailingANSI))

	// Парсер должен был перехватить и вырезать `techCmd`,
	// поэтому на экране терминала (Active Grid) не должно быть текста скрипта 's', 'e', 't'.
	if tv.Lines[tv.CursorY][0].Char == 's' {
		t.Error("Technical command was leaked to the visual screen!")
	}

	// Убеждаемся, что trailingANSI не был утерян вместе с вырезанной командой
	// и благополучно отработал, отключив режим bracketed paste.
	if tv.BracketedPasteMode {
		t.Error("Trailing ANSI sequence was ignored/cut off during technical command filtering!")
	}
}
func TestAnsiParser_WindowsAbsoluteJumpRobustness(t *testing.T) {
	// Типичный "грязный" чанк от ConPTY: очистка экрана + прыжок в середину + текст
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// \x1b[2J (Clear) \x1b[10;5H (Jump to row 10, col 5)
	chunk := "\x1b[2J\x1b[10;5HData"
	p.Process([]byte(chunk))

	// After writing 4 bytes "Data", X should be 4 + 4 = 8
	if tv.CursorY != 9 || tv.CursorX != 8 {
		t.Errorf("Absolute jump failed. Expected (8,9), got (%d,%d)", tv.CursorX, tv.CursorY)
	}

	if tv.Lines[9][4].Char != 'D' {
		t.Errorf("Data landed in wrong place. Expected 'D' at [9][4], got '%c'", tv.Lines[9][4].Char)
	}
}
func TestAnsiParser_WindowsExcision_CrossPlatform(t *testing.T) {
	// Этот тест проверяет логику вырезания технических команд Windows,
	// даже если тест запущен на Linux/macOS.
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple CD excision",
			input:    "cd /d \"C:\\Windows\" & dir\r\n",
			expected: "dir",
		},
		{
			name:     "Excision with prompt (screen scraping simulation)",
			input:    "C:\\Users\\f4>cd /d \"D:\\Data\" & echo 123\r\n",
			expected: "C:\\Users\\f4>echo 123",
		},
		{
			name:     "Multiple excisions in one buffer",
			input:    "Prompt1>cd /d \"A\" & cmd1\r\nPrompt2>cd /d \"B\" & cmd2",
			expected: "Prompt1>cmd1\nPrompt2>cmd2",
		},
		{
			name:     "Path with spaces and special chars",
			input:    "C:\\>cd /d \"C:\\My Folder & Stuff\" & whoami\r\n",
			expected: "C:\\>whoami",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tv.ResetBuffer(80, 24)
			tv.pt = piecetable.New([]byte{}) // Reset history
			tv.li.Rebuild(tv.pt)

			p.Process([]byte(tt.input))

			logBytes := tv.GetAllLogBytes()
			// Очищаем от лишних пробелов в конце строк сетки
			result := strings.TrimSpace(string(logBytes))

			if !strings.Contains(result, tt.expected) {
				t.Errorf("Excision failed for [%s].\nExpected to contain: %q\nGot log: %q", tt.name, tt.expected, result)
			}

			if strings.Contains(result, "cd /d") {
				t.Errorf("Excision failed for [%s]: technical 'cd' command leaked into log!", tt.name)
			}
		})
	}
}

func TestAnsiParser_ExcisionExtra(t *testing.T) {
	for _, tt := range []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Windows background sync excision",
			input:    "C:\\Old>cd /d \"C:\\New\" & rem f4_sync\r\n",
			expected: "",
		},
		{
			name:     "Windows technical command excision",
			input:    "C:\\Old>cd /d \"C:\\New\" & whoami\r\n",
			expected: "C:\\Old>whoami\n",
		},
		{
			name:     "Unix background sync excision",
			input:    "user@host:~$ cd '/new/path' # f4_sync\r\n",
			expected: "",
		},
		{
			name:     "macOS zsh background sync excision",
			input:    "user@host:~$ cd '/new/path'; : f4_sync\r\n",
			expected: "",
		},
		{
			name:     "Unix technical command excision",
			input:    "set +H; cd '/new/path' && { printf \"\\033]133;C\\007\"; ./'cmd' ; printf \"\\033]133;D\\007\"; }\r\n",
			expected: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tv := NewTerminalView(80, 24)
			parser := NewAnsiParser(tv, nil)
			parser.Process([]byte(tt.input))

			logBytes := tv.GetAllLogBytes()
			logStr := string(logBytes)

			// Normalize newlines for cross-platform comparison
			logStr = strings.ReplaceAll(logStr, "\r\n", "\n")

			if !strings.Contains(logStr, tt.expected) {
				t.Errorf("Expected log to contain %q, but got %q", tt.expected, logStr)
			}
		})
	}
}

func TestAnsiParser_ExcisesExpectedBackgroundSyncAcrossPTYReads(t *testing.T) {
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, nil)
	command := " cd '/Users/zoin/Documents/f4'; : f4_sync"
	parser.expectBackgroundSyncEcho(command)

	// This mirrors the fragmentation observed from an interactive zsh on a
	// Darwin PTY, including ZLE's leading backspace and bracketed-paste reset.
	for _, chunk := range [][]byte{
		[]byte("zoin@host f4 % "),
		[]byte(" \b cd '/U"),
		[]byte("sers/zoin/Doc"),
		[]byte("uments/f4'; : f4_sy"),
		[]byte("nc"),
		[]byte("\x1b[?2004l"),
		[]byte("\r\r\n"),
		[]byte("zoin@host f4 % \x1b[?2004h"),
	} {
		parser.Process(chunk)
	}

	logStr := string(tv.GetAllLogBytes())
	if strings.Contains(logStr, "cd '/Users/zoin/Documents/f4'") || strings.Contains(logStr, "f4_sync") {
		t.Fatalf("fragmented background sync echo reached scrollback: %q", logStr)
	}
	if !strings.Contains(logStr, "zoin@host f4 %") {
		t.Fatalf("shell prompt was lost while excising fragmented echo: %q", logStr)
	}
}

func TestAnsiParser_PrivateSyncSuppressesZLERedrawUntilProtocolMarker(t *testing.T) {
	tv := NewTerminalView(100, 24)
	parser := NewAnsiParser(tv, nil)
	parser.expectPrivateSyncCompletion()

	// Real interactive zsh output is not a literal command echo: ZLE inserts a
	// backspace, bracketed-paste toggles and arbitrary cursor redraws. Split the
	// private completion OSC too, proving suppression is protocol-delimited and
	// independent of PTY read boundaries.
	for _, chunk := range [][]byte{
		[]byte(" \b cd '/Users/zoin/Documents/f4'; printf '\\033]133;F4S"),
		[]byte(" \r\x1b[KY\rYNC\\007'\x1b[?2004l\r\r\n"),
		[]byte("\x1b]133;F4"),
		[]byte("SYNC\x07zoin@host f4 % \x1b[?2004h"),
	} {
		parser.Process(chunk)
	}

	got := string(tv.GetAllLogBytes())
	if strings.Contains(got, "cd '/Users") || strings.Contains(got, "F4SYNC") ||
		strings.Contains(got, "printf") {
		t.Fatalf("private cwd synchronization reached terminal history: %q", got)
	}
	if !strings.Contains(got, "zoin@host f4 %") {
		t.Fatalf("prompt after private cwd synchronization was lost: %q", got)
	}
	if parser.privateSyncPending != 0 || len(parser.privateSyncSuffix) != 0 {
		t.Fatalf("private sync did not settle: pending=%d suffix=%q",
			parser.privateSyncPending, parser.privateSyncSuffix)
	}
}

func TestAnsiParser_PrivateSyncFailsOpenForForegroundCommand(t *testing.T) {
	tv := NewTerminalView(80, 24)
	busy := false
	tv.OnBusyChange = func(value bool) { busy = value }
	parser := NewAnsiParser(tv, nil)
	parser.expectPrivateSyncCompletion()

	parser.Process([]byte("\x1b]133;C\x07user output\r\n"))
	if !busy {
		t.Fatal("stale private sync suppression swallowed foreground OSC C")
	}
	if got := string(tv.GetAllLogBytes()); !strings.Contains(got, "user output") {
		t.Fatalf("stale private sync suppression swallowed foreground output: %q", got)
	}
	if parser.privateSyncPending != 0 {
		t.Fatalf("foreground command left private sync pending=%d", parser.privateSyncPending)
	}
}

func TestAnsiParser_PrivateSyncFailsOpenForEveryForegroundOSCSplit(t *testing.T) {
	start := []byte(managedCommandStartOSC)
	for split := 0; split <= len(start); split++ {
		t.Run(fmt.Sprintf("split-%d", split), func(t *testing.T) {
			tv := NewTerminalView(80, 24)
			var busyChanges []bool
			tv.OnBusyChange = func(value bool) {
				busyChanges = append(busyChanges, value)
			}
			tv.PrepareCleanCommand("printf user-output")

			parser := NewAnsiParser(tv, nil)
			parser.expectPrivateSyncCompletion()

			// The bytes before the foreground marker are a stale private ZLE
			// redraw. They must stay suppressed, while every byte from OSC C
			// onward must survive regardless of the PTY read boundary.
			parser.Process(append([]byte("private redraw"), start[:split]...))
			second := append([]byte{}, start[split:]...)
			second = append(second, '\x07')
			second = append(second, []byte("user-output\r\n\x1b]133;D\x07")...)
			parser.Process(second)

			if len(busyChanges) != 2 || !busyChanges[0] || busyChanges[1] {
				t.Fatalf("OSC C/D lifecycle = %v, want [true false]", busyChanges)
			}
			got := string(tv.GetAllLogBytes())
			if !strings.Contains(got, "printf user-output") ||
				!strings.Contains(got, "user-output") {
				t.Fatalf("foreground command bytes lost at split %d: %q", split, got)
			}
			if strings.Contains(got, "private redraw") {
				t.Fatalf("private redraw leaked at split %d: %q", split, got)
			}
			if parser.privateSyncPending != 0 || len(parser.privateSyncSuffix) != 0 {
				t.Fatalf("foreground command left private sync state at split %d: pending=%d suffix=%q",
					split, parser.privateSyncPending, parser.privateSyncSuffix)
			}
		})
	}
}

func TestAnsiParser_QueuesRapidBackgroundSyncEchoes(t *testing.T) {
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, nil)
	first := " cd '/tmp/left'; : f4_sync"
	second := " cd '/tmp/right'; : f4_sync"
	parser.expectBackgroundSyncEcho(first)
	parser.expectBackgroundSyncEcho(second)

	parser.Process([]byte("host% " + first[:12]))
	parser.Process([]byte(first[12:] + "\r\r\nhost% " + second[:10]))
	parser.Process([]byte(second[10:] + "\x1b[?2004l\r\r\nhost% "))

	logStr := string(tv.GetAllLogBytes())
	if strings.Contains(logStr, "f4_sync") || strings.Contains(logStr, "cd '/tmp/") {
		t.Fatalf("queued background sync echo reached scrollback: %q", logStr)
	}
}

func TestAnsiParser_ExpectedBackgroundSyncDoesNotWithholdOrdinaryOutput(t *testing.T) {
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, nil)
	parser.expectBackgroundSyncEcho(" cd '/tmp/expected'; : f4_sync")

	// No newline is needed for fail-open streaming: only a suffix that could
	// still become the expected command may be retained between PTY reads.
	parser.Process([]byte("ordinary stdout"))
	if got := string(tv.GetAllLogBytes()); !strings.Contains(got, "ordinary stdout") {
		t.Fatalf("ordinary stdout was withheld behind a sync expectation: %q", got)
	}
}

func TestAnsiParser_ExpectedBackgroundSyncFailsOpenOnMismatchedLine(t *testing.T) {
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, nil)
	parser.expectBackgroundSyncEcho(" cd '/tmp/expected'; : f4_sync")

	parser.Process([]byte("rewritten sync echo\r\n"))
	parser.Process([]byte("ls-result\r\n"))

	got := string(tv.GetAllLogBytes())
	if !strings.Contains(got, "rewritten sync echo") || !strings.Contains(got, "ls-result") {
		t.Fatalf("mismatched sync echo swallowed terminal output: %q", got)
	}
	if len(parser.backgroundSyncExpected) != 0 || len(parser.backgroundSyncBuffer) != 0 {
		t.Fatalf("mismatched line left a stale sync expectation: expected=%d buffered=%q",
			len(parser.backgroundSyncExpected), parser.backgroundSyncBuffer)
	}
}

func TestAnsiParser_ExpectedBackgroundSyncReleasesSplitPrefixAfterRewrite(t *testing.T) {
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, nil)
	command := " cd '/tmp/expected'; : f4_sync"
	parser.expectBackgroundSyncEcho(command)

	// First retain a genuine prefix, then model a shell redraw inserting an
	// ANSI sequence into the echoed command. The now-impossible candidate and
	// the stdout following it must both be released at the line boundary.
	parser.Process([]byte(command[:12]))
	parser.Process([]byte("\x1b[0m" + command[12:] + "\r\nls-result\r\n"))

	got := string(tv.GetAllLogBytes())
	if !strings.Contains(got, "ls-result") {
		t.Fatalf("rewritten split echo swallowed subsequent stdout: %q", got)
	}
	if len(parser.backgroundSyncExpected) != 0 || len(parser.backgroundSyncBuffer) != 0 {
		t.Fatalf("rewritten split echo left a stale sync expectation: expected=%d buffered=%q",
			len(parser.backgroundSyncExpected), parser.backgroundSyncBuffer)
	}
}

func TestAnsiParser_ExpectedBackgroundSyncFailsOpenForForegroundOSC(t *testing.T) {
	tv := NewTerminalView(80, 24)
	busy := false
	tv.OnBusyChange = func(value bool) { busy = value }
	parser := NewAnsiParser(tv, nil)
	parser.expectBackgroundSyncEcho(" cd '/tmp/expected'; : f4_sync")

	// This is the first output when echo is disabled and a managed foreground
	// command starts. It must reach HandleOSC133 immediately.
	parser.Process([]byte("\x1b]133;C\x07"))
	if !busy {
		t.Fatal("foreground OSC was withheld behind a stale sync expectation")
	}
	if len(parser.backgroundSyncExpected) != 0 || len(parser.backgroundSyncBuffer) != 0 {
		t.Fatalf("foreground OSC left a stale sync expectation: expected=%d buffered=%q",
			len(parser.backgroundSyncExpected), parser.backgroundSyncBuffer)
	}
}

func TestAnsiParser_ExpectedBackgroundSyncExactEchoWithoutNewlineFailsOpenForOSC(t *testing.T) {
	tv := NewTerminalView(80, 24)
	busy := false
	tv.OnBusyChange = func(value bool) { busy = value }
	parser := NewAnsiParser(tv, nil)
	command := " cd '/tmp/expected'; : f4_sync"
	parser.expectBackgroundSyncEcho(command)

	parser.Process([]byte(command + "\x1b]133;C\x07"))
	if !busy {
		t.Fatal("foreground OSC after unterminated exact echo was withheld")
	}
	if len(parser.backgroundSyncExpected) != 0 || len(parser.backgroundSyncBuffer) != 0 {
		t.Fatalf("unterminated exact echo left a stale sync expectation: expected=%d buffered=%q",
			len(parser.backgroundSyncExpected), parser.backgroundSyncBuffer)
	}
}

func TestAnsiParser_ExpectedBackgroundSyncStreamsPastFalsePrefix(t *testing.T) {
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, nil)
	parser.expectBackgroundSyncEcho(" cd '/tmp/expected'; : f4_sync")

	// The trailing space is a one-byte prefix of the registered command. It is
	// legitimate to retain that byte, but the preceding stdout must be visible.
	parser.Process([]byte("first chunk "))
	if got := string(tv.GetAllLogBytes()); !strings.Contains(got, "first chunk") {
		t.Fatalf("stdout preceding a possible echo prefix was withheld: %q", got)
	}
	parser.Process([]byte("X second chunk"))
	if got := string(tv.GetAllLogBytes()); !strings.Contains(got, "X second chunk") {
		t.Fatalf("false echo prefix did not fail open after mismatch: %q", got)
	}
}

func TestAnsiParser_ExpectedBackgroundSyncKeepsPrefixAfterCompletedOutput(t *testing.T) {
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, nil)
	command := " cd '/tmp/expected'; : f4_sync"
	parser.expectBackgroundSyncEcho(command)

	// A single PTY read can contain output that was already in flight followed
	// by only the first fragment of the private echo. The old line must render
	// immediately, while the candidate suffix remains available for matching.
	parser.Process([]byte("old output\r\nhost% " + command[:13]))
	if got := string(tv.GetAllLogBytes()); !strings.Contains(got, "old output") {
		t.Fatalf("completed output was withheld behind a split echo: %q", got)
	}
	parser.Process([]byte(command[13:] + "\x1b[?2004l\r\r\nnext prompt% "))

	got := string(tv.GetAllLogBytes())
	if strings.Contains(got, "cd '/tmp/expected'") || strings.Contains(got, "f4_sync") {
		t.Fatalf("split sync echo following completed output leaked: %q", got)
	}
	if !strings.Contains(got, "next prompt") {
		t.Fatalf("output following split sync echo was lost: %q", got)
	}
}

type mockClipAuthManager struct {
	authorized bool
}

func (m *mockClipAuthManager) Authorize(id string) int {
	if m == nil {
		return 0
	}
	if m.authorized {
		return 1 // Allow Once
	}
	return 0 // Deny
}

func TestAnsiParser_OSC52_Read_Security(t *testing.T) {
	tv := NewTerminalView(80, 24)
	pty := &mockPtyForTerminal{}
	parser := NewAnsiParser(tv, pty)

	// Test 1: Denied access
	vtui.GlobalClipboardAccessManager = &mockClipAuthManager{authorized: false}
	parser.Process([]byte("\x1b]52;c;?\x07"))

	if pty.Len() > 0 {
		t.Errorf("Expected no output when clipboard read is denied, got %q", pty.String())
	}

	// Test 2: Allowed access
	vtui.GlobalClipboardAccessManager = &mockClipAuthManager{authorized: true}
	vtui.SetClipboard("secret_data")
	for i := 0; i < 50; i++ {
		if vtui.GetClipboard() == "secret_data" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	parser.Process([]byte("\x1b]52;c;?\x07"))

	var out string
	for start := time.Now(); time.Since(start) < 2*time.Second; {
		out = pty.String()
		if strings.Contains(out, "\x1b]52;c;") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(out, "\x1b]52;c;") {
		t.Errorf("Expected OSC 52 reply containing clipboard data, got %q", out)
	}

	// Reset global state
	vtui.GlobalClipboardAccessManager = nil
}

func TestAnsiParser_OSC52_Write_Success(t *testing.T) {
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, nil)

	testStr := "Hello OSC 52"
	b64 := base64.StdEncoding.EncodeToString([]byte(testStr))

	// OSC 52 ; selection (c) ; data (b64) BEL
	parser.Process([]byte(fmt.Sprintf("\x1b]52;c;%s\x07", b64)))

	for i := 0; i < 50; i++ {
		if vtui.GetClipboard() == testStr {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	got := vtui.GetClipboard()
	if got != testStr {
		t.Errorf("Expected clipboard to be %q, got %q", testStr, got)
	}
}
func TestAnsiParser_Excision_UTF8_Safety(t *testing.T) {
	tv := NewTerminalView(80, 24)
	p := NewAnsiParser(tv, nil)

	// Simulation: Windows sync command followed by the first byte of 'П' (0xD0)
	// In the old implementation string(data) would have corrupted 0xD0 if it was at the end of a chunk.
	syncCmd := []byte("cd /d \"C:\\\" & rem f4_sync\r\n\r\n")
	partialUTF8 := []byte{0xD0}

	p.Process(append(syncCmd, partialUTF8...))

	// Sync command must be excised
	logStr := string(tv.GetAllLogBytes())
	if strings.Contains(logStr, "f4_sync") {
		t.Error("Sync command was not correctly excised from byte buffer")
	}

	// Partial UTF-8 must be preserved in the parser's internal buffer
	if len(p.runeBuf) != 1 || p.runeBuf[0] != 0xD0 {
		t.Errorf("Partial UTF-8 byte was lost or corrupted during excision. Buffer: %v", p.runeBuf)
	}

	// Completing the sequence: sending 0x9F (second byte of 'П')
	p.Process([]byte{0x9F})
	if tv.Lines[tv.CursorY][0].Char != 'П' {
		t.Errorf("UTF-8 sequence was not correctly assembled after excision. Got: %c", rune(tv.Lines[tv.CursorY][0].Char))
	}
}

func TestAnsiParser_DECSET_Modes(t *testing.T) {
	tv := NewTerminalView(80, 24)
	p := NewAnsiParser(tv, nil)

	// 1. AutoWrap (CSI ? 7 h/l)
	tv.AutoWrap = true
	p.Process([]byte("\x1b[?7l"))
	if tv.AutoWrap {
		t.Error("CSI ? 7 l failed to disable AutoWrap")
	}
	p.Process([]byte("\x1b[?7h"))
	if !tv.AutoWrap {
		t.Error("CSI ? 7 h failed to enable AutoWrap")
	}

	// 2. Mouse Tracking (1000, 1002, 1003)
	p.Process([]byte("\x1b[?1000h"))
	if tv.MouseTrackingMode != 1000 {
		t.Errorf("Expected MouseTrackingMode 1000, got %d", tv.MouseTrackingMode)
	}
	p.Process([]byte("\x1b[?1002h"))
	if tv.MouseTrackingMode != 1002 {
		t.Errorf("Expected MouseTrackingMode 1002, got %d", tv.MouseTrackingMode)
	}
	p.Process([]byte("\x1b[?1000l"))
	if tv.MouseTrackingMode != 0 {
		t.Error("Mouse tracking was not disabled by CSI ? 1000 l")
	}

	// 3. Mouse SGR Mode (1006)
	tv.MouseSGRMode = false
	p.Process([]byte("\x1b[?1006h"))
	if !tv.MouseSGRMode {
		t.Error("CSI ? 1006 h failed to enable MouseSGRMode")
	}
}
