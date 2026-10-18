package vfs

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func regUTF16(s string) []byte {
	words := utf16.Encode([]rune(s))
	out := make([]byte, 2*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint16(out[2*i:], w)
	}
	return out
}

func regDecodeFile(t *testing.T, data []byte) string {
	t.Helper()
	if !bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
		t.Fatalf("no UTF-16LE byte order mark: % x", data[:min(len(data), 4)])
	}
	if len(data)%2 != 0 {
		t.Fatalf("odd length %d", len(data))
	}
	words := make([]uint16, (len(data)-2)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(data[2+2*i:])
	}
	return string(utf16.Decode(words))
}

func TestEncodeRegFileIsUnicodeRegedit_Issue242(t *testing.T) {
	got := regDecodeFile(t, encodeRegFile([]regExportKey{
		{Path: `HKEY_CURRENT_USER\Software\f4`, Values: []regExportValue{
			{Name: "", Type: regTypeSZ, Data: regUTF16("Привет\x00")},
			{Name: `Path "x"`, Type: regTypeSZ, Data: regUTF16(`C:\Temp` + "\x00")},
			{Name: "Count", Type: regTypeDWORD, Data: []byte{0x2a, 0, 0, 0}},
		}},
		{Path: `HKEY_CURRENT_USER\Software\f4\Empty`},
	}))
	want := "Windows Registry Editor Version 5.00\r\n\r\n" +
		"[HKEY_CURRENT_USER\\Software\\f4]\r\n" +
		"@=\"Привет\"\r\n" +
		"\"Path \\\"x\\\"\"=\"C:\\\\Temp\"\r\n" +
		"\"Count\"=dword:0000002a\r\n" +
		"\r\n" +
		"[HKEY_CURRENT_USER\\Software\\f4\\Empty]\r\n" +
		"\r\n"
	if got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}

func TestRegValueLineTypesInHex_Issue242(t *testing.T) {
	cases := []struct {
		value regExportValue
		want  string
	}{
		{regExportValue{Name: "Bin", Type: regTypeBinary, Data: []byte{0, 0xab, 0x10}}, `"Bin"=hex:00,ab,10`},
		{regExportValue{Name: "Exp", Type: regTypeExpandSZ, Data: regUTF16("%A%\x00")}, `"Exp"=hex(2):25,00,41,00,25,00,00,00`},
		{regExportValue{Name: "Multi", Type: regTypeMultiSZ, Data: regUTF16("a\x00b\x00\x00")}, `"Multi"=hex(7):61,00,00,00,62,00,00,00,00,00`},
		{regExportValue{Name: "Q", Type: regTypeQWORD, Data: []byte{1, 0, 0, 0, 0, 0, 0, 0}}, `"Q"=hex(b):01,00,00,00,00,00,00,00`},
		{regExportValue{Name: "None", Type: regTypeNone}, `"None"=hex(0):`},
		{regExportValue{Name: "Short", Type: regTypeDWORD, Data: []byte{1, 2}}, `"Short"=hex(4):01,02`},
		{regExportValue{Name: "Lines", Type: regTypeSZ, Data: regUTF16("a\r\nb\x00")}, `"Lines"=hex(1):61,00,0d,00,0a,00,62,00,00,00`},
		{regExportValue{Name: "Odd", Type: regTypeSZ, Data: []byte{0x61}}, `"Odd"=hex(1):61`},
		{regExportValue{Name: "Empty", Type: regTypeSZ}, `"Empty"=""`},
	}
	for _, c := range cases {
		if got := regValueLine(c.value); got != c.want {
			t.Errorf("%s: got %q, want %q", c.value.Name, got, c.want)
		}
	}
}

func TestRegValueLineWrapsLongHex_Issue242(t *testing.T) {
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}
	got := regValueLine(regExportValue{Name: "Big", Type: regTypeBinary, Data: data})
	lines := strings.Split(got, "\r\n")
	if len(lines) < 2 {
		t.Fatalf("not wrapped: %q", got)
	}
	var joined strings.Builder
	for i, line := range lines {
		if len(line) > regLineWidth {
			t.Errorf("line %d is %d characters: %q", i, len(line), line)
		}
		if i < len(lines)-1 && !strings.HasSuffix(line, `,\`) {
			t.Errorf("line %d does not end with a continuation: %q", i, line)
		}
		if i > 0 && !strings.HasPrefix(line, "  ") {
			t.Errorf("line %d is not indented: %q", i, line)
		}
		joined.WriteString(strings.TrimSuffix(strings.TrimPrefix(line, "  "), `\`))
	}
	want := regHexLines(`"Big"=hex:`, nil)
	for i, b := range data {
		if i > 0 {
			want += ","
		}
		want += string("0123456789abcdef"[b>>4]) + string("0123456789abcdef"[b&15])
	}
	if joined.String() != want {
		t.Fatalf("the wrapped bytes differ:\n%q\n%q", joined.String(), want)
	}
}
