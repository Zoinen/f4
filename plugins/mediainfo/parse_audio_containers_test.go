package mediainfo

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

// These tests exercise the top-level container parsers in parse_audio.go
// (parseMPEGAudio, parseID3v2, parseID3v1, parseFLAC, parseOgg's Vorbis and
// Speex branches, and parseAIFF) end to end through Analyze, plus a couple
// of parseID3v2/parseID3v1 edge cases directly. The pure bit-twiddling
// helpers they call (decodeMPEGHeader, decodeID3Text, deunsync, ...) already
// have direct unit tests elsewhere in this package.

func newMediaProbe(t *testing.T, data []byte) *probe {
	t.Helper()
	p, err := newProbe(context.Background(), Source{
		Name:   "fixture",
		Size:   int64(len(data)),
		Reader: memorySource(data),
	}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func syncSafeEncode(n int) []byte {
	return []byte{byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
}

// id3v2Header builds a 10-byte ID3v2 header: "ID3" + major version + a
// (unused) revision byte + flags + a syncsafe size of the frames that follow.
func id3v2Header(ver, flags byte, bodyLen int) []byte {
	h := []byte{'I', 'D', '3', ver, 0, flags}
	return append(h, syncSafeEncode(bodyLen)...)
}

func buildID3v2Tag(ver, flags byte, frames ...[]byte) []byte {
	var body []byte
	for _, f := range frames {
		body = append(body, f...)
	}
	return append(id3v2Header(ver, flags, len(body)), body...)
}

// id3v22Frame builds a legacy ID3v2.2 frame: a 3-byte id and a 3-byte size.
func id3v22Frame(id string, payload []byte) []byte {
	f := []byte(id)
	sz := len(payload)
	f = append(f, byte(sz>>16), byte(sz>>8), byte(sz))
	return append(f, payload...)
}

// id3v2Frame builds an ID3v2.3/2.4 frame: a 4-byte id, a 4-byte size
// (syncsafe for v2.4, plain big-endian otherwise) and two flag bytes.
func id3v2Frame(ver byte, id string, payload []byte) []byte {
	f := []byte(id)
	sz := len(payload)
	if ver == 4 {
		f = append(f, syncSafeEncode(sz)...)
	} else {
		f = append(f, byte(sz>>24), byte(sz>>16), byte(sz>>8), byte(sz))
	}
	f = append(f, 0, 0)
	return append(f, payload...)
}

func TestParseID3v2FrameVariants(t *testing.T) {
	cases := []struct {
		name     string
		ver      byte
		frames   [][]byte
		wantTags map[string]string
	}{
		{
			// canonicalTag has no mapping for the legacy 3-character v2.2
			// ids, so the raw frame id passes through unchanged as the tag
			// name; this case exists to exercise the id3v2.2 3-byte
			// id/3-byte size frame layout (parseID3v2's ver==2 branch).
			name:     "id3v2.2 legacy 3-byte ids",
			ver:      2,
			frames:   [][]byte{id3v22Frame("TT2", append([]byte{0}, []byte("LegacySong")...))},
			wantTags: map[string]string{"TT2": "LegacySong"},
		},
		{
			name:     "id3v2.3 standard ids",
			ver:      3,
			frames:   [][]byte{id3v2Frame(3, "TALB", append([]byte{3}, []byte("MyAlbum")...))},
			wantTags: map[string]string{"Album": "MyAlbum"},
		},
		{
			name:     "id3v2.4 syncsafe size and UTF-16 text",
			ver:      4,
			frames:   [][]byte{id3v2Frame(4, "TIT2", []byte{1, 0xff, 0xfe, 'H', 0, 'i', 0})},
			wantTags: map[string]string{"Title": "Hi"},
		},
		{
			name:     "COMM frame carries a comment",
			ver:      3,
			frames:   [][]byte{id3v2Frame(3, "COMM", append([]byte{0}, []byte("engNice track")...))},
			wantTags: map[string]string{"Comment": "Nice track"},
		},
		{
			name:     "TXXX is not treated as plain text",
			ver:      3,
			frames:   [][]byte{id3v2Frame(3, "TXXX", append([]byte{0}, []byte("ignored")...))},
			wantTags: map[string]string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full := buildID3v2Tag(tc.ver, 0, tc.frames...)
			p := newMediaProbe(t, full)
			if err := parseID3v2(p, 0, len(full)); err != nil {
				t.Fatalf("parseID3v2 returned an error: %v", err)
			}
			got := map[string]string{}
			for _, tag := range p.report.Tags {
				got[tag.Name] = tag.Value
			}
			for k, v := range tc.wantTags {
				if got[k] != v {
					t.Fatalf("tag %s = %q, want %q (all tags %#v)", k, got[k], v, p.report.Tags)
				}
			}
			if len(tc.wantTags) == 0 && len(p.report.Tags) != 0 {
				t.Fatalf("expected no tags, got %#v", p.report.Tags)
			}
		})
	}
}

func TestParseID3v2SkipsFrameClaimingMoreDataThanPresent(t *testing.T) {
	frameHeader := make([]byte, 10)
	copy(frameHeader[:4], "TIT2")
	binary.BigEndian.PutUint32(frameHeader[4:8], 9999)
	full := buildID3v2Tag(3, 0, frameHeader)
	p := newMediaProbe(t, full)
	if err := parseID3v2(p, 0, len(full)); err != nil {
		t.Fatalf("parseID3v2 returned an error for a truncated frame: %v", err)
	}
	if len(p.report.Tags) != 0 {
		t.Fatalf("truncated frame unexpectedly produced tags: %#v", p.report.Tags)
	}
}

func TestParseID3v1Tag(t *testing.T) {
	b := make([]byte, 128)
	copy(b[:3], "TAG")
	copy(b[3:33], "Song Title")
	copy(b[33:63], "The Artist")
	copy(b[63:93], "Great Album")
	copy(b[93:97], "2024")
	p := newMediaProbe(t, nil)
	parseID3v1(p, b)
	got := map[string]string{}
	for _, tag := range p.report.Tags {
		got[tag.Name] = tag.Value
	}
	want := map[string]string{"Title": "Song Title", "Artist": "The Artist", "Album": "Great Album", "Date": "2024"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("tag %s = %q, want %q (all tags %#v)", k, got[k], v, p.report.Tags)
		}
	}

	p2 := newMediaProbe(t, nil)
	parseID3v1(p2, make([]byte, 50))
	if len(p2.report.Tags) != 0 {
		t.Fatalf("short buffer unexpectedly produced tags: %#v", p2.report.Tags)
	}
}

func TestAnalyzeMPEGAudioWithID3v2Tag(t *testing.T) {
	frame := id3v2Frame(3, "TIT2", append([]byte{3}, []byte("Great Song")...))
	body := buildID3v2Tag(3, 0, frame)
	body = append(body, 0xff, 0xfb, 0x90, 0x64)
	r, err := analyzeBytes(t, "tagged.mp3", body, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if r.General.Format != "MPEG Audio" {
		t.Fatalf("Format = %q", r.General.Format)
	}
	found := false
	for _, tag := range r.Tags {
		if tag.Name == "Title" && tag.Value == "Great Song" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a Title tag from the ID3v2 header, got %#v", r.Tags)
	}
}

func TestAnalyzeMPEGAudioID3v2ExtendedHeader(t *testing.T) {
	frame := id3v2Frame(3, "TIT2", append([]byte{3}, []byte("X")...))
	body := buildID3v2Tag(3, 0x10, frame) // extended-header flag set
	body = append(body, make([]byte, 10)...)
	body = append(body, 0xff, 0xfb, 0x90, 0x64)
	r, err := analyzeBytes(t, "ext.mp3", body, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if r.General.Format != "MPEG Audio" {
		t.Fatalf("Format = %q", r.General.Format)
	}
}

func TestAnalyzeMPEGAudioOversizedID3TagLeavesNoFrame(t *testing.T) {
	header := id3v2Header(3, 0, 10_000_000)
	_, err := analyzeBytes(t, "huge.mp3", header, ModeFast)
	if err == nil {
		t.Fatal("expected an error when no audio frame follows an oversized ID3 tag")
	}
	var perr *ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *ParseError", err)
	}
}

func TestAnalyzeMPEGAudioXingVBR(t *testing.T) {
	b := make([]byte, 417) // frame size for this header at 128kbps/44100Hz
	copy(b, []byte{0xff, 0xfb, 0x90, 0x64})
	copy(b[4:], "Xing")
	binary.BigEndian.PutUint32(b[8:12], 3) // frames + bytes flags
	binary.BigEndian.PutUint32(b[12:16], 100)
	binary.BigEndian.PutUint32(b[16:20], 36000)
	r, err := analyzeBytes(t, "vbr.mp3", b, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Streams[0]
	if s.BitRateMode != "VBR" || s.FrameCount != 100 || s.Duration <= 0 {
		t.Fatalf("VBR stream = %#v", s)
	}
}

func flacMetadataBlock(typ byte, last bool, data []byte) []byte {
	h := typ & 0x7f
	if last {
		h |= 0x80
	}
	sz := len(data)
	return append([]byte{h, byte(sz >> 16), byte(sz >> 8), byte(sz)}, data...)
}

func flacStreamInfoBlock(rate, channels, bits int, samples uint64) []byte {
	b := make([]byte, 34)
	v := uint64(rate)<<44 | uint64(channels-1)<<41 | uint64(bits-1)<<36 | samples
	binary.BigEndian.PutUint64(b[10:18], v)
	return b
}

func flacVorbisCommentBlock(vendor string, comments []string) []byte {
	var b []byte
	vb := []byte(vendor)
	sz := make([]byte, 4)
	binary.LittleEndian.PutUint32(sz, uint32(len(vb)))
	b = append(b, sz...)
	b = append(b, vb...)
	cnt := make([]byte, 4)
	binary.LittleEndian.PutUint32(cnt, uint32(len(comments)))
	b = append(b, cnt...)
	for _, c := range comments {
		cb := []byte(c)
		l := make([]byte, 4)
		binary.LittleEndian.PutUint32(l, uint32(len(cb)))
		b = append(b, l...)
		b = append(b, cb...)
	}
	return b
}

func flacPictureBlock(width, height uint32) []byte {
	b := make([]byte, 32) // pictureType(4) + mimeLen=0(4) + descLen=0(4) + width(4) + height(4) + padding
	binary.BigEndian.PutUint32(b[12:16], width)
	binary.BigEndian.PutUint32(b[16:20], height)
	return b
}

func TestAnalyzeNativeFLACStreamInfoCommentsAndPicture(t *testing.T) {
	data := []byte("fLaC")
	data = append(data, flacMetadataBlock(0, false, flacStreamInfoBlock(48000, 2, 24, 96000))...)
	data = append(data, flacMetadataBlock(4, false, flacVorbisCommentBlock("libFLAC test", []string{"TITLE=Nightfall", "NOEQUALSIGN"}))...)
	data = append(data, flacMetadataBlock(6, true, flacPictureBlock(100, 200))...)
	r, err := analyzeBytes(t, "song.flac", data, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if r.General.Format != "FLAC" || r.General.MIME != "audio/flac" {
		t.Fatalf("General = %#v", r.General)
	}
	s := r.Streams[0]
	if s.Audio.SampleRate != 48000 || s.Audio.Channels != 2 || s.Audio.BitDepth != 24 || s.Duration != 2*time.Second {
		t.Fatalf("stream = %#v", s)
	}
	if r.General.WritingApp != "libFLAC test" {
		t.Fatalf("WritingApp = %q", r.General.WritingApp)
	}
	got := map[string]string{}
	for _, tag := range r.Tags {
		got[tag.Name] = tag.Value
	}
	if got["Title"] != "Nightfall" {
		t.Fatalf("Title tag = %q, all tags %#v", got["Title"], r.Tags)
	}
	if got["Cover dimensions"] != "100x200" {
		t.Fatalf("Cover dimensions tag = %q, all tags %#v", got["Cover dimensions"], r.Tags)
	}
}

func TestAnalyzeNativeFLACMetadataBlockExceedsFile(t *testing.T) {
	data := []byte("fLaC")
	data = append(data, flacMetadataBlock(0, false, make([]byte, 34))...)
	data = append(data, 0, 0, 0, 100) // a second block header claiming 100 bytes that never follow
	_, err := analyzeBytes(t, "broken.flac", data, ModeFast)
	if err == nil {
		t.Fatal("expected an error for a metadata block that overruns the file")
	}
	var perr *ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *ParseError", err)
	}
}

func TestAnalyzeOggVorbis(t *testing.T) {
	ident := make([]byte, 30)
	ident[0] = 1
	copy(ident[1:7], "vorbis")
	ident[11] = 2
	binary.LittleEndian.PutUint32(ident[12:16], 44100)
	comment := append([]byte{3}, []byte("vorbis")...)
	comment = append(comment, flacVorbisCommentBlock("libvorbis", []string{"ARTIST=Test Artist"})...)
	data := append(oggPage(21, 0, 0, ident), oggPage(21, 0, 1, comment)...)
	data = append(data, oggPage(21, 44100, 2, nil)...)
	r, err := analyzeBytes(t, "tone.ogg", data, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Streams[0]
	if s.Format != "Vorbis" || s.Audio.Channels != 2 || s.Audio.SampleRate != 44100 || s.Duration != time.Second {
		t.Fatalf("Vorbis stream = %#v", s)
	}
	found := false
	for _, tag := range r.Tags {
		if tag.Name == "Artist" && tag.Value == "Test Artist" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an Artist tag from the Vorbis comment header, got %#v", r.Tags)
	}
}

func TestAnalyzeOggSpeex(t *testing.T) {
	ident := make([]byte, 80)
	copy(ident[:8], "Speex   ")
	binary.LittleEndian.PutUint32(ident[36:40], 16000)
	binary.LittleEndian.PutUint32(ident[48:52], 1)
	data := append(oggPage(33, 0, 0, ident), oggPage(33, 16000, 1, nil)...)
	r, err := analyzeBytes(t, "tone.spx", data, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Streams[0]
	if s.Format != "Speex" || s.Audio.SampleRate != 16000 || s.Audio.Channels != 1 || s.Duration != time.Second {
		t.Fatalf("Speex stream = %#v", s)
	}
}

func TestReadOggPacketsRejectsBadPageCapture(t *testing.T) {
	page0 := oggPage(5, 0, 0, []byte("short packet"))
	garbage := make([]byte, 27) // does not start with "OggS"
	data := append(page0, garbage...)
	_, err := analyzeBytes(t, "broken.ogg", data, ModeFast)
	if err == nil {
		t.Fatal("expected an error for a corrupt second Ogg page")
	}
	var perr *ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *ParseError", err)
	}
}

func aiffChunk(id string, payload []byte) []byte {
	sz := make([]byte, 4)
	binary.BigEndian.PutUint32(sz, uint32(len(payload)))
	b := append([]byte(id), sz...)
	b = append(b, payload...)
	if len(payload)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

func aiffCommPayload(channels uint16, frames uint32, bits uint16, extendedRate, extra []byte) []byte {
	b := make([]byte, 0, 8+len(extendedRate)+len(extra))
	ch := make([]byte, 2)
	binary.BigEndian.PutUint16(ch, channels)
	fr := make([]byte, 4)
	binary.BigEndian.PutUint32(fr, frames)
	bd := make([]byte, 2)
	binary.BigEndian.PutUint16(bd, bits)
	b = append(b, ch...)
	b = append(b, fr...)
	b = append(b, bd...)
	b = append(b, extendedRate...)
	b = append(b, extra...)
	return b
}

func aiffFile(form string, chunks ...[]byte) []byte {
	var body []byte
	for _, c := range chunks {
		body = append(body, c...)
	}
	header := append([]byte("FORM"), make([]byte, 4)...)
	binary.BigEndian.PutUint32(header[4:8], uint32(4+len(body)))
	header = append(header, []byte(form)...)
	return append(header, body...)
}

// rate44100 is the 80-bit extended-precision encoding of 44100, matching
// TestExtended80 in parse_audio_coverage_batch20_test.go.
var rate44100 = []byte{0x40, 0x0e, 0xac, 0x44, 0, 0, 0, 0, 0, 0}

func TestAnalyzeAIFFTagsAndChunkPadding(t *testing.T) {
	comm := aiffChunk("COMM", aiffCommPayload(2, 44100, 16, rate44100, nil))
	name := aiffChunk("NAME", []byte("My Song"))   // odd length: exercises the pad byte
	auth := aiffChunk("AUTH", []byte("Artist X"))  // even length: no pad byte
	anno := aiffChunk("ANNO", []byte("A comment")) // odd length: exercises the pad byte
	data := aiffFile("AIFF", comm, name, auth, anno)
	r, err := analyzeBytes(t, "tone.aiff", data, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	if r.General.Format != "AIFF" {
		t.Fatalf("Format = %q", r.General.Format)
	}
	s := r.Streams[0]
	if s.Audio.Channels != 2 || s.Audio.SampleRate != 44100 || s.Audio.BitDepth != 16 || s.Duration != time.Second {
		t.Fatalf("stream = %#v", s)
	}
	got := map[string]string{}
	for _, tag := range r.Tags {
		got[tag.Name] = tag.Value
	}
	want := map[string]string{"Title": "My Song", "Artist": "Artist X", "Comment": "A comment"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("tag %s = %q, want %q (all tags %#v)", k, got[k], v, r.Tags)
		}
	}
}

func TestAnalyzeAIFCCodecID(t *testing.T) {
	comm := aiffChunk("COMM", aiffCommPayload(1, 22050, 32, rate44100, []byte("fl32")))
	data := aiffFile("AIFC", comm)
	r, err := analyzeBytes(t, "tone.aifc", data, ModeFast)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Streams[0]
	if s.CodecID != "fl32" || s.Format != "IEEE Float" {
		t.Fatalf("AIFC stream = %#v", s)
	}
}

func TestAnalyzeAIFFInvalidSampleRate(t *testing.T) {
	comm := aiffChunk("COMM", aiffCommPayload(2, 100, 16, make([]byte, 10), nil))
	data := aiffFile("AIFF", comm)
	_, err := analyzeBytes(t, "bad.aiff", data, ModeFast)
	if err == nil {
		t.Fatal("expected an error for a zero AIFF sample rate")
	}
	var perr *ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *ParseError", err)
	}
}
