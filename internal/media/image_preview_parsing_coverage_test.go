package media

import (
	"encoding/binary"
	"testing"
)

func TestJPEGExifSegmentRejectsBrokenMetadata(t *testing.T) {
	cases := [][]byte{
		{},
		{0xff, 0xd8, 0xff},
		{0xff, 0xd8, 0x00, 0x00},
		{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x01},
		{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x06, 'x'},
		{0xff, 0xd8, 0xff, 0xda, 0x00, 0x02},
	}
	for _, data := range cases {
		if _, err := jpegExifSegment(data); err == nil {
			t.Fatalf("jpegExifSegment(%x) accepted malformed metadata", data)
		}
	}

	data := []byte{0xff, 0xd8, 0xff, 0xff, 0xff, 0xe0, 0x00, 0x02}
	data = append(data, 0xff, 0xe1, 0x00, 0x0a, 'E', 'x', 'i', 'f', 0, 0, 'I', 'I')
	got, err := jpegExifSegment(data)
	if err != nil || string(got) != "II" {
		t.Fatalf("jpegExifSegment padding/Exif = %q, %v", got, err)
	}
}

func makeThumbnailTIFF() []byte {
	tiff := make([]byte, 60)
	tiff[0], tiff[1] = 'I', 'I'
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	// The first directory is empty and points to the thumbnail directory.
	binary.LittleEndian.PutUint32(tiff[10:14], 14)
	binary.LittleEndian.PutUint16(tiff[14:16], 2)
	binary.LittleEndian.PutUint16(tiff[16:18], 0x0201)
	binary.LittleEndian.PutUint32(tiff[24:28], 48)
	binary.LittleEndian.PutUint16(tiff[28:30], 0x0202)
	binary.LittleEndian.PutUint32(tiff[36:40], 4)
	tiff[48], tiff[49], tiff[50], tiff[51] = 0xff, 0xd8, 0x01, 0x02
	return tiff
}

func TestTIFFThumbnailWalksDirectoriesAndRejectsBadBlocks(t *testing.T) {
	if _, err := tiffThumbnail(nil); err == nil {
		t.Fatal("short TIFF was accepted")
	}
	if _, err := tiffThumbnail([]byte("XX!!!!!!")); err == nil {
		t.Fatal("unknown byte order was accepted")
	}
	wrongMagic := make([]byte, 8)
	wrongMagic[0], wrongMagic[1] = 'I', 'I'
	if _, err := tiffThumbnail(wrongMagic); err == nil {
		t.Fatal("wrong TIFF magic was accepted")
	}

	valid := makeThumbnailTIFF()
	thumb, err := tiffThumbnail(valid)
	if err != nil || len(thumb) != 4 || thumb[0] != 0xff || thumb[1] != 0xd8 {
		t.Fatalf("valid thumbnail = %x, %v", thumb, err)
	}

	for _, bad := range [][]byte{
		append([]byte(nil), valid[:14]...),
		func() []byte { b := append([]byte(nil), valid...); b[10] = 0xff; return b }(),
		func() []byte { b := append([]byte(nil), valid...); b[48] = 0; return b }(),
	} {
		if _, err := tiffThumbnail(bad); err == nil {
			t.Fatalf("malformed TIFF %x was accepted", bad)
		}
	}
}

func TestTIFFEntryHelpersValidateBounds(t *testing.T) {
	if _, err := tiffEntryCount([]byte{0}, binary.LittleEndian, 0); err == nil {
		t.Fatal("truncated directory count was accepted")
	}
	if _, err := tiffEntryCount([]byte{1, 0, 0, 0}, binary.LittleEndian, 0); err == nil {
		t.Fatal("truncated directory entry was accepted")
	}
	if _, err := tiffEntryCount(nil, binary.LittleEndian, -1); err == nil {
		t.Fatal("negative directory offset was accepted")
	}
	if _, err := tiffNextIFD([]byte{0, 0, 0, 0}, binary.LittleEndian, 0); err == nil {
		t.Fatal("truncated next-directory pointer was accepted")
	}
}
