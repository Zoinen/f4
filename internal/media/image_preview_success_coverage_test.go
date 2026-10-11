package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func makeExifPreviewJPEG(t *testing.T) []byte {
	t.Helper()
	var thumb bytes.Buffer
	if err := jpeg.Encode(&thumb, image.NewRGBA(image.Rect(0, 0, 2, 2)), &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	thumbnail := thumb.Bytes()
	tiff := make([]byte, 48+len(thumbnail))
	tiff[0], tiff[1] = 'I', 'I'
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint32(tiff[10:14], 14)
	binary.LittleEndian.PutUint16(tiff[14:16], 2)
	binary.LittleEndian.PutUint16(tiff[16:18], 0x0201)
	binary.LittleEndian.PutUint32(tiff[24:28], 48)
	binary.LittleEndian.PutUint16(tiff[28:30], 0x0202)
	binary.LittleEndian.PutUint32(tiff[36:40], uint32(len(thumbnail))) //nolint:gosec // the encoded test thumbnail is bounded by the test buffer
	copy(tiff[48:], thumbnail)

	segmentLength := 2 + 6 + len(tiff)
	data := []byte{0xff, 0xd8, 0xff, 0xe1, byte(segmentLength >> 8), byte(segmentLength)} //nolint:gosec // JPEG segment length is bounded by the test fixture
	data = append(data, []byte{'E', 'x', 'i', 'f', 0, 0}...)
	return append(data, tiff...)
}

func TestImageQuickPreviewDecodesEmbeddedThumbnail(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "camera.jpg")
	if err := os.WriteFile(path, makeExifPreviewJPEG(t), 0o600); err != nil {
		t.Fatal(err)
	}
	surf, decoder, err := imageQuickPreview(context.Background(), vfs.NewOSVFS(root), path)
	if err != nil {
		t.Fatalf("imageQuickPreview: %v", err)
	}
	if decoder != imagePreviewDecoder || surf == nil || !surf.Valid() {
		t.Fatalf("preview result = surface %v, decoder %q", surf, decoder)
	}
}
