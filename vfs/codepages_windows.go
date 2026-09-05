//go:build windows
// +build windows

package vfs

import (
	"fmt"
	"sort"
	"strconv"
	"unicode/utf16"
	"unsafe"

	"github.com/unxed/localecp"
	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

const cpSupported = 2

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	enumSystemCodePagesW = kernel32.NewProc("EnumSystemCodePagesW")
	getACP               = kernel32.NewProc("GetACP")
	getOEMCP             = kernel32.NewProc("GetOEMCP")
	wideCharToMultiByte  = kernel32.NewProc("WideCharToMultiByte")
)

// platformCodepages asks Windows which code pages it can convert beyond the
// built-in table.
//
// This runs from package init: before main, before the log file exists,
// before the console has been touched. Anything that wedges here leaves a
// process that sits in memory with nothing on screen and nothing on disk to
// explain itself (#895). So the rule is that startup only *lists* code
// pages; it never asks Windows to *open* one.
//
// Opening is what GetCPInfoExW does. A .nls code page is cheap, but the
// DLL-backed ones (c_gsm7.dll for 55000, c_iscii.dll, c_is2022.dll, ...)
// are LoadLibrary'd on first touch, with their DllMains -- and msvcrt.dll,
// which nothing else in f4 links -- running under the loader lock. Doing
// that a hundred times at startup was slow everywhere, and it was done
// from inside the EnumSystemCodePagesW callback, nested in the enumeration
// with the Go runtime's callback trampoline in between. That is exactly
// where #895 stops: c_gsm7.dll is mapped, and the process never returns
// from the callback.
//
// Now the callback records numbers and nothing else. Names come from the
// table below rather than from the code page's own DLL, and the DLL is
// loaded by MultiByteToWideChar the first time somebody actually converts
// text in that code page -- long after startup, from a place that can
// report a failure.
func platformCodepages() []Codepage {
	ids := enumerateWindowsCodepages()
	if len(ids) == 0 {
		return nil
	}
	sort.Ints(ids)

	result := make([]Codepage, 0, len(ids))
	for _, cp := range ids {
		if _, exists := FindCodepage(cp); exists {
			continue
		}
		result = append(result, Codepage{
			ID:    cp,
			Name:  windowsCodepageName(cp),
			Enc:   windowsCodepageEncoding(cp),
			group: codepageOther,
		})
	}
	return result
}

// enumerateWindowsCodepages returns the IDs Windows reports as supported.
// The callback must not call back into Win32: it is invoked from inside
// EnumSystemCodePagesW, and everything it needs (a decimal string to an
// int) is plain Go.
func enumerateWindowsCodepages() []int {
	var ids []int
	seen := make(map[int]struct{})
	callback := windows.NewCallback(func(name *uint16) uintptr {
		cp, err := parseWindowsCodepage(name)
		if err != nil || cp <= 0 {
			return 1
		}
		if _, dup := seen[cp]; dup {
			return 1
		}
		seen[cp] = struct{}{}
		ids = append(ids, cp)
		return 1
	})

	if resultCode, _, _ := enumSystemCodePagesW.Call(callback, cpSupported); resultCode == 0 {
		return nil
	}
	return ids
}

func parseWindowsCodepage(name *uint16) (int, error) {
	if name == nil {
		return 0, fmt.Errorf("nil code page name")
	}
	return strconv.Atoi(windows.UTF16PtrToString(name))
}

func windowsCodepageName(cp int) string {
	if name, ok := windowsCodepageNames[cp]; ok {
		return name
	}
	return fmt.Sprintf("Windows codepage %d", cp)
}

// windowsCodepageNames names the code pages Windows can enumerate that are
// not in the built-in table, after Microsoft's code page identifier list.
// Static on purpose: see platformCodepages.
var windowsCodepageNames = map[int]string{
	500:   "IBM EBCDIC International",
	708:   "Arabic (ASMO 708)",
	720:   "DOS Arabic (Transparent ASMO)",
	737:   "DOS Greek",
	775:   "DOS Baltic",
	857:   "DOS Turkish",
	861:   "DOS Icelandic",
	864:   "DOS Arabic",
	869:   "DOS Modern Greek",
	870:   "IBM EBCDIC Multilingual Latin 2",
	875:   "IBM EBCDIC Greek Modern",
	949:   "Korean (ks_c_5601-1987)",
	1026:  "IBM EBCDIC Turkish (Latin 5)",
	1141:  "IBM EBCDIC Germany (Euro)",
	1142:  "IBM EBCDIC Denmark-Norway (Euro)",
	1143:  "IBM EBCDIC Finland-Sweden (Euro)",
	1144:  "IBM EBCDIC Italy (Euro)",
	1145:  "IBM EBCDIC Latin America-Spain (Euro)",
	1146:  "IBM EBCDIC United Kingdom (Euro)",
	1147:  "IBM EBCDIC France (Euro)",
	1148:  "IBM EBCDIC International (Euro)",
	1149:  "IBM EBCDIC Icelandic (Euro)",
	1361:  "Korean (Johab)",
	10000: "Mac Roman",
	10001: "Mac Japanese",
	10002: "Mac Traditional Chinese (Big5)",
	10003: "Mac Korean",
	10004: "Mac Arabic",
	10005: "Mac Hebrew",
	10006: "Mac Greek",
	10007: "Mac Cyrillic",
	10008: "Mac Simplified Chinese (GB 2312)",
	10010: "Mac Romanian",
	10017: "Mac Ukrainian",
	10021: "Mac Thai",
	10029: "Mac Latin 2",
	10079: "Mac Icelandic",
	10081: "Mac Turkish",
	10082: "Mac Croatian",
	20000: "CNS Taiwan",
	20001: "TCA Taiwan",
	20002: "Eten Taiwan",
	20003: "IBM5550 Taiwan",
	20004: "TeleText Taiwan",
	20005: "Wang Taiwan",
	20105: "IA5 (IRV International Alphabet No. 5)",
	20106: "IA5 German (7-bit)",
	20107: "IA5 Swedish (7-bit)",
	20108: "IA5 Norwegian (7-bit)",
	20261: "T.61",
	20269: "ISO 6937 Non-Spacing Accent",
	20273: "IBM EBCDIC Germany",
	20277: "IBM EBCDIC Denmark-Norway",
	20278: "IBM EBCDIC Finland-Sweden",
	20280: "IBM EBCDIC Italy",
	20284: "IBM EBCDIC Latin America-Spain",
	20285: "IBM EBCDIC United Kingdom",
	20290: "IBM EBCDIC Japanese Katakana Extended",
	20297: "IBM EBCDIC France",
	20420: "IBM EBCDIC Arabic",
	20423: "IBM EBCDIC Greek",
	20424: "IBM EBCDIC Hebrew",
	20833: "IBM EBCDIC Korean Extended",
	20838: "IBM EBCDIC Thai",
	20871: "IBM EBCDIC Icelandic",
	20880: "IBM EBCDIC Cyrillic Russian",
	20905: "IBM EBCDIC Turkish",
	20924: "IBM EBCDIC Latin 1/Open System (Euro)",
	20932: "EUC-JP (JIS 0208-1990 and 0212-1990)",
	20936: "GB2312-80 (Simplified Chinese)",
	20949: "Korean Wansung",
	21025: "IBM EBCDIC Cyrillic Serbian-Bulgarian",
	29001: "Europa 3",
	38598: "ISO 8859-8 Hebrew (logical)",
	50221: "ISO 2022 JP (allow 1 byte Kana)",
	50222: "ISO 2022 JP (allow 1 byte Kana - SO/SI)",
	50225: "ISO 2022 Korean",
	50227: "ISO 2022 Simplified Chinese",
	50229: "ISO 2022 Traditional Chinese",
	50930: "EBCDIC Japanese (Katakana) Extended",
	50931: "EBCDIC US-Canada and Japanese",
	50933: "EBCDIC Korean Extended and Korean",
	50935: "EBCDIC Simplified Chinese Extended and Simplified Chinese",
	50936: "EBCDIC Simplified Chinese",
	50937: "EBCDIC US-Canada and Traditional Chinese",
	50939: "EBCDIC Japanese (Latin) Extended and Japanese",
	51936: "EUC Simplified Chinese",
	51950: "EUC Traditional Chinese",
	55000: "GSM 7-bit",
	57002: "ISCII Devanagari",
	57003: "ISCII Bengali",
	57004: "ISCII Tamil",
	57005: "ISCII Telugu",
	57006: "ISCII Assamese",
	57007: "ISCII Oriya",
	57008: "ISCII Kannada",
	57009: "ISCII Malayalam",
	57010: "ISCII Gujarati",
	57011: "ISCII Punjabi",
	65000: "UTF-7",
}

func windowsSystemCodepage(proc *windows.LazyProc) int {
	cp, _, _ := proc.Call()
	return int(cp)
}

// systemCodepageIDs are the numbers behind the "System ANSI" and "System
// OEM" entries. They come from localecp, which owns the matching encodings,
// so the number shown and the decoder used can never disagree -- and on a
// machine set to a UTF-8 system codepage, where GetACP and GetOEMCP both say
// 65001, localecp answers with the locale's legacy codepages (1252 / 850 on
// the #875 reporter's box, as Far shows), not with a "System ANSI" that is
// UTF-8 under another name. The raw calls remain the fallback for a localecp
// that has no number.
func systemCodepageIDs() (int, int) {
	ansi, oem := localecp.ANSICodepage, localecp.OEMCodepage
	if ansi == 0 {
		ansi = windowsSystemCodepage(getACP)
	}
	if oem == 0 {
		oem = windowsSystemCodepage(getOEMCP)
	}
	return ansi, oem
}

func systemCodepageNames() (string, string) {
	return fmt.Sprintf("System ANSI (%d)", systemANSI), fmt.Sprintf("System OEM (%d)", systemOEM)
}

type windowsCodepage struct {
	cp uint32
}

func windowsCodepageEncoding(cp int) encoding.Encoding {
	return windowsCodepage{cp: uint32(cp)}
}

func (e windowsCodepage) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: windowsCodepageTransformer{cp: e.cp}}
}

func (e windowsCodepage) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: windowsCodepageTransformer{cp: e.cp, encode: true}}
}

type windowsCodepageTransformer struct {
	cp     uint32
	encode bool
}

func (windowsCodepageTransformer) Reset() {}

func (t windowsCodepageTransformer) Transform(dst, src []byte, atEOF bool) (int, int, error) {
	if len(src) == 0 {
		return 0, 0, nil
	}
	if t.encode {
		return t.encodeBytes(dst, src)
	}
	return t.decodeBytes(dst, src)
}

func (t windowsCodepageTransformer) decodeBytes(dst, src []byte) (int, int, error) {
	wideLen, err := windows.MultiByteToWideChar(t.cp, 0, &src[0], int32(len(src)), nil, 0)
	if err != nil {
		return 0, 0, err
	}
	wide := make([]uint16, wideLen)
	if _, err = windows.MultiByteToWideChar(t.cp, 0, &src[0], int32(len(src)), &wide[0], wideLen); err != nil {
		return 0, 0, err
	}
	out := []byte(string(utf16.Decode(wide)))
	if len(out) > len(dst) {
		return 0, 0, transform.ErrShortDst
	}
	return copy(dst, out), len(src), nil
}

func (t windowsCodepageTransformer) encodeBytes(dst, src []byte) (int, int, error) {
	wide := utf16.Encode([]rune(string(src)))
	defaultChar := byte('?')
	usedDefaultChar := int32(0)
	outLen, err := callWideCharToMultiByte(t.cp, wide, nil, nil, nil)
	if err != nil {
		return 0, 0, err
	}
	out := make([]byte, outLen)
	outLen, err = callWideCharToMultiByte(t.cp, wide, &defaultChar, &usedDefaultChar, out)
	if err != nil {
		return 0, 0, err
	}
	if usedDefaultChar != 0 {
		return 0, 0, fmt.Errorf("character cannot be represented in Windows codepage %d", t.cp)
	}
	if outLen > len(dst) {
		return 0, 0, transform.ErrShortDst
	}
	return copy(dst, out[:outLen]), len(src), nil
}

func callWideCharToMultiByte(cp uint32, wide []uint16, defaultChar *byte, usedDefaultChar *int32, dst []byte) (int, error) {
	var widePtr uintptr
	if len(wide) > 0 {
		widePtr = uintptr(unsafe.Pointer(&wide[0]))
	}
	var defaultPtr uintptr
	if defaultChar != nil {
		defaultPtr = uintptr(unsafe.Pointer(defaultChar))
	}
	var usedPtr uintptr
	if usedDefaultChar != nil {
		usedPtr = uintptr(unsafe.Pointer(usedDefaultChar))
	}
	var dstPtr uintptr
	if len(dst) > 0 {
		dstPtr = uintptr(unsafe.Pointer(&dst[0]))
	}
	result, _, err := wideCharToMultiByte.Call(
		uintptr(cp), 0, widePtr, uintptr(len(wide)), dstPtr, uintptr(len(dst)), defaultPtr, usedPtr,
	)
	if result == 0 {
		return 0, err
	}
	return int(result), nil
}
