package mediainfo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"strings"
	"testing"
	"time"
)

func mediaFixtureUint32(value int) uint32 {
	return uint32(value) // #nosec G115 -- generated media fixtures are bounded well below uint32 size fields.
}

func mediaFixtureUint64(value int) uint64 {
	return uint64(value) // #nosec G115 -- generated media fixtures have small non-negative buffer sizes.
}

func mediaFixtureByte(value int) byte {
	return byte(value) // #nosec G115 -- callers deliberately encode bounded fixture values into byte fields.
}

type memorySource []byte

func (m memorySource) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off < 0 || off >= int64(len(m)) {
		return 0, io.EOF
	}
	n := copy(p, m[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func analyzeBytes(t *testing.T, name string, b []byte, mode Mode) (Report, error) {
	t.Helper()
	return Analyze(context.Background(), Source{Name: name, Size: int64(len(b)), Reader: memorySource(b)}, DefaultOptions(mode))
}

func TestAnalyzeWave(t *testing.T) {
	b := make([]byte, 12+8+16+8+8000)
	copy(b[:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], mediaFixtureUint32(len(b)-8))
	copy(b[8:12], "WAVE")
	copy(b[12:16], "fmt ")
	binary.LittleEndian.PutUint32(b[16:20], 16)
	binary.LittleEndian.PutUint16(b[20:22], 1)
	binary.LittleEndian.PutUint16(b[22:24], 1)
	binary.LittleEndian.PutUint32(b[24:28], 8000)
	binary.LittleEndian.PutUint32(b[28:32], 8000)
	binary.LittleEndian.PutUint16(b[32:34], 1)
	binary.LittleEndian.PutUint16(b[34:36], 8)
	copy(b[36:40], "data")
	binary.LittleEndian.PutUint32(b[40:44], 8000)
	r, e := analyzeBytes(t, "tone.wav", b, ModeFast)
	if e != nil {
		t.Fatal(e)
	}
	if r.General.Format != "Wave" || len(r.Streams) != 1 {
		t.Fatalf("unexpected report: %#v", r)
	}
	if got := r.Streams[0].Audio.SampleRate; got != 8000 {
		t.Fatalf("sample rate=%d", got)
	}
	if r.General.Duration != time.Second {
		t.Fatalf("duration=%v", r.General.Duration)
	}
}

func TestAnalyzeFLACStreamInfo(t *testing.T) {
	b := make([]byte, 4+4+34)
	copy(b, "fLaC")
	b[4] = 0x80
	b[7] = 34
	rate := uint64(48000)
	channels := uint64(1)
	bits := uint64(23)
	samples := uint64(96000)
	v := rate<<44 | channels<<41 | bits<<36 | samples
	binary.BigEndian.PutUint64(b[18:26], v)
	r, e := analyzeBytes(t, "tone.flac", b, ModeFast)
	if e != nil {
		t.Fatal(e)
	}
	s := r.Streams[0]
	if s.Audio.SampleRate != 48000 || s.Audio.Channels != 2 || s.Audio.BitDepth != 24 {
		t.Fatalf("bad stream: %#v", s)
	}
	if s.Duration != 2*time.Second {
		t.Fatalf("duration=%v", s.Duration)
	}
}

func TestAnalyzeMPEGAudio(t *testing.T) {
	frame := make([]byte, 417)
	copy(frame, []byte{0xff, 0xfb, 0x90, 0x00})
	data := append(append([]byte(nil), frame...), frame...)
	r, e := analyzeBytes(t, "tone.mp3", data, ModeFast)
	if e != nil {
		t.Fatal(e)
	}
	s := r.Streams[0]
	if s.Audio.SampleRate != 44100 || s.Audio.Channels != 2 || s.BitRate != 128000 {
		t.Fatalf("bad MP3 stream: %#v", s)
	}
}

func oggPage(serial uint32, granule uint64, seq uint32, payload []byte) []byte {
	segments := (len(payload) + 254) / 255
	if len(payload) == 0 {
		segments = 0
	}
	b := make([]byte, 27+segments+len(payload))
	copy(b, "OggS")
	b[4] = 0
	binary.LittleEndian.PutUint64(b[6:14], granule)
	binary.LittleEndian.PutUint32(b[14:18], serial)
	binary.LittleEndian.PutUint32(b[18:22], seq)
	b[26] = byte(segments)
	left := len(payload)
	for i := 0; i < segments; i++ {
		n := left
		if n > 255 {
			n = 255
		}
		b[27+i] = byte(n)
		left -= n
	}
	copy(b[27+segments:], payload)
	return b
}
func TestAnalyzeOggOpus(t *testing.T) {
	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8] = 1
	head[9] = 2
	binary.LittleEndian.PutUint16(head[10:12], 312)
	comments := make([]byte, 8)
	binary.LittleEndian.PutUint32(comments[:4], 0)
	binary.LittleEndian.PutUint32(comments[4:8], 0)
	tags := append([]byte("OpusTags"), comments...)
	data := append(oggPage(7, 0, 0, head), oggPage(7, 0, 1, tags)...)
	data = append(data, oggPage(7, 48000, 2, nil)...)
	r, e := analyzeBytes(t, "tone.ogg", data, ModeFast)
	if e != nil {
		t.Fatal(e)
	}
	s := r.Streams[0]
	if s.Format != "Opus" || s.Audio.SampleRate != 48000 || s.Duration != time.Second {
		t.Fatalf("bad Ogg stream: %#v", s)
	}
}

func TestAnalyzeSubRip(t *testing.T) {
	b := []byte("1\r\n00:00:01,000 --> 00:00:02,500\r\nHello\r\n\r\n2\r\n00:00:03,000 --> 00:00:04,000\r\nWorld\r\n")
	r, e := analyzeBytes(t, "captions.srt", b, ModeDetailed)
	if e != nil {
		t.Fatal(e)
	}
	text := r.Streams[0].Text
	if text.CueCount != 2 || text.FirstCue != time.Second || text.LastCue != 4*time.Second {
		t.Fatalf("bad text report: %#v", text)
	}
}

func TestAnalyzePNG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.Set(1, 1, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	if e := png.Encode(&b, img); e != nil {
		t.Fatal(e)
	}
	r, e := analyzeBytes(t, "image.png", b.Bytes(), ModeFast)
	if e != nil {
		t.Fatal(e)
	}
	if r.Streams[0].Image.Width != 3 || r.Streams[0].Image.Height != 2 {
		t.Fatalf("bad image: %#v", r.Streams[0].Image)
	}
}

func TestAnalyzeMinimalMP4(t *testing.T) {
	box := func(typ string, payload []byte) []byte {
		x := make([]byte, 8+len(payload))
		binary.BigEndian.PutUint32(x[:4], mediaFixtureUint32(len(x)))
		copy(x[4:8], typ)
		copy(x[8:], payload)
		return x
	}
	ftypPayload := append([]byte("isom\x00\x00\x00\x00"), []byte("isommp42")...)
	ftyp := box("ftyp", ftypPayload)
	mvhd := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd[12:16], 1000)
	binary.BigEndian.PutUint32(mvhd[16:20], 2500)
	moov := box("moov", box("mvhd", mvhd))
	data := append(ftyp, moov...)
	r, e := analyzeBytes(t, "clip.mp4", data, ModeFast)
	if e != nil {
		t.Fatal(e)
	}
	if r.General.Format != "MPEG-4" || r.General.Duration != 2500*time.Millisecond {
		t.Fatalf("bad MP4 report: %#v", r.General)
	}
}

func ebmlSize(n int) []byte {
	if n < 127 {
		return []byte{0x80 | mediaFixtureByte(n)}
	}
	return []byte{0x40 | mediaFixtureByte(n>>8), mediaFixtureByte(n)}
}
func ebmlElement(id, payload []byte) []byte {
	b := append(append([]byte(nil), id...), ebmlSize(len(payload))...)
	return append(b, payload...)
}
func TestAnalyzeMatroska(t *testing.T) {
	doc := ebmlElement([]byte{0x42, 0x82}, []byte("matroska"))
	header := ebmlElement([]byte{0x1a, 0x45, 0xdf, 0xa3}, doc)
	scale := ebmlElement([]byte{0x2a, 0xd7, 0xb1}, []byte{0x0f, 0x42, 0x40})
	dur := make([]byte, 8)
	binary.BigEndian.PutUint64(dur, math.Float64bits(2000))
	info := ebmlElement([]byte{0x15, 0x49, 0xa9, 0x66}, append(scale, ebmlElement([]byte{0x44, 0x89}, dur)...))
	typ := ebmlElement([]byte{0x83}, []byte{2})
	codec := ebmlElement([]byte{0x86}, []byte("A_OPUS"))
	rate := make([]byte, 8)
	binary.BigEndian.PutUint64(rate, math.Float64bits(48000))
	audio := ebmlElement([]byte{0xe1}, append(ebmlElement([]byte{0xb5}, rate), ebmlElement([]byte{0x9f}, []byte{2})...))
	entry := ebmlElement([]byte{0xae}, append(append(typ, codec...), audio...))
	tracks := ebmlElement([]byte{0x16, 0x54, 0xae, 0x6b}, entry)
	segment := ebmlElement([]byte{0x18, 0x53, 0x80, 0x67}, append(info, tracks...))
	data := append(header, segment...)
	r, e := analyzeBytes(t, "clip.mkv", data, ModeFast)
	if e != nil {
		t.Fatal(e)
	}
	if r.General.Duration != 2*time.Second || len(r.Streams) != 1 || r.Streams[0].Format != "Opus" {
		t.Fatalf("bad Matroska report: %#v", r)
	}
}

func TestAnalyzeMalformedRIFF(t *testing.T) {
	b := []byte("RIFF\xff\xff\xff\x7fWAVEfmt \xff\xff\xff\x7f")
	_, e := analyzeBytes(t, "bad.wav", b, ModeFast)
	var pe *ParseError
	if !errors.As(e, &pe) {
		t.Fatalf("expected ParseError, got %v", e)
	}
}

func TestAnalyzeHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := Analyze(ctx, Source{Name: "x.wav", Size: 12, Reader: memorySource(make([]byte, 12))}, DefaultOptions(ModeFast))
	if !errors.Is(e, context.Canceled) {
		t.Fatalf("error=%v", e)
	}
}

func TestAnalyzeWithBackgroundContext(t *testing.T) {
	data := []byte("RIFF\x04\x00\x00\x00WAVE")
	if _, err := Analyze(context.Background(), Source{Name: "background-context.wav", Size: int64(len(data)), Reader: memorySource(data)}, DefaultOptions(ModeFast)); err != nil {
		t.Fatalf("Analyze() = %v", err)
	}
}

func TestAnalyzeReadLimitReturnsPartial(t *testing.T) {
	b := make([]byte, 12+8+16)
	copy(b[:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], mediaFixtureUint32(len(b)-8))
	copy(b[8:12], "WAVE")
	copy(b[12:16], "fmt ")
	binary.LittleEndian.PutUint32(b[16:20], 16)
	opts := DefaultOptions(ModeFast)
	opts.MaxReadBytes = 14
	r, e := Analyze(context.Background(), Source{Name: "x.wav", Size: int64(len(b)), Reader: memorySource(b)}, opts)
	if e != nil {
		t.Fatal(e)
	}
	if !r.Truncated {
		t.Fatal("expected a partial report")
	}
}

func TestRenderAndTemplate(t *testing.T) {
	r := Report{General: General{FileName: "a.flac", FileSize: 1024, Format: "FLAC", Duration: time.Second}, Streams: []Stream{{ID: "1", Kind: StreamAudio, Format: "FLAC", Audio: &Audio{Channels: 2, SampleRate: 48000}}}, Tags: []Field{{Name: "Artist", Value: "First"}, {Name: "Artist", Value: "Second"}}}
	text := RenderText(r, RenderOptions{})
	if !strings.Contains(text, "General") || !strings.Contains(text, "Audio #1") {
		t.Fatalf("rendered text:\n%s", text)
	}
	ru := RenderText(r, RenderOptions{Language: "ru"})
	if !strings.Contains(ru, "Общее") || !strings.Contains(ru, "Имя файла") {
		t.Fatalf("Russian render:\n%s", ru)
	}
	compact := RenderText(r, RenderOptions{Compact: true})
	if strings.Contains(compact, "File name") || strings.Contains(compact, "Compression mode") {
		t.Fatalf("compact render contains full-only fields:\n%s", compact)
	}
	got, e := ExecuteTemplate(r, "%CompleteName%: %Format%")
	if e != nil || got != "a.flac: FLAC" {
		t.Fatalf("template=%q err=%v", got, e)
	}
	inform := "General;%Format%[ by %Artist%]\\n\nAudio_Begin;audio:\\n\nAudio;%Format% %Channel_s_%ch\\n\nAudio_End;done\\n"
	got, e = ExecuteTemplate(r, inform)
	if e != nil || got != "FLAC by First\naudio:\nFLAC 2ch\ndone\n" {
		t.Fatalf("Inform=%q err=%v", got, e)
	}
	got, e = ExecuteTemplate(r, `{{.General.Format}}/{{len .Streams}}`)
	if e != nil || got != "FLAC/1" {
		t.Fatalf("go template=%q err=%v", got, e)
	}
	sections := CanonicalSections(r)
	artists := 0
	for _, f := range sections[0].Fields {
		if f.Name == "Artist" {
			artists++
		}
	}
	if artists != 2 {
		t.Fatalf("duplicate tags lost: %#v", sections[0].Fields)
	}
}

func TestTemplateOutputIsBoundedDuringExecution(t *testing.T) {
	report := Report{Chapters: make([]Chapter, 3000)}
	payload := strings.Repeat("x", 1024)

	if _, err := ExecuteTemplate(report, "Menu;"+payload); !errors.Is(err, errTemplateOutputTooLarge) {
		t.Fatalf("Inform template error = %v, want output limit", err)
	}
	if _, err := ExecuteTemplate(report, "{{range .Chapters}}"+payload+"{{end}}"); !errors.Is(err, errTemplateOutputTooLarge) {
		t.Fatalf("Go template error = %v, want output limit", err)
	}
}

func FuzzAnalyzeDoesNotPanic(f *testing.F) {
	f.Add("sample.bin", []byte("not media"))
	f.Add("sample.wav", []byte("RIFF\x04\x00\x00\x00WAVE"))
	f.Fuzz(func(t *testing.T, name string, b []byte) {
		if len(b) > 1<<20 {
			t.Skip()
		}
		opts := DefaultOptions(ModeFast)
		opts.MaxReadBytes = 2 << 20
		opts.MaxReadOps = 10000
		_, _ = Analyze(context.Background(), Source{Name: name, Size: int64(len(b)), Reader: memorySource(b)}, opts)
	})
}
