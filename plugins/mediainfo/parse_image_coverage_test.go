package mediainfo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
	"time"
)

// minimalTIFFCameraMake builds the smallest valid little-endian TIFF byte
// stream that carries a single ASCII CameraMake (0x010f) tag, for embedding
// as EXIF payload inside JPEG/PNG/WebP containers.
func minimalTIFFCameraMake(cameraMake string) []byte {
	strOff := 26
	strBytes := append([]byte(cameraMake), 0)
	b := make([]byte, strOff+len(strBytes))
	copy(b, "II*\x00")
	binary.LittleEndian.PutUint32(b[4:8], 8)
	binary.LittleEndian.PutUint16(b[8:10], 1) // one IFD entry
	binary.LittleEndian.PutUint16(b[10:12], 0x010f)
	binary.LittleEndian.PutUint16(b[12:14], 2) // ASCII
	binary.LittleEndian.PutUint32(b[14:18], mediaFixtureUint32(len(strBytes)))
	binary.LittleEndian.PutUint32(b[18:22], mediaFixtureUint32(strOff))
	// b[22:26] is the "next IFD offset" field, left zero.
	copy(b[strOff:], strBytes)
	return b
}

func jpegSegment(marker byte, payload []byte) []byte {
	b := make([]byte, 4+len(payload))
	b[0] = 0xff
	b[1] = marker
	// #nosec G115 -- test fixture segment sizes are small constants, well under uint16's range.
	binary.BigEndian.PutUint16(b[2:4], uint16(len(payload)+2))
	copy(b[4:], payload)
	return b
}

// TestParseImageDecodeConfigError covers parseImage's early return when the
// payload carries recognizable image magic bytes but the body itself is not
// a decodable image (image.DecodeConfig fails).
func TestParseImageDecodeConfigError(t *testing.T) {
	b := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...) // PNG magic, garbage body
	_, err := analyzeBytes(t, "broken.png", b, ModeFast)
	if err == nil {
		t.Fatal("expected a decode error for a corrupt PNG body")
	}
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Format != "Image" {
		t.Fatalf("err = %v, want *ParseError with Format=Image", err)
	}
}

// TestParseJPEGMetaExifXMPAndIPTC exercises parseJPEGMeta's APP1 (Exif and
// XMP) and APP13 (IPTC) marker handling, none of which any existing test
// drove through a real JPEG payload.
func TestParseJPEGMetaExifXMPAndIPTC(t *testing.T) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewGray(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	base := encoded.Bytes()
	if len(base) < 2 || base[0] != 0xff || base[1] != 0xd8 {
		t.Fatalf("unexpected JPEG header: %x", base[:2])
	}

	exifPayload := append([]byte("Exif\x00\x00"), minimalTIFFCameraMake("Canon")...)
	xmpPayload := []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta/>")

	inserted := append(jpegSegment(0xe1, exifPayload), jpegSegment(0xe1, xmpPayload)...)
	inserted = append(inserted, jpegSegment(0xed, []byte{0})...)

	full := append([]byte{0xff, 0xd8}, inserted...)
	full = append(full, base[2:]...)

	r, err := analyzeBytes(t, "photo.jpg", full, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if r.General.Format != "JPEG" || len(r.Streams) != 1 {
		t.Fatalf("report = %#v", r)
	}
	im := r.Streams[0].Image
	if im == nil || im.CameraMake != "Canon" {
		t.Fatalf("Exif CameraMake not decoded: %#v", im)
	}
	tags := map[string]string{}
	for _, f := range r.Tags {
		tags[f.Name] = f.Value
	}
	if tags["XMP"] != "Present" || tags["IPTC"] != "Present" {
		t.Fatalf("tags = %#v, want XMP and IPTC present", r.Tags)
	}
}

// TestParseJPEGMetaStopsOnTruncatedSegment covers the defensive bounds check
// that stops the marker scan when a declared segment size would run past
// the end of the source, instead of reading out of bounds.
func TestParseJPEGMetaStopsOnTruncatedSegment(t *testing.T) {
	// SOI, then an APP1 marker claiming a size far larger than the buffer.
	b := []byte{0xff, 0xd8, 0xff, 0xe1, 0x7f, 0xff}
	p, err := newProbe(context.Background(), Source{Name: "x.jpg", Size: int64(len(b)), Reader: memorySource(b)}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	im := &Image{}
	parseJPEGMeta(p, im) // must return quietly, not panic or read out of bounds.
	if len(p.report.Tags) != 0 {
		t.Fatalf("no tags expected from a truncated segment, got %#v", p.report.Tags)
	}
}

// TestParsePNGMetaPhysExifTextAndZeroDenominatorFCTL exercises the pHYs,
// eXIf and tEXt chunk branches of parsePNGMeta, plus fcTL's zero-denominator
// fallback to 100 — none of which any existing PNG test reached (previous
// coverage only drove IHDR/acTL/fcTL-with-a-real-denominator/IEND).
func TestParsePNGMetaPhysExifTextAndZeroDenominatorFCTL(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	b := encoded.Bytes()
	ihdrEnd := 8 + 12 + int(binary.BigEndian.Uint32(b[8:12]))

	phys := make([]byte, 9)
	binary.BigEndian.PutUint32(phys[:4], 1000)  // 1000 px/m => 25.4 DPI
	binary.BigEndian.PutUint32(phys[4:8], 2000) // 2000 px/m => 50.8 DPI
	phys[8] = 1                                 // unit: meter

	exif := minimalTIFFCameraMake("Nikon")

	text := []byte("Comment\x00Hello World")

	fctl := make([]byte, 26)
	binary.BigEndian.PutUint16(fctl[20:22], 5) // numerator
	binary.BigEndian.PutUint16(fctl[22:24], 0) // denominator 0 -> defaults to 100

	extra := append(pngTestChunk("pHYs", phys), pngTestChunk("eXIf", exif)...)
	extra = append(extra, pngTestChunk("tEXt", text)...)
	extra = append(extra, pngTestChunk("fcTL", fctl)...)

	combined := append(append(append([]byte(nil), b[:ihdrEnd]...), extra...), b[ihdrEnd:]...)

	r, err := analyzeBytes(t, "photo.png", combined, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	im := r.Streams[0].Image
	if im.DPIX != 25.4 || im.DPIY != 50.8 {
		t.Fatalf("pHYs DPI = %v/%v, want 25.4/50.8", im.DPIX, im.DPIY)
	}
	if im.CameraMake != "Nikon" {
		t.Fatalf("eXIf CameraMake = %q, want Nikon", im.CameraMake)
	}
	if im.AnimationDuration != 50*time.Millisecond {
		t.Fatalf("fcTL zero-denominator duration = %v, want 50ms", im.AnimationDuration)
	}
	found := false
	for _, f := range r.Tags {
		if f.Name == "Comment" && f.Value == "Hello World" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tEXt Comment tag missing: %#v", r.Tags)
	}
}

// TestParseWebPMetaExifXmpAndPadding exercises parseWebPMeta's EXIF and XMP
// chunk handling together with the odd-length chunk padding step, using an
// odd-length EXIF payload so the byte immediately after it only becomes
// reachable if the padding advance (pos++) actually runs.
func TestParseWebPMetaExifXmpAndPadding(t *testing.T) {
	exifPayload := append([]byte("Exif\x00\x00"), minimalTIFFCameraMake("Canon2")...)
	if len(exifPayload)%2 == 0 {
		t.Fatalf("test fixture bug: EXIF payload must be odd-length, got %d", len(exifPayload))
	}
	xmpPayload := []byte("<x:xmpmeta/>")

	payload := append(riffTestChunk("EXIF", exifPayload), riffTestChunk("XMP ", xmpPayload)...)
	file := make([]byte, 12)
	copy(file, "RIFF")
	copy(file[8:], "WEBP")
	file = append(file, payload...)
	binary.LittleEndian.PutUint32(file[4:8], mediaFixtureUint32(len(file)-8))

	p, err := newProbe(context.Background(), Source{Name: "x.webp", Size: int64(len(file)), Reader: memorySource(file)}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	im := &Image{FrameCount: 1}
	parseWebPMeta(p, im)
	if im.CameraMake != "Canon2" {
		t.Fatalf("WebP EXIF CameraMake = %q, want Canon2", im.CameraMake)
	}
	found := false
	for _, f := range p.report.Tags {
		if f.Name == "XMP" && f.Value == "Present" {
			found = true
		}
	}
	if !found {
		t.Fatalf("WebP XMP tag missing: %#v", p.report.Tags)
	}
}

// TestParseWebPMetaStaticImageDefaultsFrameCount covers the fallback branch
// that gives a still (non-animated) WebP a FrameCount of 1 when no ANMF
// chunk was ever seen — every prior WebP test starts from FrameCount:1 or
// includes real animation frames, so this branch was never reached.
func TestParseWebPMetaStaticImageDefaultsFrameCount(t *testing.T) {
	file := make([]byte, 12)
	copy(file, "RIFF")
	copy(file[8:], "WEBP")
	binary.LittleEndian.PutUint32(file[4:8], mediaFixtureUint32(len(file)-8))

	p, err := newProbe(context.Background(), Source{Name: "x.webp", Size: int64(len(file)), Reader: memorySource(file)}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	im := &Image{}
	parseWebPMeta(p, im)
	if im.FrameCount != 1 || im.Animated {
		t.Fatalf("static WebP image = %#v, want FrameCount=1 Animated=false", im)
	}
}

// TestParseGIFMetaStopsOnUnknownBlockIntroducer covers the default branch of
// parseGIFMeta's block-introducer switch: a byte that is none of image
// separator (0x2c), extension introducer (0x21) or trailer (0x3b) must stop
// the scan instead of looping forever or misreading the stream.
func TestParseGIFMetaStopsOnUnknownBlockIntroducer(t *testing.T) {
	b := make([]byte, 20)
	copy(b, "GIF89a")
	b[10] = 0    // no global color table
	b[13] = 0x99 // not 0x2c/0x21/0x3b

	p, err := newProbe(context.Background(), Source{Name: "x.gif", Size: int64(len(b)), Reader: memorySource(b)}, DefaultOptions(ModeDetailed))
	if err != nil {
		t.Fatal(err)
	}
	im := &Image{}
	parseGIFMeta(p, im)
	if im.FrameCount != 0 || im.Animated {
		t.Fatalf("malformed GIF must not report frames: %#v", im)
	}
}

// TestSkipGIFBlocksStopsOnReadError covers skipGIFBlocks's own defensive
// return when it is asked to keep walking sub-blocks past the end of the
// source: every existing GIF fixture only ever runs it over well-formed
// blocks that terminate normally.
func TestSkipGIFBlocksStopsOnReadError(t *testing.T) {
	b := make([]byte, 16)
	p, err := newProbe(context.Background(), Source{Name: "x.gif", Size: int64(len(b)), Reader: memorySource(b)}, DefaultOptions(ModeDetailed))
	if err != nil {
		t.Fatal(err)
	}
	limit := p.src.Size + 100
	if got := skipGIFBlocks(p, p.src.Size, limit); got != limit {
		t.Fatalf("skipGIFBlocks past EOF = %d, want limit %d", got, limit)
	}
}
