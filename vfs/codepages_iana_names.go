//go:build !windows

package vfs

import (
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// singleByteIANANames are the names the IANA index gives the single-byte
// encodings a system locale can use. The extra-lite build names them from
// here: the index itself imports the East Asian tables (f4#1671).
var singleByteIANANames = map[encoding.Encoding]string{
	charmap.Windows1250: "windows-1250",
	charmap.Windows1251: "windows-1251",
	charmap.Windows1252: "windows-1252",
	charmap.Windows1253: "windows-1253",
	charmap.Windows1254: "windows-1254",
	charmap.Windows1255: "windows-1255",
	charmap.Windows1256: "windows-1256",
	charmap.Windows1257: "windows-1257",
	charmap.Windows1258: "windows-1258",
	charmap.Windows874:  "windows-874",
	charmap.CodePage437: "IBM437",
	charmap.CodePage850: "IBM850",
	charmap.CodePage852: "IBM852",
	charmap.CodePage855: "IBM855",
	charmap.CodePage860: "IBM860",
	charmap.CodePage862: "IBM862",
	charmap.CodePage863: "IBM863",
	charmap.CodePage865: "IBM865",
	charmap.CodePage866: "IBM866",
	charmap.ISO8859_1:   "ISO_8859-1:1987",
	charmap.ISO8859_2:   "ISO_8859-2:1987",
	charmap.ISO8859_3:   "ISO_8859-3:1988",
	charmap.ISO8859_4:   "ISO_8859-4:1988",
	charmap.ISO8859_5:   "ISO_8859-5:1988",
	charmap.ISO8859_6:   "ISO_8859-6:1987",
	charmap.ISO8859_7:   "ISO_8859-7:1987",
	charmap.ISO8859_8:   "ISO_8859-8:1988",
	charmap.ISO8859_9:   "ISO_8859-9:1989",
	charmap.ISO8859_10:  "ISO-8859-10",
	charmap.ISO8859_13:  "ISO-8859-13",
	charmap.ISO8859_14:  "ISO-8859-14",
	charmap.ISO8859_15:  "ISO-8859-15",
	charmap.ISO8859_16:  "ISO-8859-16",
	charmap.KOI8R:       "KOI8-R",
	charmap.KOI8U:       "KOI8-U",
}
