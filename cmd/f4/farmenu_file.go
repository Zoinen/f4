package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// FarMenu.ini text format, as written by far2l (usermenu.cpp:103):
//
//	HotKey:  Label\r\n
//	    Command1\r\n
//	    Command2\r\n
//
// For submenu items, the line right after the header is "{\r\n", and the
// nested level is terminated by "}\r\n". Separator items use HotKey "--"
// and an empty label.
//
// Encoding (read): we accept everything anyone might hand us — UTF-8
// with or without BOM, UTF-16 LE/BE with BOM, UTF-32 LE/BE with BOM.
// far2l on Linux/macOS/BSD writes UTF-32LE because its SIGN_WIDE_LE is
// emitted as one wchar_t and wchar_t is 4 bytes there; Far Manager 2
// on Windows writes UTF-16LE (2-byte wchar_t).
//
// Encoding (write): we mirror the dominant native tool on the running
// platform so the file we produce is consumable by it without any
// conversion. On Windows that's Far Manager 2 → UTF-16LE+BOM; on
// Linux/macOS/BSD that's far2l → UTF-32LE+BOM. Both with CRLF line
// endings, matching the upstream tools byte-for-byte.

const (
	farMenuColonSep      = ":  " // colon + two spaces between HotKey and Label
	farMenuCommandIndent = "    "
	farMenuLineEnding    = "\r\n"
)

// ParseFarMenu reads a FarMenu.ini text file and returns its items.
func ParseFarMenu(r io.Reader) ([]UserMenuItem, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text, err := decodeFarMenuBytes(raw)
	if err != nil {
		return nil, err
	}
	p := &farMenuParser{lines: splitTextLines(text)}
	items := p.parseLevel()
	if items == nil {
		items = []UserMenuItem{}
	}
	return items, nil
}

// WriteFarMenu writes items to w using the platform-native wide
// encoding (UTF-16LE+BOM on Windows, UTF-32LE+BOM elsewhere) so the
// resulting FarMenu.ini is consumable by the dominant tool on that
// platform without conversion.
func WriteFarMenu(w io.Writer, items []UserMenuItem) error {
	text := renderFarMenuText(items)
	_, err := w.Write(encodeFarMenuForPlatform(text))
	return err
}

// renderFarMenuText returns the canonical FarMenu.ini representation
// as a UTF-8 string with CRLF endings. WriteFarMenu wraps it with the
// platform wide encoding; the editor-temp-file path in user_menu_ui.go
// uses the raw UTF-8 bytes directly so the user gets a friendly file
// to hand-edit.
func renderFarMenuText(items []UserMenuItem) string {
	var buf bytes.Buffer
	writeFarMenuLevel(&buf, items)
	return buf.String()
}

// encodeFarMenuForPlatform applies the wchar_t-width-dependent encoding
// the running platform's native tool produces.
func encodeFarMenuForPlatform(s string) []byte {
	if runtime.GOOS == "windows" {
		return encodeUTF16LEWithBOM(s)
	}
	return encodeUTF32LEWithBOM(s)
}

func encodeUTF16LEWithBOM(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	buf := make([]byte, 0, 2+len(u16)*2)
	buf = append(buf, 0xFF, 0xFE)
	for _, w := range u16 {
		buf = binary.LittleEndian.AppendUint16(buf, w)
	}
	return buf
}

func encodeUTF32LEWithBOM(s string) []byte {
	runes := []rune(s)
	buf := make([]byte, 0, 4+len(runes)*4)
	buf = append(buf, 0xFF, 0xFE, 0x00, 0x00)
	for _, r := range runes {
		buf = binary.LittleEndian.AppendUint32(buf, uint32(r))
	}
	return buf
}

func decodeFarMenuBytes(b []byte) (string, error) {
	// Order matters: a UTF-32LE BOM (FF FE 00 00) starts with the same
	// two bytes as a UTF-16LE BOM (FF FE), so the wider check has to
	// come first. Same for the BE pair.
	switch {
	case len(b) >= 4 && b[0] == 0xFF && b[1] == 0xFE && b[2] == 0x00 && b[3] == 0x00:
		return decodeUTF32Bytes(b[4:], binary.LittleEndian), nil
	case len(b) >= 4 && b[0] == 0x00 && b[1] == 0x00 && b[2] == 0xFE && b[3] == 0xFF:
		return decodeUTF32Bytes(b[4:], binary.BigEndian), nil
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		return string(b[3:]), nil
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		return decodeUTF16Bytes(b[2:], binary.LittleEndian), nil
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		return decodeUTF16Bytes(b[2:], binary.BigEndian), nil
	}
	if !utf8.Valid(b) {
		return "", errors.New("FarMenu.ini: input is neither valid UTF-8 nor BOM-marked UTF-16/UTF-32")
	}
	return string(b), nil
}

func decodeUTF16Bytes(b []byte, bo binary.ByteOrder) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = bo.Uint16(b[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16))
}

func decodeUTF32Bytes(b []byte, bo binary.ByteOrder) string {
	b = b[:len(b)-len(b)%4]
	runes := make([]rune, 0, len(b)/4)
	for i := 0; i < len(b); i += 4 {
		value := bo.Uint32(b[i : i+4])
		if value > utf8.MaxRune || (value >= 0xD800 && value <= 0xDFFF) {
			runes = append(runes, utf8.RuneError)
			continue
		}
		// #nosec G115 -- value is bounded to a valid Unicode scalar above.
		r := rune(value)
		if !utf8.ValidRune(r) {
			r = utf8.RuneError
		}
		runes = append(runes, r)
	}
	return string(runes)
}

func splitTextLines(s string) []string {
	// Normalize CRLF and lone CR to LF, then split. Trailing empty string
	// from a final newline is fine; the parser skips blank lines.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

type farMenuParser struct {
	lines []string
	pos   int
}

// parseLevel parses items until it hits "}" or EOF. The opening "{" of
// a nested level (if any) must be consumed by the caller before calling.
func (p *farMenuParser) parseLevel() []UserMenuItem {
	var items []UserMenuItem
	for p.pos < len(p.lines) {
		line := strings.TrimRight(p.lines[p.pos], " \t")
		if line == "" {
			p.pos++
			continue
		}
		if line == "}" {
			p.pos++
			return items
		}
		if line == "{" {
			// Stray opening before any item — far2l ignores it.
			p.pos++
			continue
		}
		if isFarMenuSpaceByte(line[0]) {
			// Indented line with no item to attach to — drop it (matches
			// far2l, which would attach to the previous item's command
			// list at KeyNumber < 0 and silently no-op).
			p.pos++
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			p.pos++
			continue
		}
		item := UserMenuItem{
			HotKey: line[:colon],
			Label:  strings.TrimLeft(line[colon+1:], " \t"),
		}
		p.pos++
		if p.peekIsSubmenuOpen() {
			// Skip blank lines, then consume the "{".
			for p.pos < len(p.lines) {
				cur := strings.TrimRight(p.lines[p.pos], " \t")
				p.pos++
				if cur == "{" {
					break
				}
			}
			children := p.parseLevel()
			if children == nil {
				children = []UserMenuItem{}
			}
			item.Submenu = children
		} else {
			for p.pos < len(p.lines) {
				cl := strings.TrimRight(p.lines[p.pos], " \t")
				if cl == "" {
					p.pos++
					continue
				}
				if cl == "}" || !isFarMenuSpaceByte(cl[0]) {
					break
				}
				item.Commands = append(item.Commands, strings.TrimLeft(cl, " \t"))
				p.pos++
			}
		}
		items = append(items, item)
	}
	return items
}

func (p *farMenuParser) peekIsSubmenuOpen() bool {
	for i := p.pos; i < len(p.lines); i++ {
		line := strings.TrimRight(p.lines[i], " \t")
		if line == "" {
			continue
		}
		return line == "{"
	}
	return false
}

func isFarMenuSpaceByte(c byte) bool { return c == ' ' || c == '\t' }

func writeFarMenuLevel(buf *bytes.Buffer, items []UserMenuItem) {
	for i := range items {
		it := &items[i]
		buf.WriteString(it.HotKey)
		buf.WriteString(farMenuColonSep)
		buf.WriteString(it.Label)
		buf.WriteString(farMenuLineEnding)
		if it.IsSubmenu() {
			buf.WriteString("{")
			buf.WriteString(farMenuLineEnding)
			writeFarMenuLevel(buf, it.Submenu)
			buf.WriteString("}")
			buf.WriteString(farMenuLineEnding)
		} else {
			for _, cmd := range it.Commands {
				buf.WriteString(farMenuCommandIndent)
				buf.WriteString(cmd)
				buf.WriteString(farMenuLineEnding)
			}
		}
	}
}
