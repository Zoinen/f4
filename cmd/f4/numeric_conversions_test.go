package main

import (
	"math"
	"unicode"
)

// These helpers keep test-fixture conversions checked without obscuring the
// assertions that use them. An out-of-range fixture is a test bug, so panic is
// preferable to silently truncating it.
func testRune(value uint64) rune {
	if value > unicode.MaxRune {
		panic("test character is outside the Unicode range")
	}
	return rune(value) // #nosec G115 -- value is bounded by unicode.MaxRune above.
}

func testInt16(value int) int16 {
	if value < math.MinInt16 || value > math.MaxInt16 {
		panic("test coordinate is outside the int16 range")
	}
	return int16(value) // #nosec G115 -- value is bounded to the int16 range above.
}

func testInt32(value int) int32 {
	if int64(value) < math.MinInt32 || int64(value) > math.MaxInt32 {
		panic("test value is outside the int32 range")
	}
	return int32(value) // #nosec G115 -- value is bounded to the int32 range above.
}

func testUint8(value int) uint8 {
	if value < 0 || value > math.MaxUint8 {
		panic("test value is outside the uint8 range")
	}
	return uint8(value) // #nosec G115 -- value is bounded to the uint8 range above.
}

func testUint16(value int) uint16 {
	if value < 0 || value > math.MaxUint16 {
		panic("test value is outside the uint16 range")
	}
	return uint16(value) // #nosec G115 -- value is bounded to the uint16 range above.
}

func testUint32(value int) uint32 {
	// int is 32 bits on the platforms this project still vets, where the
	// untyped MaxUint32 does not fit in one, so widen before comparing.
	if value < 0 || int64(value) > math.MaxUint32 {
		panic("test value is outside the uint32 range")
	}
	return uint32(value) // #nosec G115 -- value is bounded to the uint32 range above.
}

func testUint32Int32Bits(value int) uint32 {
	signed := testInt32(value)
	return uint32(signed) // #nosec G115 -- BMP fixtures intentionally encode a checked signed int32 as raw bits.
}

func testUint16Rune(value rune) uint16 {
	if value < 0 || value > math.MaxUint16 {
		panic("test character is outside the uint16 range")
	}
	return uint16(value) // #nosec G115 -- value is bounded to the uint16 range above.
}

func testUint32Rune(value rune) uint32 {
	if value < 0 {
		panic("test character is negative")
	}
	return uint32(value) // #nosec G115 -- non-negative runes fit in uint32.
}

func testUint64Rune(value rune) uint64 {
	if value < 0 {
		panic("test character is negative")
	}
	return uint64(value) // #nosec G115 -- value is checked to be non-negative above.
}

func testByteInt64(value int64) byte {
	if value < 0 || value > math.MaxUint8 {
		panic("test value is outside the byte range")
	}
	return byte(value) // #nosec G115 -- value is bounded to the byte range above.
}

func testInt64Uint64(value uint64) int64 {
	if value > math.MaxInt64 {
		panic("test value is outside the int64 range")
	}
	return int64(value) // #nosec G115 -- value is bounded to the int64 range above.
}

func testInt64Uint(value uint) int64 {
	// uint is 32 bits on the platforms this project still vets, where the
	// untyped MaxInt64 does not fit in one, so widen before comparing.
	if uint64(value) > math.MaxInt64 {
		panic("test value is outside the int64 range")
	}
	return int64(value) // #nosec G115 -- value is bounded to the int64 range above.
}
