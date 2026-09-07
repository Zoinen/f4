package mediainfo

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

type rangeCountingSource struct {
	data   []byte
	size   int64
	bytes  int64
	maxEnd int64
}

func (s *rangeCountingSource) ReadAt(ctx context.Context, dst []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off < 0 || off >= s.size {
		return 0, io.EOF
	}
	n := len(dst)
	if remaining := s.size - off; int64(n) > remaining {
		n = int(remaining)
	}
	clear(dst[:n])
	if off < int64(len(s.data)) {
		copyEnd := int64(len(s.data))
		if copyEnd > off+int64(n) {
			copyEnd = off + int64(n)
		}
		copy(dst[:n], s.data[off:copyEnd])
	}
	s.bytes += int64(n)
	if end := off + int64(n); end > s.maxEnd {
		s.maxEnd = end
	}
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}

func TestMatroskaDirectTextFieldsAreBoundedAndOwned(t *testing.T) {
	const capBytes = 8
	long := []byte("abcdefgh0123456789-should-not-be-retained")
	doc := ebmlElement([]byte{0x42, 0x82}, []byte("matroska"))
	header := ebmlElement([]byte{0x1a, 0x45, 0xdf, 0xa3}, doc)

	infoPayload := ebmlElement([]byte{0x4d, 0x80}, long)
	infoPayload = append(infoPayload, ebmlElement([]byte{0x57, 0x41}, long)...)
	infoPayload = append(infoPayload, ebmlElement([]byte{0x7b, 0xa9}, long)...)
	info := ebmlElement([]byte{0x15, 0x49, 0xa9, 0x66}, infoPayload)

	trackPayload := ebmlElement([]byte{0x83}, []byte{2})
	for _, field := range []struct {
		id []byte
	}{
		{[]byte{0x53, 0x6e}},
		{[]byte{0x22, 0xb5, 0x9d}},
		{[]byte{0x86}},
		{[]byte{0x25, 0x86, 0x88}},
	} {
		trackPayload = append(trackPayload, ebmlElement(field.id, long)...)
	}
	entry := ebmlElement([]byte{0xae}, trackPayload)
	tracks := ebmlElement([]byte{0x16, 0x54, 0xae, 0x6b}, entry)

	displayPayload := ebmlElement([]byte{0x85}, long)
	displayPayload = append(displayPayload, ebmlElement([]byte{0x43, 0x7c}, long)...)
	display := ebmlElement([]byte{0x80}, displayPayload)
	chapter := ebmlElement([]byte{0xb6}, display)
	edition := ebmlElement([]byte{0x45, 0xb9}, chapter)
	chapters := ebmlElement([]byte{0x10, 0x43, 0xa7, 0x70}, edition)

	simplePayload := ebmlElement([]byte{0x45, 0xa3}, long)
	simplePayload = append(simplePayload, ebmlElement([]byte{0x44, 0x87}, long)...)
	simple := ebmlElement([]byte{0x67, 0xc8}, simplePayload)
	tag := ebmlElement([]byte{0x73, 0x73}, simple)
	tags := ebmlElement([]byte{0x12, 0x54, 0xc3, 0x67}, tag)

	segmentPayload := append(append(append(info, tracks...), chapters...), tags...)
	segment := ebmlElement([]byte{0x18, 0x53, 0x80, 0x67}, segmentPayload)
	data := append(header, segment...)
	opts := DefaultOptions(ModeDetailed)
	opts.MaxValueBytes = capBytes
	report, err := Analyze(context.Background(), Source{Name: "bounded.mkv", Size: int64(len(data)), Reader: memorySource(data)}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Truncated {
		t.Fatal("expected oversized Matroska text to mark the report truncated")
	}
	assertBounded := func(name, value string) {
		t.Helper()
		if len(value) > capBytes {
			t.Fatalf("%s retained %d bytes: %q", name, len(value), value)
		}
	}
	assertBounded("muxing app", report.General.MuxingApp)
	assertBounded("writing app", report.General.WritingApp)
	if len(report.Streams) != 1 {
		t.Fatalf("streams = %d", len(report.Streams))
	}
	stream := report.Streams[0]
	assertBounded("track title", stream.Title)
	assertBounded("track language", stream.Language)
	assertBounded("track codec ID", stream.CodecID)
	assertBounded("track codec name", stream.CodecName)
	if len(report.Chapters) != 1 {
		t.Fatalf("chapters = %d", len(report.Chapters))
	}
	assertBounded("chapter title", report.Chapters[0].Title)
	assertBounded("chapter language", report.Chapters[0].Language)
	for _, tag := range report.Tags {
		assertBounded("tag value", tag.Value)
	}

	// All retained strings must own their compact allocation, not alias the
	// source passed to Analyze.
	before := report.General.MuxingApp
	for i := range data {
		data[i] = 'z'
	}
	if report.General.MuxingApp != before {
		t.Fatalf("retained text aliases source storage: %q", report.General.MuxingApp)
	}
}

func TestMatroskaHugeUnknownLeafDoesNotReadPayload(t *testing.T) {
	doc := ebmlElement([]byte{0x42, 0x82}, []byte("matroska"))
	header := ebmlElement([]byte{0x1a, 0x45, 0xdf, 0xa3}, doc)
	const payloadSize = 1 << 20
	// Void (0xec) with a four-byte EBML size. Its payload is represented by
	// the sparse source and must never be requested by the parser.
	unknownHeader := []byte{0xec, 0x10, 0x10, 0x00, 0x00}
	prefix := append(append([]byte(nil), header...), unknownHeader...)
	payloadStart := int64(len(prefix))
	source := &rangeCountingSource{data: prefix, size: payloadStart + payloadSize}
	p, err := newProbe(context.Background(), Source{Name: "unknown.mkv", Size: source.size, Reader: source}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	if err := parseMatroska(p, nil); err != nil {
		t.Fatal(err)
	}
	if source.maxEnd > payloadStart {
		t.Fatalf("unknown leaf payload was read through offset %d (payload starts at %d)", source.maxEnd, payloadStart)
	}
	if source.bytes > 128 {
		t.Fatalf("unknown leaf required %d bytes of I/O", source.bytes)
	}
}

func TestMatroskaOversizedScalarLeavesDoNotReadPayload(t *testing.T) {
	source := &rangeCountingSource{size: 1 << 20}
	p, err := newProbe(context.Background(), Source{Name: "scalar.mkv", Size: source.size, Reader: source}, DefaultOptions(ModeFast))
	if err != nil {
		t.Fatal(err)
	}
	state := &ebmlState{}
	ctx := &ebmlContext{}
	if err := parseEBMLLeaf(p, state, 0x2ad7b1, 0, source.size, ctx); err != nil {
		t.Fatal(err)
	}
	if err := parseEBMLLeaf(p, state, 0x4489, 0, source.size, ctx); err != nil {
		t.Fatal(err)
	}
	if source.bytes != 0 {
		t.Fatalf("oversized scalar leaves read %d payload bytes", source.bytes)
	}
	if !p.report.Truncated {
		t.Fatal("expected oversized scalar fields to mark the report truncated")
	}
}

func TestVorbisVendorIsBoundedAndOwned(t *testing.T) {
	const capBytes = 8
	vendor := []byte("vendor-application-with-a-long-version")
	b := make([]byte, 4+len(vendor)+4)
	binary.LittleEndian.PutUint32(b[:4], mediaFixtureUint32(len(vendor)))
	copy(b[4:], vendor)
	p, err := newProbe(context.Background(), Source{Name: "comments", Reader: memorySource(nil)}, Options{Mode: ModeFast, MaxValueBytes: capBytes})
	if err != nil {
		t.Fatal(err)
	}
	parseVorbisComments(p, b)
	if got := p.report.General.WritingApp; got != string(vendor[:capBytes]) {
		t.Fatalf("WritingApp = %q", got)
	}
	if !p.report.Truncated {
		t.Fatal("expected oversized vendor to mark the report truncated")
	}
	got := p.report.General.WritingApp
	for i := 4; i < 4+len(vendor); i++ {
		b[i] = 'x'
	}
	if p.report.General.WritingApp != got {
		t.Fatalf("WritingApp aliases the comment packet: %q", p.report.General.WritingApp)
	}
}

func TestOggPacketAccumulatorIsBoundedAcrossPages(t *testing.T) {
	first := oggPage(7, 0, 0, []byte(strings.Repeat("a", 255)))
	second := oggPage(7, 0, 1, []byte(strings.Repeat("b", 255)))
	data := append(first, second...)
	secondPayloadStart := int64(len(first) + 28) // 27-byte header + one lacing byte.
	source := &rangeCountingSource{data: data, size: int64(len(data))}
	opts := DefaultOptions(ModeFast)
	opts.MaxSingleMetadataBytes = 300
	p, err := newProbe(context.Background(), Source{Name: "unterminated.ogg", Size: source.size, Reader: source}, opts)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = readOggPackets(p, 3)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("readOggPackets error = %v, want ErrLimit", err)
	}
	if source.maxEnd > secondPayloadStart {
		t.Fatalf("second page payload was read through offset %d (starts at %d)", source.maxEnd, secondPayloadStart)
	}
}
