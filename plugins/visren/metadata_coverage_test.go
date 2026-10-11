package visren

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestReadMetadataDispatchesByExtension exercises readMetadata's per-extension
// dispatch on real files: a legacy-tagged MP3, a real PNG image and an
// (invalid) EXE, checking that each branch's result actually lands in the
// merged Metadata value it returns.
func TestReadMetadataDispatchesByExtension(t *testing.T) {
	dir := t.TempDir()
	mtime := time.Date(2024, 5, 6, 7, 8, 9, 0, time.Local)

	mp3 := filepath.Join(dir, "song.mp3")
	data := make([]byte, 256)
	tag := data[len(data)-128:]
	copy(tag[:3], "TAG")
	copy(tag[3:33], "Song")
	tag[125], tag[126], tag[127] = 0, 7, 17
	if err := os.WriteFile(mp3, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if meta, err := readMetadata(mp3, mtime); err != nil || meta.Title != "Song" || meta.Track != "7" {
		t.Fatalf("mp3 metadata = %+v, %v", meta, err)
	}

	pngPath := filepath.Join(dir, "photo.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pngPath, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if meta, err := readMetadata(pngPath, mtime); err != nil || meta.Width != 4 || meta.Height != 3 {
		t.Fatalf("png metadata = %+v, %v", meta, err)
	}

	// The exe branch assigns whatever readPEVersion returns even when the
	// file is not a real PE image (the error is deliberately discarded), so
	// readMetadata itself must still report success with an empty version.
	exe := filepath.Join(dir, "app.exe")
	if err := os.WriteFile(exe, []byte("not a real PE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if meta, err := readMetadata(exe, mtime); err != nil || meta.Version != "" {
		t.Fatalf("exe metadata = %+v, %v", meta, err)
	}
}

func buildID3v2Tag(t *testing.T, version, flags byte, size [4]byte, extra []byte) string {
	t.Helper()
	header := []byte{'I', 'D', '3', version, 0, flags, size[0], size[1], size[2], size[3]}
	data := append(header, extra...)
	path := filepath.Join(t.TempDir(), "tag.mp3")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestReadID3v2HeaderEdgeCases covers the header-validation branches of
// readID3: unsupported tag versions, a zero-length tag body and a body
// shorter than the declared tag size (os.File.ReadAt reports the short read
// as io.EOF, which readID3 treats as "no more data", not as a hard error, so
// it must fall back to the frame loop discarding the oversized/garbage frame
// it finds instead of failing).
func TestReadID3v2HeaderEdgeCases(t *testing.T) {
	if meta, err := readID3(buildID3v2Tag(t, 2, 0, [4]byte{}, nil)); err != nil || meta != (Metadata{}) {
		t.Fatalf("unsupported ID3v2 version 2 = %+v, %v", meta, err)
	}
	if meta, err := readID3(buildID3v2Tag(t, 5, 0, [4]byte{}, nil)); err != nil || meta != (Metadata{}) {
		t.Fatalf("unsupported ID3v2 version 5 = %+v, %v", meta, err)
	}
	if meta, err := readID3(buildID3v2Tag(t, 3, 0, [4]byte{0, 0, 0, 0}, nil)); err != nil || meta != (Metadata{}) {
		t.Fatalf("zero-size ID3v2 tag = %+v, %v", meta, err)
	}
	if meta, err := readID3(buildID3v2Tag(t, 3, 0, [4]byte{0, 0, 0, 100}, []byte("short"))); err != nil || meta != (Metadata{}) {
		t.Fatalf("truncated ID3v2 body = %+v, %v", meta, err)
	}
}

// TestReadID3v2UnsynchronisationShortensBody covers the unsynchronisation
// flag branch: a two-byte 0xff 0x00 body collapses to a single 0xff byte,
// too short to contain any frame, so readID3 must return an empty result
// without error.
func TestReadID3v2UnsynchronisationShortensBody(t *testing.T) {
	header := []byte{'I', 'D', '3', 3, 0, 0x80, 0, 0, 0, 2}
	data := append(header, 0xff, 0x00)
	path := filepath.Join(t.TempDir(), "tag.mp3")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := readID3(path)
	if err != nil || meta != (Metadata{}) {
		t.Fatalf("unsynchronised short tag = %+v, %v", meta, err)
	}
}

// TestReadID3v2ExtendedHeaderIsSkipped covers the extended-header-size
// branch: readID3 must skip past the declared extended header and still
// parse the TIT2 frame that follows it.
func TestReadID3v2ExtendedHeaderIsSkipped(t *testing.T) {
	extHeader := []byte{0, 0, 0, 6, 0, 0} // declared extended-header size = 6 bytes
	buildFrame := func(id, text string) []byte {
		payload := append([]byte{3}, []byte(text)...)
		frame := make([]byte, 10+len(payload))
		copy(frame[:4], id)
		binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
		copy(frame[10:], payload)
		return frame
	}
	body := append(append([]byte{}, extHeader...), buildFrame("TIT2", "Ext Title")...)
	header := []byte{'I', 'D', '3', 3, 0, 0x40, 0, 0, 0, byte(len(body))}
	data := append(header, body...)
	path := filepath.Join(t.TempDir(), "tag.mp3")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := readID3(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Ext Title" {
		t.Fatalf("title after skipping extended header = %q", meta.Title)
	}
}

// TestReadID3v2AllTextFrameTypes covers the TPE1/TALB/TYER/TCON frame-id
// branches of the ID3v2 frame switch; the existing suite only exercises
// TIT2 and TRCK.
func TestReadID3v2AllTextFrameTypes(t *testing.T) {
	buildFrame := func(id, text string) []byte {
		payload := append([]byte{3}, []byte(text)...)
		frame := make([]byte, 10+len(payload))
		copy(frame[:4], id)
		binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
		copy(frame[10:], payload)
		return frame
	}
	var body []byte
	body = append(body, buildFrame("TPE1", "Artist X")...)
	body = append(body, buildFrame("TALB", "Album Y")...)
	body = append(body, buildFrame("TYER", "1999")...)
	body = append(body, buildFrame("TCON", "17")...)
	header := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, byte(len(body))}
	data := append(header, body...)
	path := filepath.Join(t.TempDir(), "tag.mp3")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := readID3(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Artist != "Artist X" || meta.Album != "Album Y" || meta.Year != "1999" || meta.Genre != "Rock" {
		t.Fatalf("metadata = %+v", meta)
	}
}

// TestDecodeID3TextUTF16LittleEndianBOM covers the FF FE (little-endian)
// byte-order-mark branch of decodeID3Text; the existing suite only exercises
// the FE FF (big-endian) mark.
func TestDecodeID3TextUTF16LittleEndianBOM(t *testing.T) {
	payload := []byte{1, 0xff, 0xfe, 'H', 0, 'i', 0, 0, 0}
	if got := decodeID3Text(payload); got != "Hi" {
		t.Fatalf("utf16 le bom decode = %q", got)
	}
}

func TestReadImageMetadataDecodeError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.png")
	if err := os.WriteFile(path, []byte("not a real image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readImageMetadata(path, time.Unix(0, 0)); err == nil {
		t.Fatal("corrupted image was accepted")
	}
}

// TestReadImageMetadataAppliesJPEGExifDate builds a real, decodable JPEG
// (via the standard encoder) with a synthetic Exif APP1 segment spliced in
// right after the SOI marker, so both image.DecodeConfig and parseJPEGEXIF
// accept it, and checks that a present EXIF date overrides the mtime-based
// default.
func TestReadImageMetadataAppliesJPEGExifDate(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 4, 4))
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, nil); err != nil {
		t.Fatal(err)
	}
	base := encoded.Bytes()
	if len(base) < 2 || base[0] != 0xff || base[1] != 0xd8 {
		t.Fatalf("unexpected JPEG signature: % x", base[:2])
	}

	order := binary.BigEndian
	tiff := make([]byte, 100)
	copy(tiff[:2], "MM")
	order.PutUint16(tiff[2:4], 42)
	order.PutUint32(tiff[4:8], 8)
	order.PutUint16(tiff[8:10], 1)
	order.PutUint16(tiff[10:12], 0x9003) // DateTimeOriginal
	order.PutUint16(tiff[12:14], 2)      // ASCII
	order.PutUint32(tiff[14:18], 20)
	order.PutUint32(tiff[18:22], 56)
	copy(tiff[56:], "2025:06:07 08:09:10\x00")
	section := append([]byte("Exif\x00\x00"), tiff...)
	// #nosec G115 -- the synthetic EXIF section is far below the JPEG segment limit.
	app1 := append([]byte{0xff, 0xe1, byte((len(section) + 2) >> 8), byte(len(section) + 2)}, section...)

	full := append(append([]byte{}, base[:2]...), app1...)
	full = append(full, base[2:]...)

	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, full, 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := readImageMetadata(path, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Width != 4 || meta.Height != 4 {
		t.Fatalf("dimensions = %dx%d", meta.Width, meta.Height)
	}
	if meta.ImageDate != "2025.06.07 08-09-10" {
		t.Fatalf("image date = %q", meta.ImageDate)
	}
}

func TestParseJPEGEXIFMalformedMarkerStream(t *testing.T) {
	data := []byte{0xff, 0xd8, 0x00, 0x00, 0x00, 0x00}
	if make_, model, date := parseJPEGEXIF(data); make_ != "" || model != "" || date != "" {
		t.Fatalf("non-marker byte mid-stream was accepted: %q %q %q", make_, model, date)
	}
}

func TestParseJPEGEXIFStopsAtScanMarkerAfterSkippingSegment(t *testing.T) {
	data := []byte{
		0xff, 0xd8, // SOI
		0xff, 0xe0, 0x00, 0x04, 0x00, 0x00, // APP0, size 4 (2 content bytes)
		0xff, 0xda, // SOS
		0x00, 0x00, // padding so the loop still sees the SOS marker
	}
	if make_, model, date := parseJPEGEXIF(data); make_ != "" || model != "" || date != "" {
		t.Fatalf("scan marker was not treated as end of headers: %q %q %q", make_, model, date)
	}
}

func TestParseJPEGEXIFRejectsUndersizedSegment(t *testing.T) {
	data := []byte{0xff, 0xd8, 0xff, 0xe1, 0x00, 0x00}
	if make_, model, date := parseJPEGEXIF(data); make_ != "" || model != "" || date != "" {
		t.Fatalf("undersized segment was accepted: %q %q %q", make_, model, date)
	}
}

// TestParseTIFFEdgeCases covers parseTIFF branches not reached by the
// existing suite: an unrecognised byte-order mark on an otherwise
// long-enough buffer, an out-of-range IFD offset, a non-ASCII entry type
// that must be skipped, and an entry whose value offset runs past the end
// of the buffer.
func TestParseTIFFEdgeCases(t *testing.T) {
	if make_, model, date := parseTIFF([]byte("XXaaaaaa")); make_ != "" || model != "" || date != "" {
		t.Fatalf("unknown byte order was accepted: %q %q %q", make_, model, date)
	}

	oobIFD := make([]byte, 8)
	copy(oobIFD[:2], "II")
	binary.LittleEndian.PutUint16(oobIFD[2:4], 42)
	binary.LittleEndian.PutUint32(oobIFD[4:8], 1000)
	if make_, _, _ := parseTIFF(oobIFD); make_ != "" {
		t.Fatalf("out-of-range IFD offset was accepted: %q", make_)
	}

	wrongType := make([]byte, 22)
	copy(wrongType[:2], "II")
	binary.LittleEndian.PutUint16(wrongType[2:4], 42)
	binary.LittleEndian.PutUint32(wrongType[4:8], 8)
	binary.LittleEndian.PutUint16(wrongType[8:10], 1)
	binary.LittleEndian.PutUint16(wrongType[10:12], 0x010f)
	binary.LittleEndian.PutUint16(wrongType[12:14], 3) // SHORT, not ASCII
	binary.LittleEndian.PutUint32(wrongType[14:18], 1)
	binary.LittleEndian.PutUint32(wrongType[18:22], 5)
	if make_, _, _ := parseTIFF(wrongType); make_ != "" {
		t.Fatalf("non-ASCII entry type was not skipped: %q", make_)
	}

	oobValue := make([]byte, 30)
	copy(oobValue[:2], "II")
	binary.LittleEndian.PutUint16(oobValue[2:4], 42)
	binary.LittleEndian.PutUint32(oobValue[4:8], 8)
	binary.LittleEndian.PutUint16(oobValue[8:10], 1)
	binary.LittleEndian.PutUint16(oobValue[10:12], 0x010f)
	binary.LittleEndian.PutUint16(oobValue[12:14], 2) // ASCII
	binary.LittleEndian.PutUint32(oobValue[14:18], 100)
	binary.LittleEndian.PutUint32(oobValue[18:22], 5)
	if make_, _, _ := parseTIFF(oobValue); make_ != "" {
		t.Fatalf("out-of-range value offset was not skipped: %q", make_)
	}
}
