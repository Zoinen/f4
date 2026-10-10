//go:build !noffi && !lite && !android && (windows || ((linux || darwin || freebsd) && (amd64 || arm64)))

package media

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeWAVCoverageRejectsBrokenHeadersAndFormats(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("short"),
		[]byte("RIFF\x00\x00\x00\x00NOPE"),
	} {
		path := filepath.Join(t.TempDir(), "broken.wav")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := openAudioSource(path, 0); err == nil {
			t.Fatalf("broken WAV %q was accepted", data)
		}
	}

	chunk := func(id string, body []byte) []byte {
		var b bytes.Buffer
		b.WriteString(id)
		//nolint:gosec // test chunks are intentionally built from bounded slices.
		_ = binary.Write(&b, binary.LittleEndian, uint32(len(body)))
		b.Write(body)
		return b.Bytes()
	}
	header := []byte("RIFF\x00\x00\x00\x00WAVE")
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"data before fmt", append(append([]byte{}, header...), chunk("data", []byte{0, 0})...), "data before fmt"},
		{"short fmt", append(append([]byte{}, header...), chunk("fmt ", make([]byte, 4))...), "fmt chunk too short"},
		{"no data", append(append([]byte{}, header...), chunk("fmt ", make([]byte, 16))...), "no data chunk"},
		{"bad format", wavBytes(0x11, 1, 8000, 4, []byte{0, 0}), "unsupported"},
		{"bad channels", wavBytes(1, 0, 8000, 16, nil), "unusable format"},
		{"bad rate", wavBytes(1, 1, 0, 16, nil), "unusable format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "case.wav")
			if err := os.WriteFile(path, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := openAudioSource(path, 0); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDecodeWAVCoverageFloatExtensibleAndUnknownChunk(t *testing.T) {
	floatPath := filepath.Join(t.TempDir(), "float.wav")
	floatData := make([]byte, 4)
	binary.LittleEndian.PutUint32(floatData, 0x3fc00000) // 1.5, clamped to 1
	if err := os.WriteFile(floatPath, wavBytes(3, 1, 8000, 32, floatData), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := openAudioSource(floatPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(src)
	src.Close()
	if err != nil || len(out) != AudioBytesPerFrame || binary.LittleEndian.Uint16(out) != uint16(32767) {
		t.Fatalf("float WAV output = %v, err=%v", out, err)
	}

	extensible := make([]byte, 40)
	binary.LittleEndian.PutUint16(extensible[0:], 0xFFFE)
	binary.LittleEndian.PutUint16(extensible[2:], 1)
	binary.LittleEndian.PutUint32(extensible[4:], 8000)
	binary.LittleEndian.PutUint16(extensible[14:], 16)
	binary.LittleEndian.PutUint16(extensible[24:], 1)
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(0))
	b.WriteString("WAVE")
	b.WriteString("JUNK")
	_ = binary.Write(&b, binary.LittleEndian, uint32(3))
	b.Write([]byte{1, 2, 3})
	b.WriteByte(0)
	b.WriteString("fmt ")
	//nolint:gosec // the test fixture is a fixed, bounded WAV chunk.
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(extensible)))
	b.Write(extensible)
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(2))
	b.Write([]byte{0, 0})
	extensiblePath := filepath.Join(t.TempDir(), "extensible.wav")
	if err := os.WriteFile(extensiblePath, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err = openAudioSource(extensiblePath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if src.Rate != 8000 || !src.Mono {
		t.Fatalf("extensible metadata = rate %d mono %v", src.Rate, src.Mono)
	}
	src.Close()
}

func TestWAVPCM32AndShortReads(t *testing.T) {
	var frame [4]byte
	binary.LittleEndian.PutUint32(frame[:], 0xffff0000)
	w := &wavPCM{r: bytes.NewReader(frame[:]), bits: 32, channels: 1, left: int64(len(frame)), frame: make([]byte, 4)}
	out := make([]byte, AudioBytesPerFrame)
	if n, err := w.Read(out); n != AudioBytesPerFrame || err != nil || binary.LittleEndian.Uint16(out) != uint16(0xffff) {
		t.Fatalf("32-bit sample = n%d err%v bytes%v", n, err, out)
	}
	if n, err := w.Read(out); n != 0 || err != io.EOF {
		t.Fatalf("exhausted WAV reader = n%d err%v", n, err)
	}

	short := &wavPCM{r: strings.NewReader("\x00"), bits: 16, channels: 1, left: 2, frame: make([]byte, 2)}
	if n, err := short.Read(out); n != 0 || err != io.EOF {
		t.Fatalf("short WAV reader = n%d err%v", n, err)
	}
	for _, tc := range []struct {
		bits int
		data []byte
		want int16
	}{
		{8, []byte{255}, 127 << 8},
		{16, []byte{0x34, 0x12}, 0x1234},
		{24, []byte{0x56, 0x34, 0x12}, 0x1234},
		{32, []byte{0, 0, 1, 0}, 1},
	} {
		got := (&wavPCM{bits: tc.bits, float: false}).sample(tc.data)
		if got != tc.want {
			t.Errorf("sample(%d) = %d, want %d", tc.bits, got, tc.want)
		}
	}
}
