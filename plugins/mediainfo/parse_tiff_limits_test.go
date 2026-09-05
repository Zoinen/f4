package mediainfo

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

func sharedTIFFValueFixture(tag, typ uint16, valueCount uint32, entries uint16, value byte) []byte {
	unit := 4
	switch typ {
	case 1:
		unit = 1
	case 3:
		unit = 2
	}
	ifdSize := 2 + int(entries)*12 + 4
	payloadOffset := 8 + ifdSize
	b := make([]byte, payloadOffset+int(valueCount)*unit)
	copy(b, "II*\x00")
	binary.LittleEndian.PutUint32(b[4:8], 8)
	binary.LittleEndian.PutUint16(b[8:10], entries)
	for i := uint16(0); i < entries; i++ {
		pos := 10 + int(i)*12
		binary.LittleEndian.PutUint16(b[pos:pos+2], tag)
		binary.LittleEndian.PutUint16(b[pos+2:pos+4], typ)
		binary.LittleEndian.PutUint32(b[pos+4:pos+8], valueCount)
		binary.LittleEndian.PutUint32(b[pos+8:pos+12], mediaFixtureUint32(payloadOffset))
	}
	for i := uint32(0); i < valueCount; i++ {
		pos := payloadOffset + int(i)*unit
		switch typ {
		case 1:
			b[pos] = value
		case 3:
			binary.LittleEndian.PutUint16(b[pos:pos+2], uint16(value))
		default:
			binary.LittleEndian.PutUint32(b[pos:pos+4], uint32(value))
		}
	}
	return b
}

func scanTIFFFixture(ctx context.Context, b []byte, opts Options) (*probe, *tiffImageScanner, error) {
	p := &probe{ctx: ctx, opts: opts.normalized()}
	s := &tiffImageScanner{
		p: p, b: b, order: binary.LittleEndian,
		seen: make(map[uint32]bool), pending: make(map[uint32]bool),
	}
	return p, s, s.walkIFD(8, 0)
}

func TestTIFFScannerClassifiesBeforeSharedValueDecode(t *testing.T) {
	const entries = 4096
	// Every unknown entry claims the same 4096-value payload. Decoding before
	// checking the tag used to allocate and discard more than 16 million
	// uint32 values.
	b := sharedTIFFValueFixture(0xeeee, 4, 4096, entries, 0)
	opts := DefaultOptions(ModeDetailed)
	opts.MaxElements = entries + 16

	var elements int
	var scanErr error
	allocs := testing.AllocsPerRun(5, func() {
		p, _, err := scanTIFFFixture(context.Background(), b, opts)
		elements, scanErr = p.elements, err
	})
	if scanErr != nil {
		t.Fatal(scanErr)
	}
	if elements != entries {
		t.Fatalf("elements = %d, want %d", elements, entries)
	}
	if allocs > 128 {
		t.Fatalf("shared-value scan allocations = %.0f, want <= 128", allocs)
	}
}

func TestTIFFScannerDuplicateSubIFDsAreElementBounded(t *testing.T) {
	const entries = 4096
	// All references point back to the root. Duplicate references still consume
	// the element budget but never grow a duplicate child-offset slice.
	b := sharedTIFFValueFixture(0x014a, 4, 64, entries, 8)
	opts := DefaultOptions(ModeDetailed)
	opts.MaxElements = 1024

	var p *probe
	var scanner *tiffImageScanner
	var scanErr error
	allocs := testing.AllocsPerRun(5, func() {
		p, scanner, scanErr = scanTIFFFixture(context.Background(), b, opts)
	})
	if !errors.Is(scanErr, ErrLimit) {
		t.Fatalf("error = %v, want ErrLimit", scanErr)
	}
	if p.elements != opts.MaxElements+1 {
		t.Fatalf("elements = %d, want %d", p.elements, opts.MaxElements+1)
	}
	if len(scanner.pending) != 0 || len(scanner.seen) != 1 {
		t.Fatalf("IFD state: seen=%d pending=%d", len(scanner.seen), len(scanner.pending))
	}
	if allocs > 128 {
		t.Fatalf("duplicate-reference scan allocations = %.0f, want <= 128", allocs)
	}
}

func TestTIFFScannerCapsArrayValueCounts(t *testing.T) {
	tests := []struct {
		name  string
		tag   uint16
		typ   uint16
		count uint32
	}{
		{"bits per sample", 0x0102, 3, maxTIFFBitsPerSampleValues + 1},
		{"SubIFDs", 0x014a, 4, maxTIFFSubIFDReferenceCount + 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := sharedTIFFValueFixture(tc.tag, tc.typ, tc.count, 1, 8)
			p, _, err := scanTIFFFixture(context.Background(), b, DefaultOptions(ModeDetailed))
			if !errors.Is(err, ErrLimit) {
				t.Fatalf("error = %v, want ErrLimit", err)
			}
			if p.elements != 1 {
				t.Fatalf("elements = %d, want 1", p.elements)
			}
		})
	}
}

func TestTIFFScannerCancellationWinsForVisitedIFD(t *testing.T) {
	b := sharedTIFFValueFixture(0xeeee, 4, 1, 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	p, scanner, err := scanTIFFFixture(ctx, b, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := scanner.walkIFD(8, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled (elements=%d)", err, p.elements)
	}
}
