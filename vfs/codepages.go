package vfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/unxed/vtui"
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
		{11111, "1251 ANSI (Cyrillic)", nil},
		{22222, "866 OEM (Russian)", nil},
		{1200, "UTF-16 (Little endian)", unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)},
		{1201, "UTF-16 (Big endian)", unicode.UTF16(unicode.BigEndian, unicode.UseBOM)},
		{1251, "Windows-1251 (Cyrillic)", charmap.Windows1251},
		{866, "CP866 (Cyrillic OEM)", charmap.CodePage866},
		{20866, "KOI8-R (Cyrillic)", charmap.KOI8R},
		{1252, "Windows-1252 (Western)", charmap.Windows1252},
		{437, "CP437 (US OEM)", charmap.CodePage437},
		{850, "CP850 (Western OEM)", charmap.CodePage850},
		{852, "CP852 (Slavic OEM)", charmap.CodePage852},
	}
}

func DisplayCodepageName(id int) string {
	if id == 11111 {
		return "ANSI"
	}
	if id == 22222 {
		return "OEM"
	}
	if id == 65001 {
		return "UTF-8"
	}
	if cp, ok := FindCodepage(id); ok {
		return cp.Name
	}
	return fmt.Sprintf("%d", id)
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
		if enc := GetSystemANSIEncoding(); enc != nil {
			decoder = enc.NewDecoder()
		}
	} else if cpID == 22222 {
		if enc := GetSystemOEMEncoding(); enc != nil {
			decoder = enc.NewDecoder()
		}
	} else {
		cp, ok := FindCodepage(cpID)
		if !ok || cp.Enc == nil {
			return data, fmt.Errorf("unsupported codepage: %d", cpID)
		}
		decoder = cp.Enc.NewDecoder()
	}

	if decoder == nil {
		return data, fmt.Errorf("decoder is nil for codepage: %d", cpID)
	}

	return decoder.Bytes(data)
}

func EncodeBytes(data []byte, cpID int) ([]byte, error) {
	if cpID == 65001 {
		return data, nil
	}

	var encoder *encoding.Encoder
	if cpID == 11111 {
		if enc := GetSystemANSIEncoding(); enc != nil {
			encoder = enc.NewEncoder()
		}
	} else if cpID == 22222 {
		if enc := GetSystemOEMEncoding(); enc != nil {
			encoder = enc.NewEncoder()
		}
	} else {
		cp, ok := FindCodepage(cpID)
		if !ok || cp.Enc == nil {
			return data, fmt.Errorf("unsupported codepage: %d", cpID)
		}
		encoder = cp.Enc.NewEncoder()
	}

	if encoder == nil {
		return data, fmt.Errorf("encoder is nil for codepage: %d", cpID)
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
		if enc := GetSystemANSIEncoding(); enc != nil {
			return enc.NewDecoder(), enc.NewEncoder()
		}
		return nil, nil
	}
	if id == 22222 {
		if enc := GetSystemOEMEncoding(); enc != nil {
			return enc.NewDecoder(), enc.NewEncoder()
		}
		return nil, nil
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
func BuildCodepageMenuItems(currentCpID int, autoDetect bool) ([]vtui.MenuItem, int) {
	var items []vtui.MenuItem
	currIdx := 0

	addHeader := func(title string) {
		items = append(items, vtui.MenuItem{Text: title, Separator: true})
	}

	addCP := func(cp Codepage) {
		if cp.ID == 1251 || cp.ID == 866 {
			return // Exclude duplicate 1251 and 866 from the UI menu
		}
		var text string
		if cp.ID == 11111 || cp.ID == 22222 {
			text = cp.Name // Don't show technical "11111" / "22222" IDs
		} else {
			text = fmt.Sprintf("%5d  %s", cp.ID, cp.Name)
		}

		if cp.ID == currentCpID && !autoDetect {
			text = "√ " + text
			currIdx = len(items)
		} else {
			text = "  " + text
		}
		items = append(items, vtui.MenuItem{
			Text:     text,
			UserData: cp.ID,
		})
	}

	autoText := "  Auto-detect "
	if autoDetect {
		autoText = "√ Auto-detect "
		currIdx = 0
	}
	items = append(items, vtui.MenuItem{
		Text:     autoText,
		UserData: -1,
	})

	addHeader(" System ")
	for _, cp := range AvailableCodepages {
		if cp.ID == 11111 || cp.ID == 22222 {
			addCP(cp)
		}
	}

	addHeader(" Unicode ")
	for _, cp := range AvailableCodepages {
		if cp.ID == 65001 || cp.ID == 1200 || cp.ID == 1201 {
			addCP(cp)
		}
	}

	addHeader(" Other ")
	for _, cp := range AvailableCodepages {
		if cp.ID != 11111 && cp.ID != 22222 && cp.ID != 65001 && cp.ID != 1200 && cp.ID != 1201 {
			addCP(cp)
		}
	}

	return items, currIdx
}
