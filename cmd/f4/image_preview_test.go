package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// jpegBytes encodes a picture of the given size.
func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

// jpegWithThumbnail builds a JPEG whose Exif block carries the given
// thumbnail, the way a camera writes it.
func jpegWithThumbnail(t *testing.T, outer, thumb []byte) []byte {
	t.Helper()

	var tiff bytes.Buffer
	put16 := func(v uint16) {
		if err := binary.Write(&tiff, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	put32 := func(v uint32) {
		if err := binary.Write(&tiff, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := tiff.WriteString("II"); err != nil {
		t.Fatal(err)
	}
	put16(42)
	put32(8) // the first directory follows the header

	// An empty first directory pointing at the second one.
	put16(0)
	put32(14)

	// The second directory: where the thumbnail is and how long it is.
	const thumbOffset = 44
	put16(2)
	put16(0x0201)
	put16(4)
	put32(1)
	put32(thumbOffset)
	put16(0x0202)
	put16(4)
	put32(1)
	put32(testUint32(len(thumb)))
	put32(0) // no further directories

	if tiff.Len() != thumbOffset {
		t.Fatalf("the test built a %d byte header, expected %d", tiff.Len(), thumbOffset)
	}
	if _, err := tiff.Write(thumb); err != nil {
		t.Fatal(err)
	}

	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)

	out := []byte{0xFF, 0xD8, 0xFF, 0xE1}
	out = binary.BigEndian.AppendUint16(out, testUint16(len(payload)+2))
	out = append(out, payload...)
	return append(out, outer[2:]...)
}

func TestExifThumbnail(t *testing.T) {
	thumb := jpegBytes(t, 8, 6)
	file := jpegWithThumbnail(t, jpegBytes(t, 64, 48), thumb)

	got, err := exifThumbnail(file)
	if err != nil {
		t.Fatalf("the thumbnail was not found: %v", err)
	}
	if !bytes.Equal(got, thumb) {
		t.Errorf("the thumbnail came out wrong: %d bytes instead of %d", len(got), len(thumb))
	}

	img, err := jpeg.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("the thumbnail does not decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 8 || b.Dy() != 6 {
		t.Errorf("thumbnail geometry: %dx%d", b.Dx(), b.Dy())
	}
}

func TestExifThumbnailRejectsWhatItCannotRead(t *testing.T) {
	if _, err := exifThumbnail(jpegBytes(t, 16, 16)); err == nil {
		t.Error("a file without an Exif block must be reported as such")
	}
	if _, err := exifThumbnail([]byte("not a picture at all")); err == nil {
		t.Error("a file that is not a JPEG must be reported as such")
	}
	if _, err := exifThumbnail(nil); err == nil {
		t.Error("an empty file must be reported as such")
	}

	// A directory promising more entries than the block can hold.
	truncated := []byte("II")
	truncated = binary.LittleEndian.AppendUint16(truncated, 42)
	truncated = binary.LittleEndian.AppendUint32(truncated, 8)
	truncated = binary.LittleEndian.AppendUint16(truncated, 500)
	if _, err := tiffThumbnail(truncated); err == nil {
		t.Error("a truncated directory must be reported as such")
	}
}

func TestImagePipelinePreview(t *testing.T) {
	previews := 0
	p := newTestPipeline(func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		return imageTestSurface(64, 64), "stub", nil
	})
	p.preview = func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		previews++
		return imageTestSurface(8, 8), imagePreviewDecoder, nil
	}

	res, ok := p.PreviewSync(context.Background(), nil, "a.jpg")
	if !ok || !res.Preview {
		t.Fatalf("expected a preview, got %+v (ok=%v)", res, ok)
	}
	if res.Surface.Width != 8 {
		t.Errorf("the preview is the small copy, got %d wide", res.Surface.Width)
	}

	// The thumbnail is remembered, so reopening does not read the file
	// again.
	if _, ok := p.PreviewSync(context.Background(), nil, "a.jpg"); !ok {
		t.Fatal("the second request must be answered too")
	}
	if previews != 1 {
		t.Errorf("the thumbnail must be extracted once, got %d times", previews)
	}

	// Once the whole picture is decoded, that is what a request gets.
	if res := p.LoadSync(context.Background(), nil, "a.jpg"); res.Err != nil {
		t.Fatalf("decoding failed: %v", res.Err)
	}
	res, ok = p.PreviewSync(context.Background(), nil, "a.jpg")
	if !ok || res.Preview || res.Surface.Width != 64 {
		t.Errorf("the finished picture must win over its thumbnail: %+v", res)
	}
}

func TestImagePipelinePreviewAbsent(t *testing.T) {
	p := newTestPipeline(func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		return imageTestSurface(64, 64), "stub", nil
	})
	p.preview = func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		return nil, "", errors.New("no thumbnail here")
	}

	if res, ok := p.PreviewSync(context.Background(), nil, "a.png"); ok {
		t.Errorf("a file without a thumbnail has no preview: %+v", res)
	}
}

func TestImageQuickPreviewSkipsNonJPEGWithoutOpeningIt(t *testing.T) {
	// A nil VFS makes any attempted Open fail with "no file system". PNG is
	// rejected by its format before that point, avoiding remote I/O entirely.
	_, _, err := imageQuickPreview(context.Background(), nil, "a.png")
	if err == nil || err.Error() != "embedded preview is only supported for JPEG files" {
		t.Fatalf("PNG preview error = %v", err)
	}
}
