package semantic

import (
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf8"
)

type testBaseNamer struct{}

func (testBaseNamer) Base(path string) string {
	return "custom:" + path
}

func TestString(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"string", "value", "value"},
		{"non-string", 42, ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := String(tt.input); got != tt.want {
				t.Errorf("String(%#v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInt(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  int
	}{
		{"int", int(-1), -1},
		{"int8", int8(-2), -2},
		{"int16", int16(-3), -3},
		{"int32", int32(-4), -4},
		{"int64", int64(-5), -5},
		{"uint", uint(6), 6},
		{"uint8", uint8(7), 7},
		{"uint16", uint16(8), 8},
		{"uint32", uint32(9), 9},
		{"uint64", uint64(10), 10},
		{"float32", float32(11.5), 11},
		{"float64", float64(12.5), 12},
		{"unsupported", "13", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Int(tt.input); got != tt.want {
				t.Errorf("Int(%#v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestBaseName(t *testing.T) {
	if got := BaseName(nil, "dir/file.txt"); got != filepath.Base("dir/file.txt") {
		t.Errorf("BaseName(nil) = %q, want %q", got, filepath.Base("dir/file.txt"))
	}
	if got := BaseName(testBaseNamer{}, "dir/file.txt"); got != "custom:dir/file.txt" {
		t.Errorf("BaseName(custom) = %q", got)
	}
	if got := BaseName(nil, ""); got != "" {
		t.Errorf("BaseName(empty) = %q, want empty", got)
	}
}

func TestBool(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  bool
	}{
		{"true", true, true},
		{"false", false, false},
		{"non-zero int", 1, true},
		{"zero int", 0, false},
		{"non-zero float", float64(1), true},
		{"zero float", float64(0), false},
		{"unsupported", "true", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Bool(tt.input); got != tt.want {
				t.Errorf("Bool(%#v) = %t, want %t", tt.input, got, tt.want)
			}
		})
	}
}

func TestRunsFromCells(t *testing.T) {
	if got := RunsFromCells(nil); got != nil {
		t.Fatalf("RunsFromCells(nil) = %#v, want nil", got)
	}

	cells := []vtui.CharInfo{
		{Char: 'a', Attributes: 1},
		{Char: 'b', Attributes: 1},
		{Char: vtui.WideCharFiller, Attributes: 1},
		{Char: 'c', Attributes: 2},
		{Char: 0, Attributes: 2},
	}
	want := []extui.RunModel{
		{Text: "ab ", Attr: 1, Foreground: "#000000", Background: "#000000"},
		{Text: "c ", Attr: 2, Foreground: "#000000", Background: "#000000"},
	}
	if got := RunsFromCells(cells); !reflect.DeepEqual(got, want) {
		t.Fatalf("RunsFromCells() = %#v, want %#v", got, want)
	}
}

func TestCellRune(t *testing.T) {
	tests := []struct {
		name string
		char uint64
		want rune
	}{
		{"zero", 0, ' '},
		{"valid", 'Ж', 'Ж'},
		{"above unicode", utf8.MaxRune + 1, ' '},
		{"surrogate", 0xD800, ' '},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CellRune(tt.char); got != tt.want {
				t.Errorf("CellRune(%#x) = %q, want %q", tt.char, got, tt.want)
			}
		})
	}
}
