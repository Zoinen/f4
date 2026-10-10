package observer

import "encoding/binary"

// encodeWChars renders s as a NUL-terminated wchar_t string the way a
// wasi-sdk wasm32 module reads it: one Unicode code point per 4-byte
// little-endian unit. The returned slice's length is always a multiple of
// wcharSize and always includes the terminating NUL unit.
func encodeWChars(s string) []byte {
	runes := []rune(s)
	b := make([]byte, (len(runes)+1)*wcharSize)
	for i, r := range runes {
		binary.LittleEndian.PutUint32(b[i*wcharSize:], uint32(r))
	}
	// The trailing NUL unit is already zero from make().
	return b
}

// decodeWCharField reads a NUL-terminated (or field-length-bound) wchar_t
// string out of a fixed-size struct field already copied into b.
func decodeWCharField(b []byte) string {
	runes := make([]rune, 0, len(b)/wcharSize)
	for off := 0; off+wcharSize <= len(b); off += wcharSize {
		v := binary.LittleEndian.Uint32(b[off:])
		if v == 0 {
			break
		}
		// #nosec G115 -- v is a wchar_t code point out of guest memory and
		// may be anything a buggy or malicious module wrote; string() below
		// replaces a rune outside the Unicode range with U+FFFD, so an
		// out-of-range v only garbles the decoded text, it cannot misbehave.
		runes = append(runes, rune(v))
	}
	return string(runes)
}
