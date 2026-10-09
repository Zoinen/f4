package visren

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestVisrenMetadataFileAndConfigEdges(t *testing.T) {
	mtime := time.Date(2024, 1, 2, 3, 4, 5, 0, time.Local)
	if meta, err := readMetadata(filepath.Join(t.TempDir(), "note.txt"), mtime); err != nil || meta != (Metadata{}) {
		t.Fatalf("text metadata = %#v, %v", meta, err)
	}
	if _, err := readMetadata(filepath.Join(t.TempDir(), "missing.mp3"), mtime); err == nil {
		t.Fatal("missing MP3 metadata was accepted")
	}
	if _, err := readMetadata(filepath.Join(t.TempDir(), "missing.JPG"), mtime); err == nil {
		t.Fatal("missing image metadata was accepted")
	}

	dir := t.TempDir()
	oldDir := vfs.CustomConfigDir
	vfs.CustomConfigDir = dir
	t.Cleanup(func() { vfs.CustomConfigDir = oldDir })
	if err := os.WriteFile(configPath(), []byte(`{"word_div":"","editor_format":"invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadConfig(); got.WordDiv != "-. _&" || got.EditorFormat != editorFormatSourceTarget {
		t.Fatalf("invalid config = %#v", got)
	}
	long := strings.Repeat("аб", 12)
	if err := os.WriteFile(configPath(), []byte(`{"word_div":"`+long+`","editor_format":"`+editorFormatTargetsOnly+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadConfig(); len([]rune(got.WordDiv)) != 18 || got.EditorFormat != editorFormatTargetsOnly {
		t.Fatalf("long config = %#v", got)
	}
	if err := os.WriteFile(configPath(), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadConfig(); got.WordDiv != "-. _&" || got.EditorFormat != editorFormatSourceTarget {
		t.Fatalf("malformed config = %#v", got)
	}
}

func putTIFF16(order binary.ByteOrder, dst []byte, value uint16) { order.PutUint16(dst, value) }
func putTIFF32(order binary.ByteOrder, dst []byte, value uint32) { order.PutUint32(dst, value) }

func TestVisrenJPEGExifAndTIFFBounds(t *testing.T) {
	order := binary.BigEndian
	tiff := make([]byte, 100)
	copy(tiff[:2], "MM")
	putTIFF16(order, tiff[2:4], 42)
	putTIFF32(order, tiff[4:8], 8)
	putTIFF16(order, tiff[8:10], 3)
	// Make, model and DateTimeOriginal exercise inline and offset ASCII values.
	entries := []struct {
		tag    uint16
		length uint32
		value  uint32
	}{
		{0x010f, 4, uint32('F')<<24 | uint32('4')<<16},
		{0x0110, 6, 50},
		{0x9003, 20, 56},
	}
	for i, entry := range entries {
		p := 10 + i*12
		putTIFF16(order, tiff[p:p+2], entry.tag)
		putTIFF16(order, tiff[p+2:p+4], 2)
		putTIFF32(order, tiff[p+4:p+8], entry.length)
		putTIFF32(order, tiff[p+8:p+12], entry.value)
	}
	copy(tiff[50:], "Camera\x00")
	copy(tiff[56:], "2025:06:07 08:09:10\x00")
	section := append([]byte("Exif\x00\x00"), tiff...)
	// #nosec G115 -- the synthetic EXIF section is far below the JPEG segment limit.
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe1, byte((len(section) + 2) >> 8), byte(len(section) + 2)}
	jpeg = append(jpeg, section...)
	jpeg = append(jpeg, 0xff, 0xd9)
	make_, model, date := parseJPEGEXIF(jpeg)
	if make_ != "F4" || model != "Camera" || date != "2025.06.07 08-09-10" {
		t.Fatalf("EXIF = %q, %q, %q", make_, model, date)
	}
	for _, data := range [][]byte{nil, {0xff}, {0xff, 0xd8, 0xff, 0xd9}, {0xff, 0xd8, 0x01, 0x02}} {
		if make_, model, date := parseJPEGEXIF(data); make_ != "" || model != "" || date != "" {
			t.Errorf("malformed JPEG metadata = %q, %q, %q", make_, model, date)
		}
	}
	for _, data := range [][]byte{nil, []byte("XX"), {0x49, 0x49, 0, 42, 0, 0, 0, 0}} {
		if make_, model, date := parseTIFF(data); make_ != "" || model != "" || date != "" {
			t.Errorf("malformed TIFF metadata = %q, %q, %q", make_, model, date)
		}
	}
	badOffset := make([]byte, 20)
	copy(badOffset[:2], "II")
	binary.LittleEndian.PutUint16(badOffset[2:4], 42)
	binary.LittleEndian.PutUint32(badOffset[4:8], 8)
	binary.LittleEndian.PutUint16(badOffset[8:10], 1)
	binary.LittleEndian.PutUint16(badOffset[10:12], 0x010f)
	binary.LittleEndian.PutUint16(badOffset[12:14], 2)
	binary.LittleEndian.PutUint32(badOffset[14:18], 100)
	if make_, _, _ := parseTIFF(badOffset); make_ != "" {
		t.Fatalf("out-of-range TIFF value = %q", make_)
	}
}

func TestVisrenMaskRangeAndMetadataEdges(t *testing.T) {
	item := testItem("name.txt")
	item.MTime = time.Date(2024, 5, 6, 7, 8, 9, 0, time.Local)
	for _, tc := range []struct {
		token string
		want  string
	}{
		{"DM", "2024.05.06"}, {"TM", "07-08-09"}, {"R", "123"},
	} {
		got, err := expandToken(tc.token, "name", "txt", item, 0, &maskFlags{})
		if err != nil || got != tc.want {
			t.Errorf("expandToken(%q) = %q, %v", tc.token, got, err)
		}
	}
	for _, mask := range []string{"bad]", "[bad", "[unknown]"} {
		if _, _, err := expandMask(mask, "name", "txt", item, 0); err == nil {
			t.Errorf("expandMask(%q) succeeded", mask)
		}
	}
	for _, tc := range []struct {
		spec   string
		length int
		start  int
		end    int
	}{
		{"2", 5, 1, 1}, {"-1", 5, 4, 4}, {"2,2", 5, 1, 2}, {"2-4", 5, 1, 3}, {"2-", 5, 1, 4},
	} {
		start, end, err := parseRange(tc.spec, tc.length)
		if err != nil || start != tc.start || end != tc.end {
			t.Errorf("parseRange(%q) = %d, %d, %v", tc.spec, start, end, err)
		}
	}
	for _, spec := range []string{"0", "1,0", "1,2,3", "4-2", "x"} {
		if _, _, err := parseRange(spec, 5); err == nil {
			t.Errorf("parseRange(%q) succeeded", spec)
		}
	}
	if got, err := applyRange([]rune("abc"), "99"); err != nil || got != "" {
		t.Fatalf("out-of-range applyRange = %q, %v", got, err)
	}
	value, ranges := applyFlagsToRanges("one two", []TextRange{{-2, 99}, {2, 2}}, maskFlags{upper: true}, " ")
	if value != "ONE TWO" || !reflect.DeepEqual(ranges, []TextRange{{0, 7}}) {
		t.Fatalf("flag ranges = %q, %#v", value, ranges)
	}
	name, ext, separator := splitTextRanges("name.txt", []TextRange{{0, 4}, {4, 8}})
	if !reflect.DeepEqual(name, []TextRange{{0, 4}}) || !reflect.DeepEqual(ext, []TextRange{{0, 3}}) || !separator {
		t.Fatalf("split ranges = %#v, %#v, %v", name, ext, separator)
	}
}

func TestVisrenImageMetadataUsesDecodedDimensionsForJPGExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	meta, err := readImageMetadata(path, time.Unix(0, 0))
	if err != nil || meta.Width != 3 || meta.Height != 2 {
		t.Fatalf("image metadata = %#v, %v", meta, err)
	}
}
