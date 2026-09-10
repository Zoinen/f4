package visren

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func TestMetadataTextDecodersCoverEncodingsAndFallbacks(t *testing.T) {
	if got := decodeID3Text(nil); got != "" {
		t.Fatalf("empty ID3 text = %q", got)
	}
	if got := decodeID3Text([]byte{3, ' ', 't', 'i', 't', 'l', 'e', 0}); got != "title" {
		t.Fatalf("UTF-8 ID3 text = %q", got)
	}
	if got := decodeID3Text([]byte{0, ' ', 'l', 'e', 'g', 'a', 'c', 'y', ' '}); got != "legacy" {
		t.Fatalf("legacy ID3 text = %q", got)
	}
	little := []byte{1, 'h', 0, 'i', 0, 0, 0}
	if got := decodeID3Text(little); got != "hi" {
		t.Fatalf("little-endian ID3 text = %q", got)
	}
	big := []byte{2, 0, 'h', 0, 'i', 0, 0}
	if got := decodeID3Text(big); got != "hi" {
		t.Fatalf("big-endian ID3 text = %q", got)
	}
	bom := []byte{1, 0xfe, 0xff, 0, 'h', 0, 'i'}
	if got := decodeID3Text(bom); got != "hi" {
		t.Fatalf("BOM-marked ID3 text = %q", got)
	}
	if got := decodeID3Text([]byte{9, 'x'}); got != "" {
		t.Fatalf("unknown ID3 encoding = %q", got)
	}
	if got := decodeID3Text([]byte{1, 'x'}); got != "" {
		t.Fatalf("truncated UTF-16 text = %q", got)
	}
}

func TestMetadataSmallDecodersHandleBoundaries(t *testing.T) {
	if got := syncSafe([]byte{0x7f, 0x7f, 0x7f, 0x7f}); got != 0x0fffffff {
		t.Fatalf("syncSafe max = %d", got)
	}
	if syncSafe([]byte{1, 2, 3}) != 0 {
		t.Fatal("short sync-safe value was accepted")
	}
	if got := removeUnsynchronisation([]byte{0xff, 0, 0xff, 1}); !reflect.DeepEqual(got, []byte{0xff, 0xff, 1}) {
		t.Fatalf("unsynchronised bytes = %x", got)
	}
	if got := decodeGenre("17"); got != "Rock" {
		t.Fatalf("numeric genre = %q", got)
	}
	if got := decodeGenre("(17)"); got != "Rock" {
		t.Fatalf("parenthesised genre = %q", got)
	}
	if got := decodeGenre("(17) Custom"); got != "Custom" {
		t.Fatalf("genre suffix = %q", got)
	}
	if got := decodeGenre("unknown"); got != "unknown" {
		t.Fatalf("unknown genre = %q", got)
	}
}

func TestFixedFileVersionRejectsMalformedAndZeroValues(t *testing.T) {
	if fixedFileVersion([]byte{1, 2, 3}) != "" {
		t.Fatal("short version data was accepted")
	}
	zero := make([]byte, 24)
	copy(zero[4:8], []byte{0xbd, 0x04, 0xef, 0xfe})
	if fixedFileVersion(zero) != "" {
		t.Fatal("zero file version was reported")
	}
	version := make([]byte, 24)
	copy(version[4:8], []byte{0xbd, 0x04, 0xef, 0xfe})
	binary.LittleEndian.PutUint32(version[12:16], uint32(5)<<16|6)
	binary.LittleEndian.PutUint32(version[16:20], uint32(7)<<16|8)
	if got := fixedFileVersion(version); got != "5.6.7.8" {
		t.Fatalf("file version = %q", got)
	}
}
