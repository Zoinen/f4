package mediainfo

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestDecodeMPEGHeaderVariants(t *testing.T) {
	tests := []struct {
		name       string
		header     []byte
		version    string
		layer      string
		sampleRate int
		channels   int
	}{
		{name: "mpeg1 layer3", header: []byte{0xff, 0xfb, 0x90, 0x64}, version: "1", layer: "3", sampleRate: 44100, channels: 2},
		{name: "mpeg2 layer2 mono", header: []byte{0xff, 0xf4, 0x50, 0xc0}, version: "2", layer: "2", sampleRate: 22050, channels: 1},
		{name: "mpeg2 layer1", header: []byte{0xff, 0xf6, 0x50, 0x00}, version: "2", layer: "1", sampleRate: 22050, channels: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, ok := decodeMPEGHeader(tc.header)
			if !ok {
				t.Fatal("decodeMPEGHeader rejected a valid header")
			}
			if h.version != tc.version || h.layer != tc.layer || h.sampleRate != tc.sampleRate || h.channels != tc.channels {
				t.Fatalf("header = %#v, want version %s layer %s rate %d channels %d", h, tc.version, tc.layer, tc.sampleRate, tc.channels)
			}
			if h.frameSize < 4 || h.bitRate <= 0 || h.samples <= 0 {
				t.Fatalf("incomplete header information: %#v", h)
			}
		})
	}
}

func TestDecodeMPEGHeaderRejectsInvalidHeaders(t *testing.T) {
	for _, header := range [][]byte{
		{}, {0xff, 0xfb}, {0x00, 0xfb, 0x90, 0x64},
		{0xff, 0xe8, 0x90, 0x64}, {0xff, 0xfb, 0x00, 0x64},
		{0xff, 0xfb, 0xf0, 0x64}, {0xff, 0xfb, 0x9c, 0x64},
	} {
		if _, ok := decodeMPEGHeader(header); ok {
			t.Errorf("decodeMPEGHeader accepted %x", header)
		}
	}
}

func TestLooksLikeMPEGAudioScansAndRejects(t *testing.T) {
	withPrefix := append([]byte("noise"), []byte{0xff, 0xfb, 0x90, 0x64}...)
	if !looksLikeMPEGAudio(withPrefix) {
		t.Fatal("looksLikeMPEGAudio did not find a frame after a prefix")
	}
	if looksLikeMPEGAudio([]byte("not MPEG audio")) {
		t.Fatal("looksLikeMPEGAudio accepted arbitrary data")
	}
	long := make([]byte, 5000)
	copy(long[4096:], []byte{0xff, 0xfb, 0x90, 0x64})
	if looksLikeMPEGAudio(long) {
		t.Fatal("looksLikeMPEGAudio searched beyond its 4096-byte limit")
	}
}

func TestParseXingAndVBRI(t *testing.T) {
	frame := make([]byte, 64)
	copy(frame[4:], "Xing")
	binary.BigEndian.PutUint32(frame[8:], 3)
	binary.BigEndian.PutUint32(frame[12:], 120)
	binary.BigEndian.PutUint32(frame[16:], 9000)
	if frames, bytesCount := parseXingVBRI(frame, mpegHeader{}); frames != 120 || bytesCount != 9000 {
		t.Fatalf("Xing = (%d, %d), want (120, 9000)", frames, bytesCount)
	}

	vbri := make([]byte, 32)
	copy(vbri[2:], "VBRI")
	binary.BigEndian.PutUint32(vbri[12:], 7000)
	binary.BigEndian.PutUint32(vbri[16:], 70)
	if frames, bytesCount := parseXingVBRI(vbri, mpegHeader{}); frames != 70 || bytesCount != 7000 {
		t.Fatalf("VBRI = (%d, %d), want (70, 7000)", frames, bytesCount)
	}
	if frames, bytesCount := parseXingVBRI([]byte("nothing"), mpegHeader{}); frames != 0 || bytesCount != 0 {
		t.Fatalf("empty VBR metadata = (%d, %d), want (0, 0)", frames, bytesCount)
	}
}

func TestSyncSafeAndDeunsync(t *testing.T) {
	if got := syncSafe([]byte{0x01, 0x02, 0x03, 0x04}); got != 0x208184 {
		t.Fatalf("syncSafe = %#x, want %#x", got, 0x208184)
	}
	if syncSafe([]byte{1, 2, 3}) != 0 {
		t.Fatal("syncSafe accepted a short input")
	}
	input := []byte{0xff, 0x00, 0x12, 0xff, 0x01, 0xff}
	want := []byte{0xff, 0x12, 0xff, 0x01, 0xff}
	if got := deunsync(input); string(got) != string(want) {
		t.Fatalf("deunsync = %x, want %x", got, want)
	}
}

func TestID3TextHelpers(t *testing.T) {
	for _, id := range []string{"TIT2", "TT2", "TP1", "TAL", "TRK", "TYE"} {
		if !isID3Text(id) {
			t.Errorf("isID3Text(%q) = false", id)
		}
	}
	for _, id := range []string{"TXXX", "COMM", "APIC"} {
		if isID3Text(id) {
			t.Errorf("isID3Text(%q) = true", id)
		}
	}
	if got := decodeID3Text([]byte{3, 'H', 'i', 0}); got != "Hi" {
		t.Fatalf("UTF-8 ID3 text = %q, want Hi", got)
	}
	if got := decodeID3Text([]byte{0}); got != "" {
		t.Fatalf("short ID3 text = %q, want empty", got)
	}
}

func TestOggFLACHeaderAndStreamInfo(t *testing.T) {
	if !isOggFLACHeader([]byte("fLaC")) || !isOggFLACHeader([]byte{0x7f, 'F', 'L', 'A', 'C'}) {
		t.Fatal("valid Ogg FLAC headers were rejected")
	}
	if isOggFLACHeader([]byte("FLAC")) || isOggFLACHeader([]byte("ogg")) {
		t.Fatal("invalid Ogg FLAC header was accepted")
	}

	packet := make([]byte, 42)
	copy(packet, "fLaC")
	packet[4] = 0
	packet[7] = 34
	value := uint64(48000)<<44 | uint64(1)<<41 | uint64(23)<<36 | 96000
	binary.BigEndian.PutUint64(packet[18:], value)
	stream := Stream{Audio: &Audio{}}
	if !parseOggFLACStreamInfo(packet, &stream) {
		t.Fatal("parseOggFLACStreamInfo rejected a valid stream-info block")
	}
	if stream.Audio.SampleRate != 48000 || stream.Audio.Channels != 2 || stream.Audio.BitDepth != 24 {
		t.Fatalf("stream info = %#v", stream.Audio)
	}
	if parseOggFLACStreamInfo([]byte("fLaC"), &stream) {
		t.Fatal("parseOggFLACStreamInfo accepted a truncated packet")
	}
}

func TestParseTheoraIdentification(t *testing.T) {
	packet := make([]byte, 42)
	packet[7], packet[8], packet[9] = 3, 2, 1
	binary.BigEndian.PutUint16(packet[10:12], 40)
	binary.BigEndian.PutUint16(packet[12:14], 30)
	binary.BigEndian.PutUint32(packet[22:26], 30)
	binary.BigEndian.PutUint32(packet[26:30], 1)
	stream := Stream{Video: &Video{}}
	parseTheoraIdentification(packet, &stream)
	if stream.Video.Width != 640 || stream.Video.Height != 480 || stream.FrameRate != 30 {
		t.Fatalf("Theora info = %#v, rate %v", stream.Video, stream.FrameRate)
	}
	if stream.Profile != "Theora 3.2.1" {
		t.Fatalf("profile = %q", stream.Profile)
	}
	parseTheoraIdentification([]byte{0, 1, 2}, &stream)
}

func TestExtended80(t *testing.T) {
	if got := extended80([]byte{0x40, 0x0e, 0xac, 0x44, 0, 0, 0, 0, 0, 0}); got != 44100 {
		t.Fatalf("extended80 = %v, want 44100", got)
	}
	if extended80([]byte{1, 2, 3}) != 0 || extended80(make([]byte, 10)) != 0 {
		t.Fatal("extended80 did not handle zero/short values")
	}
}

func TestAIFFCodecAndMinInt(t *testing.T) {
	tests := map[string]string{
		"NONE": "PCM", "twos": "PCM", "sowt": "PCM",
		"fl32": "IEEE Float", "FL64": "IEEE Float", "ulaw": "Mu-law",
		"alaw": "A-law", "ima4": "IMA ADPCM", "custom": "custom",
	}
	for input, want := range tests {
		if got := aiffCodec(input); got != want {
			t.Errorf("aiffCodec(%q) = %q, want %q", input, got, want)
		}
	}
	if aiffCodec(" NONE ") != "PCM" || minInt(3, 5) != 3 || minInt(5, 3) != 3 {
		t.Fatal("aiffCodec/minInt edge cases failed")
	}
	if math.IsNaN(extended80([]byte{0x7f, 0xff, 0, 0, 0, 0, 0, 0, 0, 0})) {
		t.Fatal("extended80 returned NaN for a zero mantissa")
	}
}
