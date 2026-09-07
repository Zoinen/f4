package visren

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadID3v11(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tag.mp3")
	data := make([]byte, 256)
	tag := data[len(data)-128:]
	copy(tag[:3], "TAG")
	copy(tag[3:33], "Song")
	copy(tag[33:63], "Artist")
	copy(tag[63:93], "Album")
	copy(tag[93:97], "2024")
	tag[125], tag[126], tag[127] = 0, 7, 17
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := readID3(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "Song" || meta.Artist != "Artist" || meta.Track != "7" || meta.Genre != "Rock" {
		t.Fatalf("metadata=%+v", meta)
	}
}

func TestAllLegacyGenresAvailable(t *testing.T) {
	if len(id3Genres) != 148 || decodeGenre("147") != "Synthpop" {
		t.Fatalf("genre table has %d entries, last=%q", len(id3Genres), decodeGenre("147"))
	}
}

func TestReadID3v2OverridesV1(t *testing.T) {
	frame := func(id, value string) []byte {
		payload := append([]byte{3}, []byte(value)...)
		buf := make([]byte, 10+len(payload))
		copy(buf[:4], id)
		binary.BigEndian.PutUint32(buf[4:8], id3FixtureUint32(len(payload)))
		copy(buf[10:], payload)
		return buf
	}
	body := append(frame("TIT2", "V2 title"), frame("TRCK", "03/12")...)
	header := []byte{'I', 'D', '3', 3, 0, 0, id3FixtureByte(len(body) >> 21), id3FixtureByte(len(body) >> 14), id3FixtureByte(len(body) >> 7), id3FixtureByte(len(body))}
	data := append(header, body...)
	padding := make([]byte, 128)
	copy(padding[:3], "TAG")
	copy(padding[3:33], "V1 title")
	copy(padding[33:63], "V1 artist")
	data = append(data, padding...)
	path := filepath.Join(t.TempDir(), "tag.mp3")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := readID3(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "V2 title" || meta.Track != "03" || meta.Artist != "" {
		t.Fatalf("metadata=%+v", meta)
	}
}

func id3FixtureUint32(value int) uint32 {
	return uint32(value) // #nosec G115 -- ID3 test frames are bounded well below the format's uint32 size field.
}

func id3FixtureByte(value int) byte {
	return byte(value) // #nosec G115 -- callers pass seven-bit chunks from the small ID3 test body.
}

func TestReadImageDimensionsAndDate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	img := image.NewRGBA(image.Rect(0, 0, 17, 9))
	img.Set(0, 0, color.White)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2024, 5, 6, 7, 8, 9, 0, time.Local)
	meta, err := readImageMetadata(path, mtime)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Width != 17 || meta.Height != 9 || meta.ImageDate != "2024.05.06 07-08-09" {
		t.Fatalf("metadata=%+v", meta)
	}
}

func TestParseTIFFAndFixedVersion(t *testing.T) {
	// Little-endian TIFF with one inline ASCII Make tag.
	tiff := make([]byte, 26)
	copy(tiff[:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x010f)
	binary.LittleEndian.PutUint16(tiff[12:14], 2)
	binary.LittleEndian.PutUint32(tiff[14:18], 4)
	copy(tiff[18:22], "F4\x00")
	make_, _, _ := parseTIFF(tiff)
	if make_ != "F4" {
		t.Fatalf("make=%q", make_)
	}

	version := make([]byte, 24)
	copy(version[4:8], []byte{0xbd, 0x04, 0xef, 0xfe})
	binary.LittleEndian.PutUint32(version[12:16], uint32(1)<<16|2)
	binary.LittleEndian.PutUint32(version[16:20], uint32(3)<<16|4)
	if got := fixedFileVersion(version); got != "1.2.3.4" {
		t.Fatalf("version=%q", got)
	}
}

func TestParseTIFFDateTimeOriginal(t *testing.T) {
	// IFD0 points at an Exif IFD containing DateTimeOriginal.
	tiff := make([]byte, 80)
	copy(tiff[:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x8769)
	binary.LittleEndian.PutUint16(tiff[12:14], 4)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint32(tiff[18:22], 26)
	binary.LittleEndian.PutUint16(tiff[26:28], 1)
	binary.LittleEndian.PutUint16(tiff[28:30], 0x9003)
	binary.LittleEndian.PutUint16(tiff[30:32], 2)
	binary.LittleEndian.PutUint32(tiff[32:36], 20)
	binary.LittleEndian.PutUint32(tiff[36:40], 48)
	copy(tiff[48:], "2025:06:07 08:09:10\x00")
	_, _, date := parseTIFF(tiff)
	if date != "2025.06.07 08-09-10" {
		t.Fatalf("date=%q", date)
	}
}
