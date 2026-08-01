package vfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/unxed/localecp"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

type Codepage struct {
	ID   int
	Name string
	Enc  encoding.Encoding
}

var AvailableCodepages []Codepage

func init() {
	AvailableCodepages = []Codepage{
		{65001, "UTF-8", unicode.UTF8},
		{1200, "UTF-16 (Little endian)", unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)},
		{1201, "UTF-16 (Big endian)", unicode.UTF16(unicode.BigEndian, unicode.UseBOM)},
		{11111, "System ANSI", nil},
		{22222, "System OEM", nil},
		{1251, "Windows-1251 (Cyrillic)", charmap.Windows1251},
		{866, "CP866 (Cyrillic OEM)", charmap.CodePage866},
		{20866, "KOI8-R (Cyrillic)", charmap.KOI8R},
		{1252, "Windows-1252 (Western)", charmap.Windows1252},
		{437, "CP437 (US OEM)", charmap.CodePage437},
		{850, "CP850 (Western OEM)", charmap.CodePage850},
		{852, "CP852 (Slavic OEM)", charmap.CodePage852},
	}
}

func FindCodepage(id int) (Codepage, bool) {
	for _, cp := range AvailableCodepages {
		if cp.ID == id {
			return cp, true
		}
	}
	return Codepage{}, false
}

func DecodeBytes(data []byte, cpID int) ([]byte, error) {
	if cpID == 65001 {
		return data, nil
	}

	var decoder *encoding.Decoder
	if cpID == 11111 {
		decoder = localecp.ANSIDecoder
	} else if cpID == 22222 {
		decoder = localecp.OEMDecoder
	} else {
		cp, ok := FindCodepage(cpID)
		if !ok || cp.Enc == nil {
			return data, fmt.Errorf("unsupported codepage: %d", cpID)
		}
		decoder = cp.Enc.NewDecoder()
	}

	return decoder.Bytes(data)
}

func EncodeBytes(data []byte, cpID int) ([]byte, error) {
	if cpID == 65001 {
		return data, nil
	}

	var encoder *encoding.Encoder
	if cpID == 11111 {
		encoder = localecp.ANSIEncoder
	} else if cpID == 22222 {
		if systemOEM := GetSystemOEMEncoding(); systemOEM != nil {
			encoder = systemOEM.NewEncoder()
		} else {
			encoder = charmap.CodePage437.NewEncoder()
		}
	} else {
		cp, ok := FindCodepage(cpID)
		if !ok || cp.Enc == nil {
			return data, fmt.Errorf("unsupported codepage: %d", cpID)
		}
		encoder = cp.Enc.NewEncoder()
	}

	return encoder.Bytes(data)
}

func DetectBOM(data []byte) (int, bool) {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return 65001, true
	}
	if len(data) >= 2 {
		if data[0] == 0xFF && data[1] == 0xFE {
			return 1200, true
		}
		if data[0] == 0xFE && data[1] == 0xFF {
			return 1201, true
		}
	}
	return 65001, false
}

func DetectEncoding(data []byte, autodetect bool, defaultCP int) int {
	if cp, ok := DetectBOM(data); ok {
		return cp
	}
	if autodetect {
		if utf8.Valid(data) {
			return 65001
		}
		return defaultCP
	}
	return defaultCP
}

func GetCodepageDecoderEncoder(cp string) (*encoding.Decoder, *encoding.Encoder) {
	if cp == "" || cp == "65001" {
		return nil, nil
	}
	id, _ := strconv.Atoi(cp)
	if id == 11111 {
		return localecp.ANSIDecoder, localecp.ANSIEncoder
	}
	if id == 22222 {
		var enc *encoding.Encoder
		if systemOEM := GetSystemOEMEncoding(); systemOEM != nil {
			enc = systemOEM.NewEncoder()
		}
		return localecp.OEMDecoder, enc
	}
	if cpObj, ok := FindCodepage(id); ok && cpObj.Enc != nil {
		return cpObj.Enc.NewDecoder(), cpObj.Enc.NewEncoder()
	}
	return nil, nil
}

func GetSystemOEMEncoding() encoding.Encoding {
	if oem := getWindowsOEMCP(); oem != nil {
		return oem
	}

	lc := os.Getenv("LC_ALL")
	if lc == "" {
		lc = os.Getenv("LC_CTYPE")
	}
	if lc == "" {
		lc = os.Getenv("LANG")
	}
	if lc == "" || lc == "C" || lc == "POSIX" {
		return charmap.CodePage437
	}

	lcBase := lc
	if idx := strings.IndexByte(lcBase, '.'); idx != -1 {
		lcBase = lcBase[:idx]
	}

	switch lcBase {
	case "ru_RU", "be_BY", "bg_BG", "kk_KZ", "uk_UA", "tt_RU":
		return charmap.CodePage866
	case "cs_CZ", "pl_PL", "hu_HU", "ro_RO", "sk_SK", "hr_HR":
		return charmap.CodePage852
	}
	return charmap.CodePage437
}

func GetSystemANSIEncoding() encoding.Encoding {
	if ansi := getWindowsACP(); ansi != nil {
		return ansi
	}

	lc := os.Getenv("LC_ALL")
	if lc == "" {
		lc = os.Getenv("LC_CTYPE")
	}
	if lc == "" {
		lc = os.Getenv("LANG")
	}
	if lc == "" || lc == "C" || lc == "POSIX" {
		return charmap.Windows1252
	}

	lcBase := lc
	if idx := strings.IndexByte(lcBase, '.'); idx != -1 {
		lcBase = lcBase[:idx]
	}

	switch lcBase {
	case "ru_RU", "be_BY", "bg_BG", "kk_KZ", "uk_UA", "tt_RU":
		return charmap.Windows1251
	case "cs_CZ", "pl_PL", "hu_HU", "ro_RO", "sk_SK", "hr_HR":
		return charmap.Windows1250
	}
	return charmap.Windows1252
}

type MemoryReadAtCloser struct {
	Data []byte
}

func (m *MemoryReadAtCloser) Size() int64 { return int64(len(m.Data)) }
func (m *MemoryReadAtCloser) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if off >= int64(len(m.Data)) {
		return 0, io.EOF
	}
	n := copy(p, m.Data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
func (m *MemoryReadAtCloser) Read(ctx context.Context, p []byte) (int, error) {
	return 0, io.EOF
}
func (m *MemoryReadAtCloser) Close() error { return nil }

func GetNextFastSwitchCodepage(current int) int {
	FastSwitchCodepages := []int{65001, 11111, 22222}
	for i, id := range FastSwitchCodepages {
		if id == current {
			nextIdx := (i + 1) % len(FastSwitchCodepages)
			return FastSwitchCodepages[nextIdx]
		}
	}
	return 65001
}
