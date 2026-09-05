package mediainfo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func isoTestBox(typ string, payload []byte) []byte {
	b := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(b[:4], mediaFixtureUint32(len(b)))
	copy(b[4:8], typ)
	copy(b[8:], payload)
	return b
}

func TestAnalyzeHEIFItemSpatialProperties(t *testing.T) {
	for _, tc := range []struct {
		name, brand, format, codec string
	}{
		{"photo.heic", "heic", "HEIC", "HEVC"},
		{"photo.avif", "avif", "AVIF", "AV1"},
		{"photo.heif", "mif1", "HEIF", "HEIF"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			ftyp := isoTestBox("ftyp", append(append([]byte(tc.brand), 0, 0, 0, 0), []byte("mif1")...))
			ispeData := make([]byte, 12)
			binary.BigEndian.PutUint32(ispeData[4:8], 4032)
			binary.BigEndian.PutUint32(ispeData[8:12], 3024)
			pixiData := []byte{0, 0, 0, 0, 3, 10, 10, 10}
			properties := isoTestBox("ipco", append(isoTestBox("ispe", ispeData), isoTestBox("pixi", pixiData)...))
			meta := isoTestBox("meta", append(make([]byte, 4), isoTestBox("iprp", properties)...))
			r, err := analyzeBytes(t, tc.name, append(ftyp, meta...), ModeFast)
			if err != nil {
				t.Fatal(err)
			}
			if r.General.Format != tc.format || len(r.Streams) != 1 {
				t.Fatalf("report = %#v", r)
			}
			s := r.Streams[0]
			if s.Format != tc.codec || s.Image.Width != 4032 || s.Image.Height != 3024 || s.Image.BitDepth != 10 {
				t.Fatalf("image stream = %#v", s)
			}
		})
	}
}

func rawTIFFFixture(cr2, dng bool) []byte {
	ifd := 8
	if cr2 {
		ifd = 16
	}
	entries := 5
	if dng {
		entries++
	}
	b := make([]byte, 256)
	copy(b, "II*\x00")
	binary.LittleEndian.PutUint32(b[4:8], uint32(ifd))
	if cr2 {
		copy(b[8:12], "CR\x02\x00")
		binary.LittleEndian.PutUint32(b[12:16], uint32(ifd))
	}
	binary.LittleEndian.PutUint16(b[ifd:ifd+2], uint16(entries))
	pos := ifd + 2
	put := func(tag, typ uint16, count uint32, value uint16) {
		binary.LittleEndian.PutUint16(b[pos:pos+2], tag)
		binary.LittleEndian.PutUint16(b[pos+2:pos+4], typ)
		binary.LittleEndian.PutUint32(b[pos+4:pos+8], count)
		if typ == 3 && count == 1 {
			binary.LittleEndian.PutUint16(b[pos+8:pos+10], value)
		} else {
			binary.LittleEndian.PutUint32(b[pos+8:pos+12], uint32(value))
		}
		pos += 12
	}
	put(0x0100, 4, 1, 6000)
	put(0x0101, 4, 1, 4000)
	put(0x0102, 3, 1, 14)
	put(0x0103, 3, 1, 7)
	makeOffset := uint16(180)
	copy(b[makeOffset:], "Camera Co\x00")
	put(0x010f, 2, uint32(len("Camera Co\x00")), makeOffset)
	if dng {
		put(0xc612, 1, 4, 0x00000401)
	}
	return b
}

func TestAnalyzeTIFFBasedCameraRAW(t *testing.T) {
	for _, tc := range []struct {
		name, format string
		cr2, dng     bool
	}{
		{"photo.dng", "DNG", false, true},
		{"photo.cr2", "Canon CR2", true, false},
		{"photo.nef", "Nikon NEF", false, false},
		{"photo.pef", "Pentax PEF", false, false},
		{"photo.arw", "Sony ARW", false, false},
		{"photo.tiff", "TIFF", false, false},
	} {
		t.Run(tc.format, func(t *testing.T) {
			r, err := analyzeBytes(t, tc.name, rawTIFFFixture(tc.cr2, tc.dng), ModeFast)
			if err != nil {
				t.Fatal(err)
			}
			im := r.Streams[0].Image
			if r.General.Format != tc.format || im.Width != 6000 || im.Height != 4000 || im.BitDepth != 14 || im.CameraMake != "Camera Co" {
				t.Fatalf("report = %#v", r)
			}
		})
	}
}

func pngTestChunk(typ string, data []byte) []byte {
	b := make([]byte, 12+len(data))
	binary.BigEndian.PutUint32(b[:4], mediaFixtureUint32(len(data)))
	copy(b[4:8], typ)
	copy(b[8:], data)
	binary.BigEndian.PutUint32(b[8+len(data):], crc32.ChecksumIEEE(b[4:8+len(data)]))
	return b
}

func TestAnalyzeAPNGFrameCountAndDuration(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	b := encoded.Bytes()
	ihdrEnd := 8 + 12 + int(binary.BigEndian.Uint32(b[8:12]))
	actl := make([]byte, 8)
	binary.BigEndian.PutUint32(actl[:4], 2)
	fctl := func(sequence uint32, numerator, denominator uint16) []byte {
		d := make([]byte, 26)
		binary.BigEndian.PutUint32(d[:4], sequence)
		binary.BigEndian.PutUint32(d[4:8], 4)
		binary.BigEndian.PutUint32(d[8:12], 3)
		binary.BigEndian.PutUint16(d[20:22], numerator)
		binary.BigEndian.PutUint16(d[22:24], denominator)
		return pngTestChunk("fcTL", d)
	}
	extra := append(pngTestChunk("acTL", actl), fctl(0, 1, 10)...)
	extra = append(extra, fctl(1, 2, 10)...)
	apng := append(append(append([]byte(nil), b[:ihdrEnd]...), extra...), b[ihdrEnd:]...)
	r, err := analyzeBytes(t, "animated.png", apng, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	im := r.Streams[0].Image
	if !im.Animated || im.FrameCount != 2 || im.AnimationDuration != 300*time.Millisecond {
		t.Fatalf("APNG metadata = %#v", im)
	}
}

func TestAnalyzeGIFSkipsGlobalPalette(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second.SetColorIndex(0, 0, 1)
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{10, 20}}); err != nil {
		t.Fatal(err)
	}
	r, err := analyzeBytes(t, "animated.gif", b.Bytes(), ModeDetailed)
	if err != nil {
		t.Fatal(err)
	}
	im := r.Streams[0].Image
	if im.FrameCount != 2 || !im.Animated || im.AnimationDuration != 300*time.Millisecond {
		t.Fatalf("GIF metadata = %#v", im)
	}
}

func riffTestChunk(typ string, data []byte) []byte {
	b := make([]byte, 8+len(data)+(len(data)&1))
	copy(b, typ)
	binary.LittleEndian.PutUint32(b[4:8], mediaFixtureUint32(len(data)))
	copy(b[8:], data)
	return b
}

func TestWebPAnimationCountAndDuration(t *testing.T) {
	vp8x := make([]byte, 10)
	vp8x[0] = 2
	vp8x[4], vp8x[7] = 3, 2
	frame := func(ms int) []byte {
		d := make([]byte, 16)
		d[12], d[13], d[14] = mediaFixtureByte(ms), mediaFixtureByte(ms>>8), mediaFixtureByte(ms>>16)
		return riffTestChunk("ANMF", d)
	}
	payload := append(riffTestChunk("VP8X", vp8x), frame(100)...)
	payload = append(payload, frame(250)...)
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
	if im.FrameCount != 2 || im.AnimationDuration != 350*time.Millisecond || im.Width != 4 || im.Height != 3 {
		t.Fatalf("WebP metadata = %#v", im)
	}
}

func TestAnalyzeWebPDispatchesBeforeGenericRIFF(t *testing.T) {
	b, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	r, err := analyzeBytes(t, "pixel.webp", b, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if r.General.Format != "WEBP" || len(r.Streams) != 1 || r.Streams[0].Image.Width != 1 || r.Streams[0].Image.Height != 1 {
		t.Fatalf("WebP report = %#v", r)
	}
}

func TestCommonWebVTTTimestampWithoutHours(t *testing.T) {
	r, err := analyzeBytes(t, "captions.vtt", []byte("WEBVTT\n\n00:01.000 --> 00:02.500\nHello\n"), ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	x := r.Streams[0].Text
	if x.CueCount != 1 || x.FirstCue != time.Second || x.LastCue != 2500*time.Millisecond {
		t.Fatalf("WebVTT metadata = %#v", x)
	}
}

func TestMicroDVDFPSDeclaration(t *testing.T) {
	r, err := analyzeBytes(t, "captions.sub", []byte("{1}{1}24.0\n{24}{48}Hello\n"), ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	x := r.Streams[0].Text
	if x.CueCount != 1 || x.FirstCue != time.Second || x.LastCue != 2*time.Second {
		t.Fatalf("MicroDVD metadata = %#v", x)
	}
}

func TestAnalyzeEBUSTL(t *testing.T) {
	b := bytes.Repeat([]byte{' '}, 1024+128)
	copy(b[3:11], "STL25.01")
	copy(b[12:14], "01")
	copy(b[14:16], "09")
	copy(b[16:48], "Programme")
	copy(b[48:80], "Episode")
	copy(b[238:243], "00001")
	tti := b[1024:]
	tti[0] = 1
	binary.BigEndian.PutUint16(tti[1:3], 7)
	tti[3] = 0xff
	copy(tti[5:9], []byte{0, 0, 1, 12})
	copy(tti[9:13], []byte{0, 0, 3, 0})
	tti[15] = 0
	r, err := analyzeBytes(t, "captions.stl", b, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	x := r.Streams[0].Text
	if r.General.Format != "EBU STL" || x.Encoding != "ISO 8859-5 Cyrillic" || x.CueCount != 1 || x.FirstCue != 1480*time.Millisecond || x.LastCue != 3*time.Second {
		t.Fatalf("EBU STL report = %#v", r)
	}
}

func TestCleanSTLFieldOnlyRemovesDefinedPaddingBytes(t *testing.T) {
	want := string([]byte{'A', 0x8e})
	if got := cleanSTLField([]byte{'A', 0x8e, 0x8f, 0}); got != want {
		t.Fatalf("cleanSTLField() = %q, want %q", got, want)
	}
}

func TestAnalyzeOggFLAC(t *testing.T) {
	streamInfo := make([]byte, 34)
	v := uint64(48000)<<44 | uint64(1)<<41 | uint64(23)<<36 | uint64(96000)
	binary.BigEndian.PutUint64(streamInfo[10:18], v)
	header := append([]byte{0x7f}, []byte("FLAC")...)
	header = append(header, 1, 0, 0, 1)
	header = append(header, []byte("fLaC")...)
	header = append(header, 0, 0, 0, 34)
	header = append(header, streamInfo...)
	data := append(oggPage(11, 0, 0, header), oggPage(11, 96000, 1, nil)...)
	r, err := analyzeBytes(t, "tone.oga", data, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Streams[0]
	if s.Format != "FLAC" || s.Audio.SampleRate != 48000 || s.Audio.Channels != 2 || s.Audio.BitDepth != 24 || s.Duration != 2*time.Second {
		t.Fatalf("Ogg FLAC stream = %#v", s)
	}
}

func TestAnalyzeOggTheoraIdentification(t *testing.T) {
	header := make([]byte, 42)
	header[0] = 0x80
	copy(header[1:7], "theora")
	header[7], header[8], header[9] = 3, 2, 1
	header[14], header[15], header[16] = 0, 2, 128 // 640
	header[17], header[18], header[19] = 0, 1, 224 // 480
	binary.BigEndian.PutUint32(header[22:26], 30000)
	binary.BigEndian.PutUint32(header[26:30], 1001)
	r, err := analyzeBytes(t, "video.ogv", oggPage(9, 0, 0, header), ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Streams[0]
	if s.Format != "Theora" || s.Video.Width != 640 || s.Video.Height != 480 || math.Abs(s.FrameRate-29.970) > 0.001 || !strings.Contains(s.Profile, "3.2.1") {
		t.Fatalf("Theora stream = %#v", s)
	}
}

func TestAnalyzeRF64UsesDS64DataSize(t *testing.T) {
	b := make([]byte, 12+8+28+8+16+8+8000)
	copy(b[:4], "RF64")
	binary.LittleEndian.PutUint32(b[4:8], ^uint32(0))
	copy(b[8:12], "WAVE")
	copy(b[12:16], "ds64")
	binary.LittleEndian.PutUint32(b[16:20], 28)
	binary.LittleEndian.PutUint64(b[20:28], mediaFixtureUint64(len(b)-8))
	binary.LittleEndian.PutUint64(b[28:36], 8000)
	binary.LittleEndian.PutUint64(b[36:44], 8000)
	copy(b[48:52], "fmt ")
	binary.LittleEndian.PutUint32(b[52:56], 16)
	binary.LittleEndian.PutUint16(b[56:58], 1)
	binary.LittleEndian.PutUint16(b[58:60], 1)
	binary.LittleEndian.PutUint32(b[60:64], 8000)
	binary.LittleEndian.PutUint32(b[64:68], 8000)
	binary.LittleEndian.PutUint16(b[68:70], 1)
	binary.LittleEndian.PutUint16(b[70:72], 8)
	copy(b[72:76], "data")
	binary.LittleEndian.PutUint32(b[76:80], ^uint32(0))
	r, err := analyzeBytes(t, "long.rf64", b, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if r.General.CodecID != "RF64" || r.General.Duration != time.Second {
		t.Fatalf("RF64 report = %#v", r)
	}
}

func TestAnalyzeOpenDMLFrameCount(t *testing.T) {
	chunk := func(id string, payload []byte) []byte {
		b := make([]byte, 8+len(payload)+(len(payload)&1))
		copy(b, id)
		binary.LittleEndian.PutUint32(b[4:8], mediaFixtureUint32(len(payload)))
		copy(b[8:], payload)
		return b
	}
	avih := make([]byte, 40)
	binary.LittleEndian.PutUint32(avih[:4], 40000)
	// Legacy frame count is zero; OpenDML dmlh carries the real count.
	odml := append([]byte("odml"), chunk("dmlh", []byte{100, 0, 0, 0})...)
	payload := append(chunk("avih", avih), chunk("LIST", odml)...)
	b := make([]byte, 12)
	copy(b, "RIFF")
	copy(b[8:], "AVI ")
	b = append(b, payload...)
	binary.LittleEndian.PutUint32(b[4:8], mediaFixtureUint32(len(b)-8))
	r, err := analyzeBytes(t, "open-dml.avi", b, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if r.General.FrameCount != 100 || r.General.Duration != 4*time.Second {
		t.Fatalf("OpenDML report = %#v", r)
	}
}

func TestAnalyze3GPP2Brand(t *testing.T) {
	ftyp := isoTestBox("ftyp", append([]byte("3g2a\x00\x00\x00\x00"), []byte("isom")...))
	r, err := analyzeBytes(t, "phone.3g2", ftyp, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if r.General.Format != "3GPP2" || r.General.MIME != "video/3gpp2" {
		t.Fatalf("3GPP2 general = %#v", r.General)
	}
}

func TestAnalyzeITunesRawCopyrightAtom(t *testing.T) {
	ftyp := isoTestBox("ftyp", []byte("M4A \x00\x00\x00\x00isom"))
	dataPayload := append(make([]byte, 8), []byte("Track title")...)
	rawTag := isoTestBox(string([]byte{0xa9, 'n', 'a', 'm'}), isoTestBox("data", dataPayload))
	ilst := isoTestBox("ilst", rawTag)
	meta := isoTestBox("meta", append(make([]byte, 4), ilst...))
	moov := isoTestBox("moov", isoTestBox("udta", meta))
	r, err := analyzeBytes(t, "song.m4a", append(ftyp, moov...), ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tags) != 1 || r.Tags[0].Name != "Title" || r.Tags[0].Value != "Track title" {
		t.Fatalf("iTunes tags = %#v", r.Tags)
	}
}

func TestAnalyzeNativeMappedOggFLAC(t *testing.T) {
	streamInfo := make([]byte, 34)
	v := uint64(44100)<<44 | uint64(1)<<41 | uint64(15)<<36 | uint64(44100)
	binary.BigEndian.PutUint64(streamInfo[10:18], v)
	metadata := append([]byte{0, 0, 0, 34}, streamInfo...)
	data := append(oggPage(12, 0, 0, []byte("fLaC")), oggPage(12, 44100, 1, metadata)...)
	r, err := analyzeBytes(t, "native.oga", data, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Streams[0]
	if s.Format != "FLAC" || s.Audio.SampleRate != 44100 || s.Audio.BitDepth != 16 || s.Duration != time.Second {
		t.Fatalf("native Ogg FLAC stream = %#v", s)
	}
}

type cancelOnReadSource struct {
	data   []byte
	cancel context.CancelFunc
	reads  int
}

func (s *cancelOnReadSource) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.reads++
	if off < 0 || off >= int64(len(s.data)) {
		return 0, context.Canceled
	}
	n := copy(p, s.data[off:])
	if s.reads == 2 {
		s.cancel()
	}
	if n < len(p) {
		return n, nil
	}
	return n, nil
}

func TestAnalyzeCancellationWinsAfterBestEffortMetadata(t *testing.T) {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	src := &cancelOnReadSource{data: encoded.Bytes(), cancel: cancel}
	_, err := Analyze(ctx, Source{Name: "photo.jpg", Size: int64(len(src.data)), Reader: src}, DefaultOptions(ModeFast))
	if err != context.Canceled {
		t.Fatalf("error = %v, reads = %d", err, src.reads)
	}
}

func TestMP4TrackHeaderGeometrySurvivesHandler(t *testing.T) {
	b := make([]byte, 140)
	// Version-0 tkhd matrix and 16.16 display dimensions.
	binary.BigEndian.PutUint32(b[40:44], 0)
	binary.BigEndian.PutUint32(b[44:48], 0x10000)
	binary.BigEndian.PutUint32(b[76:80], 640<<16)
	binary.BigEndian.PutUint32(b[80:84], 480<<16)
	copy(b[108:112], "vide")
	p, err := newProbe(context.Background(), Source{Name: "x.mp4", Size: int64(len(b)), Reader: memorySource(b)}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	s := &Stream{}
	parseTKHD(p, isoBox{data: 0, payload: 84}, s)
	parseHDLR(p, isoBox{data: 100, payload: 32}, s)
	if s.Kind != StreamVideo || s.Video == nil || s.Video.DisplayWidth != 640 || s.Video.DisplayHeight != 480 || s.Video.Rotation != 90 {
		t.Fatalf("MP4 stream = %#v", s)
	}
}

func TestMatroskaTagTargetReachesSiblingSimpleTag(t *testing.T) {
	doc := ebmlElement([]byte{0x42, 0x82}, []byte("matroska"))
	header := ebmlElement([]byte{0x1a, 0x45, 0xdf, 0xa3}, doc)
	targets := ebmlElement([]byte{0x63, 0xc0}, ebmlElement([]byte{0x63, 0xc5}, []byte{42}))
	simple := ebmlElement([]byte{0x67, 0xc8}, append(
		ebmlElement([]byte{0x45, 0xa3}, []byte("TITLE")),
		ebmlElement([]byte{0x44, 0x87}, []byte("Track title"))...,
	))
	tag := ebmlElement([]byte{0x73, 0x73}, append(targets, simple...))
	tags := ebmlElement([]byte{0x12, 0x54, 0xc3, 0x67}, tag)
	segment := ebmlElement([]byte{0x18, 0x53, 0x80, 0x67}, tags)
	r, err := analyzeBytes(t, "tagged.mkv", append(header, segment...), ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tags) != 1 || r.Tags[0].Target != "42" || r.Tags[0].Name != "Title" || r.Tags[0].Value != "Track title" {
		t.Fatalf("Matroska tags = %#v", r.Tags)
	}
}

func TestAVIRejectedStreamDoesNotReusePreviousSTRF(t *testing.T) {
	chunk := func(id string, payload []byte) []byte {
		b := make([]byte, 8+len(payload)+(len(payload)&1))
		copy(b, id)
		binary.LittleEndian.PutUint32(b[4:8], mediaFixtureUint32(len(payload)))
		copy(b[8:], payload)
		return b
	}
	strh := make([]byte, 48)
	copy(strh[:4], "vids")
	copy(strh[4:8], "H264")
	strf := func(width uint32) []byte {
		b := make([]byte, 40)
		binary.LittleEndian.PutUint32(b[:4], 40)
		binary.LittleEndian.PutUint32(b[4:8], width)
		binary.LittleEndian.PutUint32(b[8:12], 20)
		binary.LittleEndian.PutUint16(b[14:16], 24)
		copy(b[16:20], "H264")
		return b
	}
	payload := append(chunk("strh", strh), chunk("strf", strf(10))...)
	payload = append(payload, chunk("strh", strh)...)
	payload = append(payload, chunk("strf", strf(999))...)
	b := make([]byte, 12)
	copy(b, "RIFF")
	copy(b[8:], "AVI ")
	b = append(b, payload...)
	binary.LittleEndian.PutUint32(b[4:8], mediaFixtureUint32(len(b)-8))
	opts := DefaultOptions(ModeFast)
	opts.MaxStreams = 1
	r, err := Analyze(context.Background(), Source{Name: "limited.avi", Size: int64(len(b)), Reader: memorySource(b)}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Streams) != 1 || r.Streams[0].Video.Width != 10 || !r.Truncated {
		t.Fatalf("AVI report = %#v", r)
	}
}

func TestTIFFMetadataWalkIsElementAndTextBounded(t *testing.T) {
	const entries = 4096
	ifdBytes := 2 + entries*12 + 4
	sharedOffset := 8 + ifdBytes
	b := make([]byte, sharedOffset+(8<<20))
	copy(b, "II*\x00")
	binary.LittleEndian.PutUint32(b[4:8], 8)
	binary.LittleEndian.PutUint16(b[8:10], entries)
	for i := 0; i < entries; i++ {
		pos := 10 + i*12
		binary.LittleEndian.PutUint16(b[pos:pos+2], 0xeeee) // unsupported tag
		binary.LittleEndian.PutUint16(b[pos+2:pos+4], 2)
		binary.LittleEndian.PutUint32(b[pos+4:pos+8], 8<<20)
		binary.LittleEndian.PutUint32(b[pos+8:pos+12], uint32(sharedOffset))
	}
	opts := DefaultOptions(ModeFast)
	opts.MaxElements = 20
	p, err := newProbe(context.Background(), Source{Name: "hostile.tiff", Size: 1, Reader: memorySource{0}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	parseTIFFMeta(p, b, &Image{})
	if !p.report.Truncated || p.elements != 21 {
		t.Fatalf("TIFF walk elements=%d truncated=%v", p.elements, p.report.Truncated)
	}

	textTIFF := make([]byte, 128)
	copy(textTIFF, "II*\x00")
	binary.LittleEndian.PutUint32(textTIFF[4:8], 8)
	binary.LittleEndian.PutUint16(textTIFF[8:10], 1)
	binary.LittleEndian.PutUint16(textTIFF[10:12], 0x010f)
	binary.LittleEndian.PutUint16(textTIFF[12:14], 2)
	binary.LittleEndian.PutUint32(textTIFF[14:18], 64)
	binary.LittleEndian.PutUint32(textTIFF[18:22], 32)
	copy(textTIFF[32:96], strings.Repeat("A", 64))
	opts.MaxElements, opts.MaxValueBytes = 100, 8
	p, err = newProbe(context.Background(), Source{Name: "text.tiff", Size: 1, Reader: memorySource{0}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	im := &Image{}
	parseTIFFMeta(p, textTIFF, im)
	if im.CameraMake != "AAAAAAAA" || !p.report.Truncated {
		t.Fatalf("camera make=%q truncated=%v", im.CameraMake, p.report.Truncated)
	}
}

type countingMemorySource struct {
	data  []byte
	bytes int64
}

func (s *countingMemorySource) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off < 0 || off >= int64(len(s.data)) {
		return 0, context.Canceled
	}
	n := copy(p, s.data[off:])
	s.bytes += int64(n)
	return n, nil
}

func TestHEIFFTYPBrandListAndReadAreBounded(t *testing.T) {
	ftypPayload := bytes.Repeat([]byte("junk"), (1<<20)/4)
	copy(ftypPayload[:4], "avif")
	clear(ftypPayload[4:8])
	ftyp := isoTestBox("ftyp", ftypPayload)
	ispe := make([]byte, 12)
	binary.BigEndian.PutUint32(ispe[4:8], 16)
	binary.BigEndian.PutUint32(ispe[8:12], 9)
	properties := isoTestBox("ipco", isoTestBox("ispe", ispe))
	meta := isoTestBox("meta", append(make([]byte, 4), isoTestBox("iprp", properties)...))
	source := &countingMemorySource{data: append(ftyp, meta...)}
	r, err := Analyze(context.Background(), Source{Name: "hostile.avif", Size: int64(len(source.data)), Reader: source}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.General.CompatibleBrands) > 64 || source.bytes > 128<<10 {
		t.Fatalf("brands=%d bytes read=%d", len(r.General.CompatibleBrands), source.bytes)
	}
}

func TestAddTagOwnsAcceptedValue(t *testing.T) {
	large := strings.Repeat("0123456789", 1<<17)
	target := large[12000:12004]
	name := large[12300:12301]
	value := large[12345:12353]
	p, err := newProbe(context.Background(), Source{Name: "x", Size: 1, Reader: memorySource{0}}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	p.addTag(target, name, value)
	got := p.report.Tags[0]
	if got.Target != target || got.Name != name || got.Value != value ||
		unsafe.StringData(got.Target) == unsafe.StringData(target) || // #nosec G103 -- pointer identity is required to prove the accepted string owns independent backing storage.
		unsafe.StringData(got.Name) == unsafe.StringData(name) || // #nosec G103 -- pointer identity is required to prove the accepted string owns independent backing storage.
		unsafe.StringData(got.Value) == unsafe.StringData(value) { // #nosec G103 -- pointer identity is required to prove the accepted string owns independent backing storage.
		t.Fatalf("accepted field did not own its strings: %#v", got)
	}
	p.addTag("", strings.Repeat("N", 2048), "value")
	if len(p.report.Tags[1].Name) != 1024 || !p.report.Truncated {
		t.Fatalf("tag name length=%d truncated=%v", len(p.report.Tags[1].Name), p.report.Truncated)
	}
}

func TestCleanTextCompactsLargePadding(t *testing.T) {
	b := make([]byte, 8<<20)
	b[0] = 'X'
	for i := 1; i < len(b); i++ {
		b[i] = ' '
	}
	if got := cleanText(b); got != "X" {
		t.Fatalf("cleanText = %q", got)
	}
	utf16Bytes := []byte{' ', 0, 'X', 0, ' ', 0}
	if got := decodeUTF16(utf16Bytes, true); got != "X" {
		t.Fatalf("decodeUTF16 = %q", got)
	}
}

func BenchmarkCleanTextLargePadding(b *testing.B) {
	data := make([]byte, 8<<20)
	data[0] = 'X'
	for i := 1; i < len(data); i++ {
		data[i] = ' '
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := cleanText(data); got != "X" {
			b.Fatal(got)
		}
	}
}
