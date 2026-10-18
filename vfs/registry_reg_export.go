package vfs

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// The value types of the Windows registry, as numbered by winnt.h. They are
// repeated here so that the .reg encoder builds and is tested on every
// platform; the registry VFS itself exists only on Windows.
const (
	regTypeNone     uint32 = 0
	regTypeSZ       uint32 = 1
	regTypeExpandSZ uint32 = 2
	regTypeBinary   uint32 = 3
	regTypeDWORD    uint32 = 4
	regTypeMultiSZ  uint32 = 7
	regTypeQWORD    uint32 = 11
)

// regFileHeader is the first line of a Unicode .reg file, the format that
// regedit writes and imports (f4#242).
const regFileHeader = "Windows Registry Editor Version 5.00"

// regLineWidth is the width a hex(...) value is wrapped to, as regedit
// wraps it; regedit imports lines of any length.
const regLineWidth = 80

// regExportValue is one value of a key. An empty Name is the default value
// of the key, written as @.
type regExportValue struct {
	Name string
	Type uint32
	Data []byte
}

// regExportKey is one key section of a .reg file: its full path from the
// hive (HKEY_CURRENT_USER\Software\...) and its values in file order.
type regExportKey struct {
	Path   string
	Values []regExportValue
}

// encodeRegFile returns the keys as a Unicode .reg file: UTF-16LE with a
// byte order mark and CRLF line ends, the header, then every key in brackets
// with its values, each key followed by an empty line.
func encodeRegFile(keys []regExportKey) []byte {
	var text strings.Builder
	text.WriteString(regFileHeader)
	text.WriteString("\r\n\r\n")
	for _, key := range keys {
		text.WriteString("[")
		text.WriteString(key.Path)
		text.WriteString("]\r\n")
		for _, value := range key.Values {
			text.WriteString(regValueLine(value))
			text.WriteString("\r\n")
		}
		text.WriteString("\r\n")
	}
	words := utf16.Encode([]rune(text.String()))
	out := make([]byte, 2+2*len(words))
	out[0], out[1] = 0xFF, 0xFE
	for i, w := range words {
		binary.LittleEndian.PutUint16(out[2+2*i:], w)
	}
	return out
}

// regValueLine is the line of one value: "name"="text" for a string,
// "name"=dword:0000002a for a 32-bit number, and hex or hex(type) bytes for
// everything else, wrapped with a backslash the way regedit wraps them.
func regValueLine(value regExportValue) string {
	name := "@"
	if value.Name != "" {
		name = `"` + regQuote(value.Name) + `"`
	}
	switch value.Type {
	case regTypeSZ:
		if text, ok := regPlainString(value.Data); ok {
			return name + `="` + regQuote(text) + `"`
		}
	case regTypeDWORD:
		if len(value.Data) == 4 {
			return name + fmt.Sprintf("=dword:%08x", binary.LittleEndian.Uint32(value.Data))
		}
	}
	kind := "hex"
	if value.Type != regTypeBinary {
		kind = fmt.Sprintf("hex(%x)", value.Type)
	}
	return regHexLines(name+"="+kind+":", value.Data)
}

// regQuote escapes a name or a string value for the inside of quotes.
func regQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// regPlainString decodes REG_SZ data that can be written as "text": UTF-16LE
// that ends at its first terminating zero, with no line breaks in the text.
// Anything else keeps its exact bytes as hex(1).
func regPlainString(data []byte) (string, bool) {
	if len(data)%2 != 0 {
		return "", false
	}
	words := make([]uint16, len(data)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	for len(words) > 0 && words[len(words)-1] == 0 {
		words = words[:len(words)-1]
	}
	for _, w := range words {
		if w == 0 || w == '\r' || w == '\n' {
			return "", false
		}
	}
	text := string(utf16.Decode(words))
	if strings.ContainsRune(text, utf8.RuneError) {
		// An unpaired surrogate: only hex(1) keeps it.
		return "", false
	}
	return text, true
}

// regHexLines writes the bytes as comma separated hex pairs after the
// prefix. A line that would pass regLineWidth ends with ",\" and the next
// one starts with two spaces.
func regHexLines(prefix string, data []byte) string {
	var out strings.Builder
	out.WriteString(prefix)
	width := len(prefix)
	for i, b := range data {
		pair := fmt.Sprintf("%02x", b)
		if i < len(data)-1 {
			pair += ","
		}
		if width+len(pair) > regLineWidth-1 && i > 0 {
			out.WriteString("\\\r\n  ")
			width = 2
		}
		out.WriteString(pair)
		width += len(pair)
	}
	return out.String()
}
