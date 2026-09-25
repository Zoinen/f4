package vtui

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeScreenDump(t *testing.T) {
	input := "VTUI_SCREEN_DUMP_V1 4x2\n" +
		"--- TEXT PREVIEW ---\n" +
		"AB  \n" +
		"Ж中  \n" +
		"--- CELL METADATA (RLE) ---\n" +
		"Format: [AttrHex]xRepeatCount ...\n" +
		"R0: [0000000000000001]x2 [0000000000000002]x2\n" +
		"R1: [0000000000000003]x4\n"

	got, err := DecodeScreenDump(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DecodeScreenDump() error = %v", err)
	}
	want := &ScreenDump{
		Width:  4,
		Height: 2,
		Rows: []ScreenDumpRow{
			{Text: "AB  ", Attributes: []uint64{1, 1, 2, 2}},
			{Text: "Ж中  ", Attributes: []uint64{3, 3, 3, 3}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeScreenDump() = %#v, want %#v", got, want)
	}
}

func TestDecodeScreenDumpRoundTrip(t *testing.T) {
	screen := NewSilentScreenBuf()
	screen.AllocBuf(3, 1)
	attr := uint64(0x1122334455667788)
	screen.Write(0, 0, []CharInfo{
		{Char: 'A', Attributes: attr},
		{Char: 'B', Attributes: attr},
		{Char: ' ', Attributes: 0},
	})

	var encoded bytes.Buffer
	screen.Dump(&encoded)
	got, err := DecodeScreenDump(&encoded)
	if err != nil {
		t.Fatalf("DecodeScreenDump(Dump()) error = %v", err)
	}
	if got.Width != 3 || got.Height != 1 {
		t.Fatalf("dimensions = %dx%d, want 3x1", got.Width, got.Height)
	}
	if got.Rows[0].Text != "AB " {
		t.Fatalf("text row = %q, want %q", got.Rows[0].Text, "AB ")
	}
	if want := []uint64{attr, attr, 0}; !reflect.DeepEqual(got.Rows[0].Attributes, want) {
		t.Fatalf("attributes = %#v, want %#v", got.Rows[0].Attributes, want)
	}
}

func TestDecodeScreenDumpRejectsInvalidRLE(t *testing.T) {
	input := "VTUI_SCREEN_DUMP_V1 2x1\n" +
		"--- TEXT PREVIEW ---\n" +
		"ab\n" +
		"--- CELL METADATA (RLE) ---\n" +
		"Format: [AttrHex]xRepeatCount ...\n" +
		"R0: [0000000000000001]x3\n"
	if _, err := DecodeScreenDump(strings.NewReader(input)); err == nil {
		t.Fatal("DecodeScreenDump() accepted RLE wider than the screen")
	}
}

func TestDecodeScreenDumpAllowsEmptyScreen(t *testing.T) {
	input := "VTUI_SCREEN_DUMP_V1 0x0\n" +
		"--- TEXT PREVIEW ---\n" +
		"--- CELL METADATA (RLE) ---\n" +
		"Format: [AttrHex]xRepeatCount ...\n"
	got, err := DecodeScreenDump(strings.NewReader(input))
	if err != nil {
		t.Fatalf("DecodeScreenDump() error = %v", err)
	}
	if got.Width != 0 || got.Height != 0 || len(got.Rows) != 0 {
		t.Fatalf("empty dump = %#v, want zero dimensions and rows", got)
	}
}
