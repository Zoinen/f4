package numeric

import (
	"math"
	"strconv"
	"testing"
	"unicode/utf8"
)

// The conversions exist for the targets where int is 32 bits — linux/386,
// linux/mips, linux/mipsle, linux/arm — so the cases that matter are the ones
// straddling a boundary. On a 64-bit host the widening cases cannot fail, and
// the table says so rather than pretending otherwise.

func TestBoundedInt64ToInt(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   int64
		want int64
		ok   bool
	}{
		{"zero", 0, 0, true},
		{"max int32", math.MaxInt32, math.MaxInt32, true},
		{"min int32", math.MinInt32, math.MinInt32, true},
		{"above int32", math.MaxInt32 + 1, math.MaxInt32 + 1, strconv.IntSize == 64},
		{"below int32", math.MinInt32 - 1, math.MinInt32 - 1, strconv.IntSize == 64},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BoundedInt64ToInt(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && int64(got) != tt.want {
				t.Errorf("value = %d, want %d", got, tt.want)
			}
			if !ok && got != 0 {
				t.Errorf("a refused conversion returned %d, want 0", got)
			}
		})
	}
}

func TestBoundedUint64ToInt(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   uint64
		ok   bool
	}{
		{"zero", 0, true},
		{"max int32", math.MaxInt32, true},
		{"above int32", math.MaxInt32 + 1, strconv.IntSize == 64},
		{"max int64", math.MaxInt64, strconv.IntSize == 64},
		{"above int64", math.MaxInt64 + 1, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BoundedUint64ToInt(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			// #nosec G115 -- ok reports the value fits in an int, which is
			// precisely what is being asserted here.
			if ok && uint64(got) != tt.in {
				t.Errorf("value = %d, want %d", got, tt.in)
			}
		})
	}
}

func TestNonNegativeUint64Clamps(t *testing.T) {
	for _, tt := range []struct {
		in   int64
		want uint64
	}{
		{-1, 0},
		{math.MinInt64, 0},
		{0, 0},
		{1, 1},
		{math.MaxInt64, math.MaxInt64},
	} {
		if got := NonNegativeUint64(tt.in); got != tt.want {
			t.Errorf("NonNegativeUint64(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestBoundedInt16(t *testing.T) {
	for _, tt := range []struct {
		in int
		ok bool
	}{{0, true}, {math.MaxInt16, true}, {math.MinInt16, true}, {math.MaxInt16 + 1, false}, {math.MinInt16 - 1, false}} {
		got, ok := BoundedInt16(tt.in)
		if ok != tt.ok {
			t.Errorf("BoundedInt16(%d) ok = %v, want %v", tt.in, ok, tt.ok)
		}
		if ok && int(got) != tt.in {
			t.Errorf("BoundedInt16(%d) = %d", tt.in, got)
		}
	}
}

func TestBoundedInt32(t *testing.T) {
	for _, tt := range []struct {
		in int
		ok bool
	}{{0, true}, {math.MaxInt32, true}, {math.MinInt32, true}} {
		if got, ok := BoundedInt32(tt.in); !ok || int(got) != tt.in {
			t.Errorf("BoundedInt32(%d) = %d, %v", tt.in, got, ok)
		}
	}
	if strconv.IntSize == 64 {
		// Through a variable: int(int64constant + 1) is still a constant
		// conversion, and a constant that does not fit in an int fails to
		// compile on a 32-bit target even in a branch that never runs.
		aboveInt32 := int64(math.MaxInt32) + 1
		if _, ok := BoundedInt32(int(aboveInt32)); ok {
			t.Error("BoundedInt32 accepted a value above int32 on a 64-bit build")
		}
	}
}

func TestBoundedUint16(t *testing.T) {
	for _, tt := range []struct {
		in int
		ok bool
	}{{0, true}, {math.MaxUint16, true}, {math.MaxUint16 + 1, false}, {-1, false}} {
		if _, ok := BoundedUint16(tt.in); ok != tt.ok {
			t.Errorf("BoundedUint16(%d) ok = %v, want %v", tt.in, ok, tt.ok)
		}
	}
}

func TestBoundedUint32(t *testing.T) {
	for _, tt := range []struct {
		in int
		ok bool
	}{{0, true}, {math.MaxUint16, true}, {-1, false}} {
		if _, ok := BoundedUint32(tt.in); ok != tt.ok {
			t.Errorf("BoundedUint32(%d) ok = %v, want %v", tt.in, ok, tt.ok)
		}
	}
	if strconv.IntSize == 64 {
		aboveUint32 := int64(math.MaxUint32) + 1
		if _, ok := BoundedUint32(int(aboveUint32)); ok {
			t.Error("BoundedUint32 accepted a value above uint32 on a 64-bit build")
		}
	}
}

func TestBoundedRune(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   int
		ok   bool
	}{
		{"ascii", 'A', true},
		{"max rune", utf8.MaxRune, true},
		{"above max rune", utf8.MaxRune + 1, false},
		{"negative", -1, false},
		// A surrogate is inside the numeric range and is not a scalar value:
		// the range check alone would let it through.
		{"surrogate", 0xD800, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := BoundedRune(tt.in); ok != tt.ok {
				t.Errorf("BoundedRune(%#x) ok = %v, want %v", tt.in, ok, tt.ok)
			}
		})
	}
}

func TestRuneCodepoint(t *testing.T) {
	if got, ok := RuneCodepoint('é'); !ok || got != uint('é') {
		t.Errorf("RuneCodepoint('é') = %d, %v", got, ok)
	}
	for _, r := range []rune{-1, utf8.MaxRune + 1, 0xDFFF} {
		if _, ok := RuneCodepoint(r); ok {
			t.Errorf("RuneCodepoint(%#x) accepted an invalid rune", r)
		}
	}
}

// ReleaseHeavyMemory has no observable result to assert — forcing a GC is the
// point — so the check is that neither side of the threshold panics.
func TestReleaseHeavyMemoryTakesBothBranches(t *testing.T) {
	ReleaseHeavyMemory(0)
	ReleaseHeavyMemory(50*1024*1024 + 1)
}
