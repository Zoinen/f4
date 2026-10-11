//go:build !extralite

package macro

import (
	"strconv"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// The rest of Far's mf.* helpers that are plain functions of their arguments
// (unxed/f4#1686, Step 7): number/string conversion, remainder, trimming by
// mode, and the date. Behaviour follows far2l's macro functions.

// macroItoa is mf.itoa(n[, radix]): n written in the radix (10 by default, 2..36).
func macroItoa(L *lua.LState) int {
	radix := 10
	if L.GetTop() >= 2 {
		if r := L.CheckInt(2); r != 0 {
			radix = r
		}
	}
	if radix < 2 || radix > 36 {
		L.ArgError(2, "radix must be between 2 and 36")
		return 0
	}
	L.Push(lua.LString(strconv.FormatInt(int64(L.CheckNumber(1)), radix)))
	return 1
}

// macroAtoi is mf.atoi(s[, radix]): the integer at the start of s, after
// leading blanks and an optional sign; 0 when there is none. A radix of 0 or
// none picks the base from a 0x or 0 prefix, as C's strtol does.
func macroAtoi(L *lua.LState) int {
	text := strings.TrimLeft(L.CheckString(1), " \t\r\n")
	radix := 0
	if L.GetTop() >= 2 {
		radix = L.CheckInt(2)
	}
	if radix < 0 || radix == 1 || radix > 36 {
		L.ArgError(2, "radix must be 0 or between 2 and 36")
		return 0
	}
	L.Push(lua.LNumber(parseIntPrefix(text, radix)))
	return 1
}

// parseIntPrefix reads the longest integer prefix of s in the radix.
func parseIntPrefix(s string, radix int) int64 {
	negative := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		negative = s[0] == '-'
		s = s[1:]
	}
	if radix == 0 || radix == 16 {
		if len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
			s, radix = s[2:], 16
		}
	}
	if radix == 0 {
		radix = 10
		if len(s) > 1 && s[0] == '0' {
			radix = 8
		}
	}
	end := 0
	for end < len(s) && digitValue(s[end]) < radix {
		end++
	}
	if end == 0 {
		return 0
	}
	value, err := strconv.ParseInt(s[:end], radix, 64)
	if err != nil {
		if negative {
			return -1 << 63
		}
		return 1<<63 - 1
	}
	if negative {
		return -value
	}
	return value
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 99
}

// macroMod is mf.mod(a, b): the remainder of the integer division, with the
// sign of a; 0 when b is 0.
func macroMod(L *lua.LState) int {
	a, b := int64(L.CheckNumber(1)), int64(L.CheckNumber(2))
	if b == 0 {
		L.Push(lua.LNumber(0))
		return 1
	}
	L.Push(lua.LNumber(a % b))
	return 1
}

// macroTrim is mf.trim(s[, mode]): mode 0 (default) strips both ends, 1 the
// left, 2 the right; any other mode returns s unchanged.
func macroTrim(L *lua.LState) int {
	text := L.CheckString(1)
	mode := 0
	if L.GetTop() >= 2 {
		mode = L.CheckInt(2)
	}
	switch mode {
	case 0:
		text = strings.TrimSpace(text)
	case 1:
		text = strings.TrimLeft(text, " \t\r\n")
	case 2:
		text = strings.TrimRight(text, " \t\r\n")
	}
	L.Push(lua.LString(text))
	return 1
}

// defaultDateFormat is what mf.date() uses with no format.
const defaultDateFormat = "%a %b %d %H:%M:%S %Y"

// macroDate is mf.date([format]): the current local time in a C strftime
// format (English day and month names).
func macroDate(L *lua.LState) int {
	format := ""
	if L.GetTop() >= 1 {
		format = L.CheckString(1)
	}
	L.Push(lua.LString(strftime(format, time.Now())))
	return 1
}

// strftime formats t as C's strftime would in the "C" locale; an empty format
// means defaultDateFormat and an unknown conversion is kept as written.
func strftime(format string, t time.Time) string {
	if format == "" {
		format = defaultDateFormat
	}
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' || i+1 >= len(format) {
			b.WriteByte(c)
			continue
		}
		i++
		switch format[i] {
		case 'a':
			b.WriteString(t.Format("Mon"))
		case 'A':
			b.WriteString(t.Format("Monday"))
		case 'b', 'h':
			b.WriteString(t.Format("Jan"))
		case 'B':
			b.WriteString(t.Format("January"))
		case 'd':
			b.WriteString(t.Format("02"))
		case 'e':
			b.WriteString(t.Format("_2"))
		case 'H':
			b.WriteString(t.Format("15"))
		case 'I':
			b.WriteString(t.Format("03"))
		case 'j':
			b.WriteString(pad(t.YearDay(), 3))
		case 'm':
			b.WriteString(t.Format("01"))
		case 'M':
			b.WriteString(t.Format("04"))
		case 'p':
			b.WriteString(t.Format("PM"))
		case 'S':
			b.WriteString(t.Format("05"))
		case 'y':
			b.WriteString(t.Format("06"))
		case 'Y':
			b.WriteString(t.Format("2006"))
		case 'Z':
			b.WriteString(t.Format("MST"))
		case 'z':
			b.WriteString(t.Format("-0700"))
		case 'D':
			b.WriteString(t.Format("01/02/06"))
		case 'T':
			b.WriteString(t.Format("15:04:05"))
		case 'R':
			b.WriteString(t.Format("15:04"))
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(format[i])
		}
	}
	return b.String()
}

func pad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}
