package main

import (
	"runtime/debug"
	"strconv"
	"unicode/utf8"

	"github.com/unxed/vtui"
)

// ScreenRow reads a stretch of one row back out of the screen.
func ScreenRow(scr *vtui.ScreenBuf, y, x1, x2 int) string {
	runes := make([]rune, x2-x1+1)
	for i := range runes {
		cell := scr.GetCell(x1+i, y)
		runes[i] = vtui.CellBaseRune(cell.Char)
		if runes[i] == 0 {
			runes[i] = ' '
		}
	}
	return string(runes)
}

func boundedInt64ToInt(v int64) (int, bool) {
	if strconv.IntSize == 32 && (v < -1<<31 || v > 1<<31-1) {
		return 0, false
	}
	// #nosec G115 -- the 32-bit case is range-checked above; on 64-bit int and int64 have the same range.
	return int(v), true
}

func boundedUint64ToInt(v uint64) (int, bool) {
	if (strconv.IntSize == 32 && v > 1<<31-1) || (strconv.IntSize == 64 && v > 1<<63-1) {
		return 0, false
	}
	// #nosec G115 -- v is bounded to the platform's maximum int above.
	return int(v), true
}

func nonNegativeUint64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	// #nosec G115 -- negative sizes and offsets are clamped before conversion.
	return uint64(v)
}

func boundedInt16(v int) (int16, bool) {
	if v < -1<<15 || v > 1<<15-1 {
		return 0, false
	}
	// #nosec G115 -- v is bounded to the int16 range above.
	return int16(v), true
}

func boundedInt32(v int) (int32, bool) {
	if strconv.IntSize == 64 && (int64(v) < -1<<31 || int64(v) > 1<<31-1) {
		return 0, false
	}
	// #nosec G115 -- v is bounded to int32 above; on 32-bit systems the ranges are identical.
	return int32(v), true
}

func boundedUint16(v int) (uint16, bool) {
	if v < 0 || v > 1<<16-1 {
		return 0, false
	}
	// #nosec G115 -- v is bounded to the uint16 range above.
	return uint16(v), true
}

func boundedUint32(v int) (uint32, bool) {
	if v < 0 || (strconv.IntSize == 64 && int64(v) > 1<<32-1) {
		return 0, false
	}
	// #nosec G115 -- v is non-negative and, on 64-bit systems, bounded to uint32 above.
	return uint32(v), true
}

func boundedRune(v int) (rune, bool) {
	if v < 0 || v > utf8.MaxRune {
		return 0, false
	}
	// #nosec G115 -- v is within the Unicode range and the scalar-value check follows immediately.
	r := rune(v)
	return r, utf8.ValidRune(r)
}

func runeCodepoint(r rune) (uint, bool) {
	if !utf8.ValidRune(r) {
		return 0, false
	}
	// #nosec G115 -- utf8.ValidRune rejects negative runes before conversion.
	return uint(r), true
}

// ReleaseHeavyMemory forces GC and releases OS memory when heavy sessions (>50MB) close.
func ReleaseHeavyMemory(sizeBytes int64) {
	if sizeBytes > 50*1024*1024 {
		debug.FreeOSMemory()
	}
}
