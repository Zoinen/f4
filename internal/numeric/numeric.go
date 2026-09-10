// Package numeric holds the checked conversions the tree shares.
//
// They exist because a silent truncation on a 32-bit target is a bug nobody
// reproduces, and because gosec's G115 has to be answered once per conversion
// rather than once per copy of it: seven packages use these, and seven copies
// would be seven places for a #nosec annotation to go missing.
package numeric

import (
	"strconv"
	"unicode/utf8"
)

func BoundedInt64ToInt(v int64) (int, bool) {
	if strconv.IntSize == 32 && (v < -1<<31 || v > 1<<31-1) {
		return 0, false
	}
	// #nosec G115 -- the 32-bit case is range-checked above; on 64-bit int and int64 have the same range.
	return int(v), true
}

func BoundedUint64ToInt(v uint64) (int, bool) {
	if (strconv.IntSize == 32 && v > 1<<31-1) || (strconv.IntSize == 64 && v > 1<<63-1) {
		return 0, false
	}
	// #nosec G115 -- v is bounded to the platform's maximum int above.
	return int(v), true
}

func NonNegativeUint64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	// #nosec G115 -- negative sizes and offsets are clamped before conversion.
	return uint64(v)
}

// BoundedInt64 clamps a uint64 to the positive range of an int64. Sample and
// frame counts arrive from container headers as unsigned and are multiplied
// before use; a value past the sign bit would come back negative and read as
// a stream running backwards.
func BoundedInt64(v uint64) int64 {
	if v > 1<<63-1 {
		return 1<<63 - 1
	}
	// #nosec G115 -- clamped to the positive int64 range above.
	return int64(v)
}

func BoundedInt16(v int) (int16, bool) {
	if v < -1<<15 || v > 1<<15-1 {
		return 0, false
	}
	// #nosec G115 -- v is bounded to the int16 range above.
	return int16(v), true
}

func BoundedInt32(v int) (int32, bool) {
	if strconv.IntSize == 64 && (int64(v) < -1<<31 || int64(v) > 1<<31-1) {
		return 0, false
	}
	// #nosec G115 -- v is bounded to int32 above; on 32-bit systems the ranges are identical.
	return int32(v), true
}

func BoundedUint16(v int) (uint16, bool) {
	if v < 0 || v > 1<<16-1 {
		return 0, false
	}
	// #nosec G115 -- v is bounded to the uint16 range above.
	return uint16(v), true
}

func BoundedUint32(v int) (uint32, bool) {
	if v < 0 || (strconv.IntSize == 64 && int64(v) > 1<<32-1) {
		return 0, false
	}
	// #nosec G115 -- v is non-negative and, on 64-bit systems, bounded to uint32 above.
	return uint32(v), true
}

func BoundedRune(v int) (rune, bool) {
	if v < 0 || v > utf8.MaxRune {
		return 0, false
	}
	// #nosec G115 -- v is within the Unicode range and the scalar-value check follows immediately.
	r := rune(v)
	return r, utf8.ValidRune(r)
}

func RuneCodepoint(r rune) (uint, bool) {
	if !utf8.ValidRune(r) {
		return 0, false
	}
	// #nosec G115 -- utf8.ValidRune rejects negative runes before conversion.
	return uint(r), true
}
