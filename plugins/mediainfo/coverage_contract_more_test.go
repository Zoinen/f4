package mediainfo

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func TestParserSelectionAndCodecContracts(t *testing.T) {
	tests := []struct {
		name string
		head []byte
		want bool
	}{
		{"ebu stl", nil, true},
		{"webp", []byte("RIFF0000WEBP"), true},
		{"riff", []byte("RIFF0000WAVE"), true},
		{"rifx", []byte("RIFX0000WAVE"), true},
		{"rf64", []byte("RF640000WAVE"), true},
		{"bw64", []byte("BW640000WAVE"), true},
		{"aiff", []byte("FORM0000AIFF"), true},
		{"aifc", []byte("FORM0000AIFC"), true},
		{"heif brand", append([]byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, 0, 0, 0, 0), true},
		{"iso", []byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, true},
		{"styp", []byte{0, 0, 0, 0, 's', 't', 'y', 'p', 'i', 's', 'o', 'm'}, true},
		{"moov", []byte{0, 0, 0, 0, 'm', 'o', 'o', 'v', 0, 0, 0, 0}, true},
		{"moof", []byte{0, 0, 0, 0, 'm', 'o', 'o', 'f', 0, 0, 0, 0}, true},
		{"ebml", []byte{0x1a, 0x45, 0xdf, 0xa3}, true},
		{"flac", []byte("fLaC"), true},
		{"ogg", []byte("OggS"), true},
		{"jpeg", []byte{0xff, 0xd8}, true},
		{"subtitle", []byte("WEBVTT\n"), true},
		{"unknown", []byte("nope"), false},
	}
	for _, tc := range tests {
		name := tc.name
		switch name {
		case "ebu stl":
			name = "captions.stl"
		case "subtitle":
			name = "captions.vtt"
		default:
			name = "sample.bin"
		}
		if got := chooseParser(name, tc.head) != nil; got != tc.want {
			t.Errorf("chooseParser(%q)=%v, want %v", tc.name, got, tc.want)
		}
	}

	waveCases := []struct {
		tag  uint16
		data []byte
		want string
	}{
		{0x0001, nil, "PCM"}, {0x0003, nil, "IEEE Float"}, {0x0006, nil, "A-law"},
		{0x0007, nil, "Mu-law"}, {0x0011, nil, "IMA ADPCM"}, {0x0050, nil, "MPEG Audio"},
		{0x0055, nil, "MPEG Audio Layer 3"}, {0x00ff, nil, "AAC"}, {0x0161, nil, "Windows Media Audio"},
		{0x0162, nil, "Windows Media Audio"}, {0x2000, nil, "AC-3"},
		{0xfffe, make([]byte, 40), "WaveFormatExtensible"}, {0xfffe, nil, "Extensible"}, {0x9999, nil, "Unknown"},
	}
	for _, tc := range waveCases {
		if got := waveCodec(tc.tag, tc.data); got != tc.want {
			t.Errorf("waveCodec(%#x)=%q, want %q", tc.tag, got, tc.want)
		}
	}
	for _, tc := range []struct{ id, want string }{
		{"h264", "AVC"}, {"x264", "AVC"}, {"avc1", "AVC"}, {"hevc", "HEVC"}, {"hvc1", "HEVC"},
		{"av01", "AV1"}, {"vp80", "VP8"}, {"vp90", "VP9"}, {"xvid", "MPEG-4 Visual"},
		{"mjpg", "Motion JPEG"}, {"theo", "Theora"}, {"", "Video"}, {"  custom  ", "  custom  "},
	} {
		if got := videoCodec(tc.id); got != tc.want {
			t.Errorf("videoCodec(%q)=%q, want %q", tc.id, got, tc.want)
		}
	}
	for _, tc := range []struct{ id, want string }{
		{"NONE", "PCM"}, {"twos", "PCM"}, {"sowt", "PCM"}, {"fl32", "IEEE Float"}, {"FL64", "IEEE Float"},
		{"ulaw", "Mu-law"}, {"alaw", "A-law"}, {"ima4", "IMA ADPCM"}, {"codec", "codec"},
	} {
		if got := aiffCodec(tc.id); got != tc.want {
			t.Errorf("aiffCodec(%q)=%q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestAudioHeaderAndID3Contracts(t *testing.T) {
	valid := []struct {
		name string
		data []byte
		want mpegHeader
	}{
		{"mpeg1 layer3", []byte{0xff, 0xfb, 0x90, 0x64}, mpegHeader{version: "1", layer: "3", bitRate: 128000, sampleRate: 44100, channels: 2, frameSize: 417, samples: 1152, channelMode: "Joint stereo"}},
		{"mpeg2 layer3", []byte{0xff, 0xf3, 0x80, 0xc4}, mpegHeader{version: "2", layer: "3", bitRate: 64000, sampleRate: 22050, channels: 1, frameSize: 208, samples: 576, channelMode: "Mono"}},
		{"mpeg25 layer3", []byte{0xff, 0xe3, 0x64, 0x00}, mpegHeader{version: "2.5", layer: "3", bitRate: 48000, sampleRate: 12000, channels: 2, frameSize: 288, samples: 576, channelMode: "Stereo"}},
		{"mpeg1 layer1", []byte{0xff, 0xff, 0x10, 0x00}, mpegHeader{version: "1", layer: "1", bitRate: 32000, sampleRate: 44100, channels: 2, frameSize: 32, samples: 384, channelMode: "Stereo"}},
	}
	for _, tc := range valid {
		got, ok := decodeMPEGHeader(tc.data)
		if !ok || got != tc.want {
			t.Errorf("decodeMPEGHeader(%s)=(%#v,%v), want %#v,true", tc.name, got, ok, tc.want)
		}
	}
	for _, data := range [][]byte{
		{}, {0xff}, {0x00, 0xfb, 0x90, 0x64}, {0xff, 0xeb, 0x90, 0x64}, {0xff, 0xf9, 0x90, 0x64},
		{0xff, 0xfb, 0x00, 0x64}, {0xff, 0xfb, 0xf0, 0x64}, {0xff, 0xfb, 0x9c, 0x64},
	} {
		if _, ok := decodeMPEGHeader(data); ok {
			t.Errorf("decodeMPEGHeader(%x) unexpectedly succeeded", data)
		}
	}
	if looksLikeMPEGAudio([]byte("noise")) || !looksLikeMPEGAudio([]byte("noise\xff\xfb\x90\x64")) {
		t.Fatal("looksLikeMPEGAudio did not scan embedded frame")
	}

	frame := make([]byte, 32)
	copy(frame, "Xing")
	binary.BigEndian.PutUint32(frame[4:8], 3)
	binary.BigEndian.PutUint32(frame[8:12], 123)
	binary.BigEndian.PutUint32(frame[12:16], 456)
	if frames, count := parseXingVBRI(frame, mpegHeader{}); frames != 123 || count != 456 {
		t.Fatalf("Xing=(%d,%d)", frames, count)
	}
	copy(frame, "Info")
	if frames, count := parseXingVBRI(frame, mpegHeader{}); frames != 123 || count != 456 {
		t.Fatalf("Info=(%d,%d)", frames, count)
	}
	copy(frame, "VBRI")
	binary.BigEndian.PutUint32(frame[10:14], 789)
	binary.BigEndian.PutUint32(frame[14:18], 12)
	if frames, count := parseXingVBRI(frame, mpegHeader{}); frames != 12 || count != 789 {
		t.Fatalf("VBRI=(%d,%d)", frames, count)
	}
	if frames, count := parseXingVBRI([]byte("plain"), mpegHeader{}); frames != 0 || count != 0 {
		t.Fatalf("empty VBR=(%d,%d)", frames, count)
	}

	if got := syncSafe([]byte{0x01, 0x02, 0x03, 0x04}); got != 2130308 {
		t.Errorf("syncSafe=%d", got)
	}
	if syncSafe(nil) != 0 {
		t.Error("short syncSafe should be zero")
	}
	if got := deunsync([]byte{0xff, 0, 1, 0xff, 2}); !bytes.Equal(got, []byte{0xff, 1, 0xff, 2}) {
		t.Errorf("deunsync=%x", got)
	}
	for _, id := range []string{"TIT2", "TT2", "TP1", "TAL", "TRK", "TYE"} {
		if !isID3Text(id) {
			t.Errorf("isID3Text(%q)=false", id)
		}
	}
	for _, id := range []string{"TXXX", "COMM", "APIC"} {
		if isID3Text(id) {
			t.Errorf("isID3Text(%q)=true", id)
		}
	}
	if decodeID3Text([]byte{0}) != "" || decodeID3Text([]byte{9, 'x'}) != "" {
		t.Error("invalid ID3 text was decoded")
	}
	if decodeID3Text([]byte{0, ' ', 'h', 'i', ' '}) != "hi" {
		t.Error("Latin-1 ID3 text was not cleaned")
	}
	if decodeID3Text([]byte{1, 0xff, 0xfe, 'h', 0, 'i', 0}) != "hi" {
		t.Error("UTF-16LE ID3 text was not decoded")
	}
	if decodeID3Text([]byte{1, 0xfe, 0xff, 0, 'h', 0, 'i'}) != "hi" {
		t.Error("UTF-16BE ID3 text was not decoded")
	}
	if decodeID3Text([]byte{2, 0, 'h', 0, 'i'}) != "hi" {
		t.Error("UTF-16BE without BOM was not decoded")
	}
	if extended80(nil) != 0 || math.Abs(extended80([]byte{0x3f, 0xff, 0x80, 0, 0, 0, 0, 0, 0, 0})-1) > 1e-12 {
		t.Error("extended80 scalar decoding failed")
	}
	if minInt(2, 3) != 2 || minInt(3, 2) != 2 {
		t.Error("minInt failed")
	}
}

func TestSubtitleFormatContracts(t *testing.T) {
	if !isSubtitleExtension(".SRT") || isSubtitleExtension(".txt") {
		t.Error("subtitle extension classification failed")
	}
	if !looksLikeText([]byte("plain text")) || looksLikeText([]byte{0xff, 0xff, 0xff}) {
		t.Error("text detection failed")
	}
	if !looksLikeText([]byte{0xff, 0xfe, 0, 0}) {
		t.Error("UTF-16 text detection failed")
	}
	if got, enc := decodeSubtitleBytes([]byte{0xef, 0xbb, 0xbf, 'h', 'i'}); got != "hi" || enc != "UTF-8" {
		t.Errorf("UTF-8 BOM=(%q,%q)", got, enc)
	}
	if got, enc := decodeSubtitleBytes([]byte{0xff, 0xfe, 'h', 0, 'i', 0}); got != "hi" || enc != "UTF-16LE" {
		t.Errorf("UTF-16LE=(%q,%q)", got, enc)
	}
	if got := subtitleTime([]string{"01", "02", "03", "004"}, 0); got != time.Hour+2*time.Minute+3*time.Second+4*time.Millisecond {
		t.Errorf("subtitleTime=%v", got)
	}
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"250ms", 250 * time.Millisecond}, {"1.5s", 1500 * time.Millisecond}, {"1:02:03.5", time.Hour + 2*time.Minute + 3500*time.Millisecond}, {"bad", 0},
	} {
		if got := parseClock(tc.value); got != tc.want {
			t.Errorf("parseClock(%q)=%v, want %v", tc.value, got, tc.want)
		}
	}

	cases := []struct {
		name string
		data string
		want string
		cues int64
	}{
		{"captions.srt", "1\n00:00:01,000 --> 00:00:02,500\nhello\n", "SubRip", 1},
		{"captions.vtt", "WEBVTT\n\n00:00:01.000 --> 00:00:03.000\nhello\n", "WebVTT", 1},
		{"captions.ass", "Style: Default\nDialogue: 0,0:00:01.00,0:00:02.50,Default,hello\n", "ASS", 1},
		{"captions.ttml", `<tt xml:lang="en"><body><p begin="1s" dur="2s">hello</p></body></tt>`, "TTML", 1},
		{"captions.sub", "{1}{1}25\n{25}{50}hello\n", "MicroDVD", 1},
	}
	for _, tc := range cases {
		r, err := analyzeBytes(t, tc.name, []byte(tc.data), ModeFast)
		if err != nil {
			t.Fatalf("Analyze(%s): %v", tc.name, err)
		}
		if r.General.Format != tc.want || len(r.Streams) != 1 || r.Streams[0].Text == nil || r.Streams[0].Text.CueCount != tc.cues {
			t.Errorf("Analyze(%s) report=%#v", tc.name, r)
		}
	}
}

func TestImageAndTIFFScalarContracts(t *testing.T) {
	for _, data := range [][]byte{{0xff, 0xd8}, []byte("\x89PNG\r\n\x1a\n"), []byte("GIF89a"), []byte("BM"), []byte("RIFF0000WEBP")} {
		if !isImageMagic(data) {
			t.Errorf("isImageMagic(%q)=false", data)
		}
	}
	if isImageMagic([]byte("no")) || isImageMagic(nil) {
		t.Error("invalid image magic accepted")
	}
	unsigned := make([]byte, 16)
	binary.LittleEndian.PutUint32(unsigned[0:4], 3)
	binary.LittleEndian.PutUint32(unsigned[4:8], 2)
	binary.LittleEndian.PutUint32(unsigned[8:12], 4)
	binary.LittleEndian.PutUint32(unsigned[12:16], 1)
	if got := tiffRationals(binary.LittleEndian, unsigned, false, 1); len(got) != 1 || got[0] != 1.5 {
		t.Errorf("unsigned rationals=%v", got)
	}
	signed := make([]byte, 8)
	binary.BigEndian.PutUint32(signed[0:4], uint32(0xffffffff))
	binary.BigEndian.PutUint32(signed[4:8], 2)
	if got := tiffRationals(binary.BigEndian, signed, true, 2); len(got) != 1 || got[0] != -0.5 {
		t.Errorf("signed rationals=%v", got)
	}
	zero := make([]byte, 8)
	if tiffRationals(binary.LittleEndian, zero, false, 1) != nil {
		t.Error("zero denominator was accepted")
	}
	if got := tiffRationals(binary.LittleEndian, unsigned, false, 0); len(got) != 0 {
		t.Errorf("zero limit=%v", got)
	}
	for _, tc := range []struct {
		raw  []byte
		typ  uint16
		want uint64
		ok   bool
	}{
		{[]byte{7}, 1, 7, true}, {[]byte{8}, 7, 8, true}, {[]byte{9, 0}, 3, 9, true}, {[]byte{10, 0, 0, 0}, 4, 10, true}, {nil, 1, 0, false}, {[]byte{1}, 3, 0, false}, {[]byte{1}, 9, 0, false},
	} {
		got, ok := tiffFirstInteger(binary.LittleEndian, tc.raw, tc.typ)
		if got != tc.want || ok != tc.ok {
			t.Errorf("tiffFirstInteger(%d)=(%d,%v), want (%d,%v)", tc.typ, got, ok, tc.want, tc.ok)
		}
	}
	for _, tc := range []struct {
		name string
		data []byte
		dng  bool
		want string
	}{
		{"x.dng", nil, false, "DNG"}, {"x.bin", nil, true, "DNG"}, {"x.cr2", make([]byte, 12), false, "Canon RAW"}, {"x.cr2", append(make([]byte, 8), 'C', 'R', 2, 0), false, "Canon CR2"},
		{"x.nef", nil, false, "Nikon NEF"}, {"x.pef", nil, false, "Pentax PEF"}, {"x.arw", nil, false, "Sony ARW"}, {"x.tif", nil, false, "TIFF"},
	} {
		if got, _ := tiffFormat(tc.name, tc.data, tc.dng); got != tc.want {
			t.Errorf("tiffFormat(%q)=%q, want %q", tc.name, got, tc.want)
		}
	}
	for _, tc := range []struct {
		value uint16
		want  string
	}{
		{1, "Uncompressed"}, {5, "LZW"}, {6, "JPEG"}, {7, "JPEG"}, {8, "Deflate"}, {32946, "Deflate"}, {32773, "PackBits"}, {34712, "JPEG 2000"}, {34892, "Lossy JPEG"}, {34893, "Lossy JPEG"}, {65000, "Kodak DCR"}, {0, ""}, {99, "TIFF compression 99"},
	} {
		if got := tiffCompression(tc.value); got != tc.want {
			t.Errorf("tiffCompression(%d)=%q, want %q", tc.value, got, tc.want)
		}
	}
	if !usefulTIFFTag(uint8(0), 0x010f) || !usefulTIFFTag(uint8(2), 5) || usefulTIFFTag(uint8(2), 7) || usefulTIFFTag(uint8(0), 7) {
		t.Error("usefulTIFFTag classification failed")
	}
	if formatEXIFExposure(nil) != "" || formatEXIFExposure([]float64{0}) != "" || formatEXIFExposure([]float64{0.004}) != "1/250 s" || formatEXIFExposure([]float64{2}) != "2 s" {
		t.Error("EXIF exposure formatting failed")
	}
	if formatEXIFAperture(nil) != "" || formatEXIFAperture([]float64{-1}) != "" || formatEXIFAperture([]float64{2.8}) != "f/2.8" {
		t.Error("EXIF aperture formatting failed")
	}
	if formatEXIFVersion([]byte("0230")) != "02.30" || formatEXIFVersion([]byte(" text ")) != "text" {
		t.Error("EXIF version formatting failed")
	}
	for value, want := range map[uint64]string{0: "Unknown", 1: "Average", 2: "Center-weighted average", 3: "Spot", 4: "Multi-spot", 5: "Pattern", 6: "Partial", 255: "Other"} {
		if exifMeteringMode(value) != want {
			t.Errorf("metering %d=%q", value, exifMeteringMode(value))
		}
	}
	for value, want := range map[uint64]string{0: "Undefined", 1: "Manual", 2: "Normal program", 3: "Aperture priority", 4: "Shutter priority", 5: "Creative", 6: "Action", 7: "Portrait", 8: "Landscape", 9: "Bulb"} {
		if exifExposureProgram(value) != want {
			t.Errorf("program %d=%q", value, exifExposureProgram(value))
		}
	}
	if exifFlash(1) != "Fired" || exifFlash(0) != "Did not fire" || exifLightSource(21) != "D65" || exifLightSource(255) != "Other" {
		t.Error("EXIF enum formatting failed")
	}
	p, err := newProbe(context.Background(), Source{Name: "x", Size: 1, Reader: memorySource{0}}, Options{MaxValueBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := boundedEXIFComment(p, []byte("ASCII\x00\x00\x00text")); got != "tex" || !p.report.Truncated {
		t.Errorf("bounded EXIF comment=%q truncated=%v", got, p.report.Truncated)
	}
}

func TestHEIFAndMatroskaScalarContracts(t *testing.T) {
	if !isHEIFSource("photo.heic", nil) || !isHEIFSource("photo.bin", []byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0}) || isHEIFSource("photo.bin", []byte("not a heif file")) {
		t.Error("HEIF source detection failed")
	}
	p, err := newProbe(context.Background(), Source{Name: "x", Size: 20, Reader: memorySource(bytes.Repeat([]byte{0xff}, 20))}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	if value, n, unknown, err := readEBMLVInt(p, 0, false); err != nil || value != 127 || n != 1 || !unknown {
		t.Errorf("unknown EBML size=(%d,%d,%v,%v)", value, n, unknown, err)
	}
	data := []byte{0x1a, 0x45, 0xdf, 0xa3, 0x83, 0x81, 0x7f, 0xff, 0xff}
	p, err = newProbe(context.Background(), Source{Name: "x", Size: int64(len(data)), Reader: memorySource(data)}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	if value, n, unknown, err := readEBMLVInt(p, 0, true); err != nil || value != 0x1a45dfa3 || n != 4 || unknown {
		t.Errorf("EBML id=(%x,%d,%v,%v)", value, n, unknown, err)
	}
	if value, n, unknown, err := readEBMLVInt(p, 4, false); err != nil || value != 3 || n != 1 || unknown {
		t.Errorf("EBML size=(%d,%d,%v,%v)", value, n, unknown, err)
	}
	if _, _, _, err := readEBMLVInt(p, 8, false); err != nil {
		t.Errorf("short EBML integer: %v", err)
	}
	zero := []byte{0}
	p, _ = newProbe(context.Background(), Source{Name: "x", Size: 1, Reader: memorySource(zero)}, DefaultOptions(ModeFast))
	if _, _, _, err := readEBMLVInt(p, 0, false); err == nil {
		t.Error("zero EBML integer was accepted")
	}

	scalar := []byte{1, 2, 3, 4, 0x3f, 0x80, 0, 0, 'h', 'i', 0, 0}
	p, _ = newProbe(context.Background(), Source{Name: "x", Size: int64(len(scalar)), Reader: memorySource(scalar)}, Options{MaxValueBytes: 2, MaxSingleMetadataBytes: 8})
	if value, ok, err := readEBMLUint(p, 0, 4); err != nil || !ok || value != 0x01020304 {
		t.Errorf("EBML uint=(%d,%v,%v)", value, ok, err)
	}
	if value, ok, err := readEBMLFloat(p, 4, 4); err != nil || !ok || value != 1 {
		t.Errorf("EBML float=(%v,%v,%v)", value, ok, err)
	}
	if got, ok, err := readEBMLText(p, 8, 4); err != nil || !ok || got != "hi" || !p.report.Truncated {
		t.Errorf("EBML text=(%q,%v,%v), truncated=%v", got, ok, err, p.report.Truncated)
	}
	if _, ok, err := readEBMLUint(p, 0, 9); err != nil || ok || !p.report.Truncated {
		t.Error("oversized EBML uint was not bounded")
	}
	if _, ok, err := readEBMLFloat(p, 0, 3); err != nil || ok {
		t.Error("invalid EBML float was accepted")
	}
	if got, ok, err := readEBMLText(p, 0, 0); err != nil || !ok || got != "" {
		t.Error("empty EBML text failed")
	}
	if got, ok, err := readEBMLDocType(p, 0, 4); err != nil || !ok || got != "\x01\x02\x03\x04" {
		t.Errorf("EBML doc type=(%q,%v,%v)", got, ok, err)
	}
	if _, ok, err := readEBMLDocType(p, 0, 33); err != nil || ok {
		t.Error("oversized EBML doc type was accepted")
	}

	for _, id := range []uint64{0x4282, 0x2ad7b1, 0xd7, 0x4489, 0xb5, 0x7ba9, 0x4d80} {
		if classifyEBMLLeaf(id) == ebmlLeafUnknown {
			t.Errorf("classifyEBMLLeaf(%x) is unknown", id)
		}
	}
	if classifyEBMLLeaf(0xdead) != ebmlLeafUnknown {
		t.Error("unknown EBML leaf classified")
	}
	if !needEBMLLeaf(0xd7, &ebmlContext{track: &Stream{}}) || needEBMLLeaf(0xd7, &ebmlContext{}) || !needEBMLLeaf(0x4487, &ebmlContext{tagName: "x"}) || needEBMLLeaf(0x4487, &ebmlContext{}) || !needEBMLLeaf(0x63c5, &ebmlContext{tagTarget: new(string)}) || needEBMLLeaf(0x63c5, &ebmlContext{}) {
		t.Error("needEBMLLeaf context filtering failed")
	}
	for value, kind := range map[uint64]StreamKind{1: StreamVideo, 2: StreamAudio, 17: StreamText, 99: StreamText} {
		stream := Stream{}
		setMatroskaTrackType(&stream, value)
		if stream.Kind != kind {
			t.Errorf("track type %d=%q", value, stream.Kind)
		}
	}
	stream := Stream{}
	video := ensureVideo(&stream)
	audio := ensureAudio(&stream)
	if ensureVideo(&stream) != video || ensureAudio(&stream) != audio || stream.Video == nil || stream.Audio == nil {
		t.Error("stream component initialization failed")
	}
	for _, tc := range []struct{ id, want string }{
		{"V_MPEG4/ISO/AVC", "AVC"}, {"V_MPEGH/ISO/HEVC", "HEVC"}, {"V_AV1", "AV1"}, {"V_VP8", "VP8"}, {"V_VP9", "VP9"}, {"V_THEORA", "Theora"},
		{"A_AAC", "AAC"}, {"A_OPUS", "Opus"}, {"A_VORBIS", "Vorbis"}, {"A_FLAC", "FLAC"}, {"A_MPEG/L3", "MPEG Audio Layer 3"}, {"A_AC3", "AC-3"}, {"A_EAC3", "E-AC-3"}, {"A_DTS", "DTS"},
		{"S_TEXT/UTF8", "SubRip"}, {"S_TEXT/ASS", "ASS"}, {"S_TEXT/SSA", "SSA"}, {"S_TEXT/WEBVTT", "WebVTT"}, {"S_HDMV/PGS", "PGS"}, {"S_VOBSUB", "VobSub"}, {"X/custom", "custom"}, {"custom", "custom"},
	} {
		if got := matroskaCodec(tc.id); got != tc.want {
			t.Errorf("matroskaCodec(%q)=%q, want %q", tc.id, got, tc.want)
		}
	}
}
