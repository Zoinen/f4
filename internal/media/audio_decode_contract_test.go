package media

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWAVPCMScalarConversions(t *testing.T) {
	tests := []struct {
		name string
		pcm  wavPCM
		data []byte
		want int16
	}{
		{name: "float", pcm: wavPCM{bits: 32, float: true}, data: float32LE(0.5), want: 16383},
		{name: "float clamps high", pcm: wavPCM{bits: 32, float: true}, data: float32LE(2), want: 32767},
		{name: "float clamps low", pcm: wavPCM{bits: 32, float: true}, data: float32LE(-2), want: -32767},
		{name: "unsigned eight bit", pcm: wavPCM{bits: 8}, data: []byte{255}, want: 127 << 8},
		{name: "signed sixteen bit", pcm: wavPCM{bits: 16}, data: []byte{0x34, 0x12}, want: 0x1234},
		{name: "twenty four bit", pcm: wavPCM{bits: 24}, data: []byte{0x78, 0x56, 0x34}, want: 0x3456},
		{name: "signed thirty two bit", pcm: wavPCM{bits: 32}, data: []byte{0, 0, 0x34, 0x12}, want: 0x1234},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pcm.sample(tt.data); got != tt.want {
				gotBits := uint16(got)      // #nosec G115 -- the conversion is only for displaying the PCM bit pattern.
				wantBits := uint16(tt.want) // #nosec G115 -- the conversion is only for displaying the PCM bit pattern.
				t.Fatalf("sample=%d (%#x), want %d (%#x)", got, gotBits, tt.want, wantBits)
			}
		})
	}
}

func float32LE(v float32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
	return b[:]
}

func TestWAVPCMReadBoundaries(t *testing.T) {
	w := &wavPCM{
		r:        bytes.NewReader([]byte{0x34, 0x12, 0x78, 0x56}),
		bits:     16,
		channels: 1,
		left:     4,
		frame:    make([]byte, 2),
	}

	short := make([]byte, AudioBytesPerFrame-1)
	if n, err := w.Read(short); n != 0 || err != nil {
		t.Fatalf("short destination: n=%d err=%v", n, err)
	}
	out := make([]byte, AudioBytesPerFrame*2)
	n, err := w.Read(out)
	if n != len(out) || err != nil {
		t.Fatalf("first read: n=%d err=%v", n, err)
	}
	if got := binary.LittleEndian.Uint16(out[2:]); got != 0x1234 {
		t.Fatalf("mono right sample=%#x, want %#x", got, uint16(0x1234))
	}
	if n, err := w.Read(out); n != 0 || err != io.EOF {
		t.Fatalf("after end: n=%d err=%v", n, err)
	}

	partial := &wavPCM{
		r:        bytes.NewReader([]byte{0x34}),
		bits:     16,
		channels: 1,
		left:     2,
		frame:    make([]byte, 2),
	}
	if n, err := partial.Read(out); n != 0 || err != io.EOF {
		t.Fatalf("truncated frame: n=%d err=%v", n, err)
	}
}

func TestDecodeWAVRejectsMalformedHeaders(t *testing.T) {
	validHeader := func(body []byte) []byte {
		b := append([]byte("RIFF\x00\x00\x00\x00WAVE"), body...)
		return b
	}
	chunk := func(id string, body []byte) []byte {
		var b bytes.Buffer
		b.WriteString(id)
		_ = binary.Write(&b, binary.LittleEndian, uint32(len(body))) // #nosec G115 -- synthetic chunks are tiny test fixtures.
		b.Write(body)
		return b.Bytes()
	}
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "short header", data: []byte("RIFF"), want: "wav:"},
		{name: "wrong container", data: []byte("NOPE\x00\x00\x00\x00WAVE"), want: "not a RIFF/WAVE"},
		{name: "short fmt", data: validHeader(chunk("fmt ", []byte{1, 0})), want: "fmt chunk too short"},
		{name: "truncated fmt", data: validHeader([]byte("fmt \x10\x00\x00\x00\x01")), want: "fmt chunk"},
		{name: "data before fmt", data: validHeader(chunk("data", []byte{0, 0})), want: "data before fmt"},
		{name: "no data chunk", data: validHeader(chunk("fmt ", wavFmtBody(1, 1, 8000, 8))), want: "no data chunk"},
		{name: "zero channels", data: wavBytes(1, 0, 8000, 8, []byte{0}), want: "unusable format"},
		{name: "zero rate", data: wavBytes(1, 1, 0, 8, []byte{0}), want: "unusable format"},
		{name: "unsupported float width", data: wavBytes(3, 1, 8000, 16, []byte{0, 0}), want: "unsupported encoding"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.wav")
			if err := os.WriteFile(path, tt.data, 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = decodeWAV(f)
			_ = f.Close()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v, want substring %q", err, tt.want)
			}
		})
	}
}

func wavFmtBody(format, channels, rate, bits int) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[0:], uint16(format))   // #nosec G115 -- synthetic WAV fields use bounded fixture values.
	binary.LittleEndian.PutUint16(b[2:], uint16(channels)) // #nosec G115 -- synthetic WAV fields use bounded fixture values.
	binary.LittleEndian.PutUint32(b[4:], uint32(rate))     // #nosec G115 -- synthetic WAV fields use bounded fixture values.
	binary.LittleEndian.PutUint16(b[14:], uint16(bits))    // #nosec G115 -- synthetic WAV fields use bounded fixture values.
	return b
}

func TestAMRParserTruncationAndUnknownTypes(t *testing.T) {
	var b bytes.Buffer
	b.WriteString(amrNBMagic)
	b.WriteByte(7 << 3)
	b.Write(make([]byte, amrNBFrameSizes[7]-1))
	b.WriteByte(15 << 3) // legal no-payload frame type
	b.WriteByte(0 << 3)
	b.Write(make([]byte, 3))
	info, ok := parseAMR(bufio.NewReader(&b))
	if !ok || info.Frames != 2 || info.Duration != 40*time.Millisecond {
		t.Fatalf("truncated AMR=%+v ok=%v", info, ok)
	}
	if _, ok := parseAMR(bufio.NewReader(strings.NewReader("#!AMR"))); ok {
		t.Fatal("incomplete magic must not be recognized")
	}
	if _, ok := amrFileInfo(filepath.Join(t.TempDir(), "missing.amr")); ok {
		t.Fatal("missing AMR file must not be recognized")
	}
}

func TestAudioMetadataAndSourceCloseContracts(t *testing.T) {
	pi := parseFFprobeOutput("codec_name=\nchannels=bad\nduration=0\nduration=-1\nmalformed\n")
	if pi.Codec != "" || pi.Channels != 0 || pi.Duration != 0 {
		t.Fatalf("invalid probe values produced %+v", pi)
	}

	closed := 0
	src := &audioSource{closer: func() { closed++ }}
	src.Close()
	src.Close()
	if closed != 1 {
		t.Fatalf("closer called %d times, want once", closed)
	}
	var nilSource *audioSource
	nilSource.Close()

	if _, err := openAudioSource(filepath.Join(t.TempDir(), "missing.mp3"), 0); err == nil {
		t.Fatal("missing native audio file must fail")
	}
	if _, err := openAudioSource(filepath.Join(t.TempDir(), "track.txt"), 0); err == nil || !strings.Contains(err.Error(), "not an audio file") {
		t.Fatalf("unknown audio extension error=%v", err)
	}
}
