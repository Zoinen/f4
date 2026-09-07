package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
)

func u16(b []byte, order binary.ByteOrder) uint16 {
	if len(b) < 2 {
		return 0
	}
	return order.Uint16(b)
}

func u32(b []byte, order binary.ByteOrder) uint32 {
	if len(b) < 4 {
		return 0
	}
	return order.Uint32(b)
}

func u64(b []byte, order binary.ByteOrder) uint64 {
	if len(b) < 8 {
		return 0
	}
	return order.Uint64(b)
}

func fourCC(b []byte) string {
	if len(b) < 4 {
		return ""
	}
	s := string(b[:4])
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			return fmt.Sprintf("0x%08X", binary.BigEndian.Uint32(b[:4]))
		}
	}
	return strings.TrimSpace(s)
}

func cleanText(b []byte) string {
	// Trim while this is still a byte slice, then allocate only the compact
	// result. Converting first would let a tiny trimmed string retain a large
	// backing allocation in reports and their cache.
	b = bytes.TrimRight(b, "\x00")
	b = bytes.TrimSpace(b)
	return string(b)
}

func durationFromUnits(value uint64, scale uint64) time.Duration {
	if value == 0 || scale == 0 {
		return 0
	}
	seconds := float64(value) / float64(scale)
	if seconds > float64(math.MaxInt64)/float64(time.Second) {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func metadataInt(value uint64) (int, bool) {
	if value > uint64(^uint(0)>>1) {
		return 0, false
	}
	// #nosec G115 -- the architecture-sized MaxInt check above makes this conversion lossless.
	return int(value), true
}

func metadataDuration(value uint64) (time.Duration, bool) {
	if value > math.MaxInt64 {
		return 0, false
	}
	// #nosec G115 -- the explicit MaxInt64 check above makes this conversion lossless.
	return time.Duration(value), true
}

func signedInt32Bits(value uint32) int32 {
	// #nosec G115 -- binary formats encode signed int32 values as these exact two's-complement bits.
	return int32(value)
}

func parseISO639(v uint16) string {
	if v == 0 {
		return ""
	}
	b := []byte{byte((v>>10)&0x1f) + 0x60, byte((v>>5)&0x1f) + 0x60, byte(v&0x1f) + 0x60}
	for _, c := range b {
		if c < 'a' || c > 'z' {
			return ""
		}
	}
	return string(b)
}

func boolPtr(v bool) *bool { return &v }

func decodeUTF16(b []byte, little bool) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	words := make([]uint16, 0, len(b)/2)
	for len(b) >= 2 {
		var w uint16
		if little {
			w = binary.LittleEndian.Uint16(b)
		} else {
			w = binary.BigEndian.Uint16(b)
		}
		if w == 0 {
			break
		}
		words = append(words, w)
		b = b[2:]
	}
	decoded := utf16.Decode(words)
	start, end := 0, len(decoded)
	for start < end && unicode.IsSpace(decoded[start]) {
		start++
	}
	for end > start && unicode.IsSpace(decoded[end-1]) {
		end--
	}
	return string(decoded[start:end])
}

func parseDecimal(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func canonicalTag(code string) string {
	if len(code) == 4 && code[0] == 0xa9 {
		switch strings.ToUpper(code[1:]) {
		case "NAM":
			return "Title"
		case "ART":
			return "Artist"
		case "ALB":
			return "Album"
		case "CMT":
			return "Comment"
		case "GEN":
			return "Genre"
		case "DAY":
			return "Date"
		case "TOO":
			return "Encoded application"
		case "WRT":
			return "Composer"
		}
	}
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "INAM", "TITLE", "TIT2", "©NAM":
		return "Title"
	case "IART", "ARTIST", "TPE1", "©ART":
		return "Artist"
	case "IPRD", "ALBUM", "TALB", "©ALB":
		return "Album"
	case "ICMT", "COMMENT", "COMM", "©CMT":
		return "Comment"
	case "ICOP", "COPYRIGHT", "TCOP", "CPRT":
		return "Copyright"
	case "IGNR", "GENRE", "TCON", "©GEN":
		return "Genre"
	case "ICRD", "DATE", "YEAR", "TDRC", "TYER", "©DAY":
		return "Date"
	case "ISFT", "ENCODER", "TSSE", "©TOO":
		return "Encoded application"
	case "TRACKNUMBER", "TRCK":
		return "Track"
	case "DISCNUMBER", "TPOS":
		return "Disc"
	case "ALBUMARTIST", "TPE2", "AART":
		return "Album artist"
	case "COMPOSER", "TCOM", "©WRT":
		return "Composer"
	}
	return strings.TrimSpace(code)
}
