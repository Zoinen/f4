package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestParseExpandsRLERows(t *testing.T) {
	first := uint64(0x0100 | 0x0200 | 0x112233<<16 | 0x445566<<40)
	second := uint64(0x0000000000000007)
	input := fmt.Sprintf("VTUI_SCREEN_DUMP_V1 4x2\n%s\nab  \nxy  \n%s\n%s\nR0: [%016X]x2 [%016X]x2\nR1: [%016X]x4\n", textHeader, metadataHeader, metadataFormat, first, second, second)

	dump, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if dump.Width != 4 || dump.Height != 2 || dump.Text[0] != "ab  " {
		t.Fatalf("decoded dimensions/text = %#v", dump)
	}
	if got := dump.Attrs[0]; got[0] != first || got[1] != first || got[2] != second || got[3] != second {
		t.Fatalf("decoded row = %#v", got)
	}
}

func TestParseRejectsRLEThatDoesNotFillRow(t *testing.T) {
	input := "VTUI_SCREEN_DUMP_V1 2x1\n" + textHeader + "\nxx\n" + metadataHeader + "\n" + metadataFormat + "\nR0: [0000000000000000]x1\n"
	if _, err := Parse(strings.NewReader(input)); err == nil {
		t.Fatal("Parse accepted an RLE row shorter than the screen width")
	}
}

func TestWritePPMUsesDecodedForegroundAndBackground(t *testing.T) {
	attr := uint64(0x0100 | 0x0200 | 0x112233<<16 | 0x445566<<40)
	dump := ScreenDump{Width: 1, Height: 1, Text: []string{"X"}, Attrs: [][]uint64{{attr}}}
	var image bytes.Buffer
	if err := WritePPM(&image, dump, 1, 1); err != nil {
		t.Fatalf("WritePPM: %v", err)
	}
	want := append([]byte("P6\n1 1\n255\n"), 0x11, 0x22, 0x33)
	if !bytes.Equal(image.Bytes(), want) {
		t.Fatalf("foreground PPM = %x, want %x", image.Bytes(), want)
	}

	dump.Text[0] = " "
	image.Reset()
	if err := WritePPM(&image, dump, 1, 1); err != nil {
		t.Fatalf("WritePPM background: %v", err)
	}
	want = append([]byte("P6\n1 1\n255\n"), 0x44, 0x55, 0x66)
	if !bytes.Equal(image.Bytes(), want) {
		t.Fatalf("background PPM = %x, want %x", image.Bytes(), want)
	}
}

func TestAttrColorsHonorsIndexedAndReverseAttributes(t *testing.T) {
	fg, bg := attrColors(uint64(9)<<16 | uint64(12)<<40)
	if fg != 0xFF0000 || bg != 0x0000FF {
		t.Fatalf("indexed colors = %#06x/%#06x", fg, bg)
	}
	fg, bg = attrColors(uint64(9)<<16 | uint64(12)<<40 | commonLvbReverse)
	if fg != 0x0000FF || bg != 0xFF0000 {
		t.Fatalf("reversed colors = %#06x/%#06x", fg, bg)
	}
	fg, _ = attrColors(uint64(0x112233)<<16 | isFgRGB | foregroundDim)
	if fg != 0x081119 {
		t.Fatalf("dimmed RGB foreground = %#06x, want %#06x", fg, uint32(0x081119))
	}
}
