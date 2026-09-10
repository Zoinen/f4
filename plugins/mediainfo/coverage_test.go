package mediainfo

import (
	"encoding/binary"
	"reflect"
	"testing"
	"time"
)

func TestUtilityBinaryAndDurationHelpers(t *testing.T) {
	if u16([]byte{1, 2}, binary.LittleEndian) != 0x0201 || u16([]byte{1}, binary.LittleEndian) != 0 {
		t.Fatal("u16 boundary handling failed")
	}
	if u32([]byte{1, 2, 3, 4}, binary.BigEndian) != 0x01020304 || u32(nil, binary.BigEndian) != 0 {
		t.Fatal("u32 boundary handling failed")
	}
	if u64([]byte{1, 2, 3, 4, 5, 6, 7, 8}, binary.BigEndian) != 0x0102030405060708 || u64([]byte{1}, binary.BigEndian) != 0 {
		t.Fatal("u64 boundary handling failed")
	}
	if fourCC([]byte("abcd")) != "abcd" || fourCC([]byte("ab  ")) != "ab" {
		t.Fatal("printable fourCC handling failed")
	}
	if got := fourCC([]byte{0xff, 0, 0, 1}); got != "0xFF000001" {
		t.Fatalf("binary fourCC = %q", got)
	}
	if got := cleanText([]byte("  value \x00\x00")); got != "value" {
		t.Fatalf("cleanText = %q", got)
	}
	if durationFromUnits(2, 1) != 2*time.Second || durationFromUnits(0, 1) != 0 || durationFromUnits(1, 0) != 0 {
		t.Fatal("durationFromUnits basic cases failed")
	}
	if durationFromUnits(^uint64(0), 1) != 0 {
		t.Fatal("durationFromUnits overflow was not rejected")
	}
	if got, ok := metadataInt(12); !ok || got != 12 {
		t.Fatalf("metadataInt = (%d, %t)", got, ok)
	}
	if _, ok := metadataInt(uint64(^uint(0)>>1) + 1); ok {
		t.Fatal("metadataInt overflow was accepted")
	}
	if got, ok := metadataDuration(7); !ok || got != 7 {
		t.Fatalf("metadataDuration = (%v, %t)", got, ok)
	}
	if _, ok := metadataDuration(^uint64(0)); ok {
		t.Fatal("metadataDuration overflow was accepted")
	}
	if signedInt32Bits(0xffffffff) != -1 {
		t.Fatal("signedInt32Bits did not preserve two's-complement bits")
	}
}

func TestUtilityTextAndTagHelpers(t *testing.T) {
	if parseISO639(0) != "" || parseISO639(0xffff) != "" {
		t.Fatal("invalid ISO-639 values were accepted")
	}
	if got := parseISO639(uint16(5<<10 | 14<<5 | 7)); got != "eng" {
		t.Fatalf("parseISO639 = %q", got)
	}
	if !*boolPtr(true) {
		t.Fatal("boolPtr lost its value")
	}
	little := []byte{0x20, 0, 'h', 0, 'i', 0, 0, 0, 0x20, 0}
	if got := decodeUTF16(little, true); got != "hi" {
		t.Fatalf("little UTF-16 = %q", got)
	}
	big := []byte{0, 0x20, 0, 'h', 0, 'i', 0, 0, 0, 0x20, 0xff}
	if got := decodeUTF16(big, false); got != "hi" {
		t.Fatalf("big UTF-16 = %q", got)
	}
	if parseDecimal(" 12.5 ") != 12.5 || parseDecimal("not a number") != 0 {
		t.Fatal("parseDecimal cases failed")
	}
	if canonicalTag("INAM") != "Title" || canonicalTag("©ART") != "Artist" || canonicalTag("TRCK") != "Track" {
		t.Fatal("known canonical tags were not mapped")
	}
	if canonicalTag(" custom ") != "custom" {
		t.Fatal("unknown canonical tag was not trimmed")
	}
	if !reflect.DeepEqual([]byte(decodeUTF16([]byte{0x41, 0}, true)), []byte("A")) {
		t.Fatal("decodeUTF16 single character failed")
	}
}
