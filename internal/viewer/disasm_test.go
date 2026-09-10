package viewer

import (
	"context"
	"github.com/unxed/f4/vfs"
	"os"
	"path/filepath"
	"testing"
)

// movRaxRcx is "mov rax, rcx" in 64-bit mode. The same three bytes are
// "dec eax; mov eax, ecx" in 32-bit mode and "dec ax; mov ax, cx" in
// 16-bit mode: the REX prefix is a one-byte instruction there. So a run of
// them is a line every 3 bytes in one mode and lines of 1 and 2 bytes by
// turns in the other two, which is what every navigation test below leans
// on.
var movRaxRcx = []byte{0x48, 0x89, 0xC8}

func TestNextDisasmMode(t *testing.T) {
	for _, tc := range []struct{ from, want int }{
		{64, 32},
		{32, 16},
		{16, 64},
		// Undecided and garbage restart the cycle rather than jam it.
		{0, 64},
		{48, 64},
	} {
		if got := NextDisasmMode(tc.from); got != tc.want {
			t.Errorf("NextDisasmMode(%d) = %d, want %d", tc.from, got, tc.want)
		}
	}
}

func TestDisasmInstruction_ModeDecidesTheReading(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		mode int
		text string
		len  int
	}{
		{"rex is a prefix in 64", movRaxRcx, 64, "mov rax, rcx", 3},
		{"rex is dec in 32", movRaxRcx, 32, "dec eax", 1},
		{"rex is dec in 16", movRaxRcx, 16, "dec ax", 1},
		{"imm16 in 16", []byte{0xB8, 0x34, 0x12, 0x00, 0x00}, 16, "mov ax, 0x1234", 3},
		{"imm32 in 32", []byte{0xB8, 0x34, 0x12, 0x00, 0x00}, 32, "mov eax, 0x1234", 5},
		{"push es exists in 16", []byte{0x06}, 16, "push es", 1},
		// An opcode the mode does not have is shown as a byte and skipped,
		// never swallowed with what follows it.
		{"push es is invalid in 64", []byte{0x06, 0x90}, 64, "db 0x06", 1},
	} {
		text, n := DisasmInstruction(tc.data, tc.mode, 0)
		if text != tc.text || n != tc.len {
			t.Errorf("%s: DisasmInstruction(% X, %d) = (%q, %d), want (%q, %d)",
				tc.name, tc.data, tc.mode, text, n, tc.text, tc.len)
		}
		if got := DisasmInstLen(tc.data, tc.mode); got != tc.len {
			t.Errorf("%s: DisasmInstLen = %d, want %d", tc.name, got, tc.len)
		}
	}
	if text, n := DisasmInstruction(nil, 64, 0); text != "" || n != 0 {
		t.Errorf("empty input decoded to (%q, %d)", text, n)
	}
}

func TestDetectX86Mode(t *testing.T) {
	pe := func(machine uint16) []byte {
		hdr := make([]byte, 0x80)
		copy(hdr, "MZ")
		hdr[0x3C] = 0x40 // e_lfanew
		copy(hdr[0x40:], "PE\x00\x00")
		// The two bytes of the PE machine field, little-endian; the
		// truncation is the encoding.
		hdr[0x44] = byte(machine & 0xFF)
		hdr[0x45] = byte(machine >> 8)
		return hdr
	}
	for _, tc := range []struct {
		name string
		data []byte
		want int
	}{
		{"ELF32", []byte("\x7fELF\x01\x01\x01\x00"), 32},
		{"ELF64", []byte("\x7fELF\x02\x01\x01\x00"), 64},
		{"PE i386", pe(0x014C), 32},
		{"PE amd64", pe(0x8664), 64},
		// A DOS executable has no PE header; nothing says 16 bits, so it
		// gets the default and the user switches from there.
		{"MZ only", append([]byte("MZ"), make([]byte, 0x40)...), 64},
		{"raw bytes", movRaxRcx, 64},
		{"nothing", nil, 64},
	} {
		if got := DetectX86Mode(tc.data); got != tc.want {
			t.Errorf("%s: DetectX86Mode = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestViewerView_OpenDecidesDisasmModeFromTheHeader: an ELF32 opens as
// 32-bit code straight away, and the switch to the editor carries the mode
// over instead of leaving the editor to detect it again from a buffer that
// may not have its first bytes yet.
func TestViewerView_OpenDecidesDisasmModeFromTheHeader(t *testing.T) {
	tmpDir := t.TempDir()
	tmp := filepath.Join(tmpDir, "elf32.bin")
	data := append([]byte("\x7fELF\x01\x01\x01\x00"), make([]byte, 64)...)
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		t.Fatal(err)
	}
	vv, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmpDir), tmp)
	if err != nil {
		t.Fatalf("NewViewerView: %v", err)
	}
	defer vv.Close()
	if vv.DisasmMode != 32 {
		t.Fatalf("ELF32 opened as %d-bit, want 32", vv.DisasmMode)
	}
}
