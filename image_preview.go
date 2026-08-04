package main

// A stand-in for a picture that has not been decoded yet. A camera stores a
// small copy of the photograph inside the file, and reading a few tens of
// kilobytes of the header is enough to put something on the screen while the
// megapixels are still being turned into pixels.

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image/jpeg"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const (
	// imagePreviewHeadSize is how much of a file is read while looking for
	// an embedded copy. Thumbnails live in the header; past it there is
	// nothing to find.
	imagePreviewHeadSize = 256 << 10

	// imagePreviewCacheLimit bounds how many thumbnails are remembered.
	// They are tiny, so their number matters and their size does not.
	imagePreviewCacheLimit = 256

	// imagePreviewDecoder is what the interface calls this stage.
	imagePreviewDecoder = "exif-thumbnail"
)

// imageQuickPreview returns the small copy the file carries inside itself.
func imageQuickPreview(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
	head, err := imageReadHead(ctx, v, path, imagePreviewHeadSize)
	if err != nil {
		return nil, "", err
	}
	thumb, err := exifThumbnail(head)
	if err != nil {
		return nil, "", err
	}
	img, err := jpeg.Decode(bytes.NewReader(thumb))
	if err != nil {
		return nil, "", err
	}
	surf := vtui.NewImageSurfaceFromImage(img)
	if !surf.Valid() {
		return nil, "", fmt.Errorf("the thumbnail has no pixels")
	}
	return surf, imagePreviewDecoder, nil
}

// imageReadHead reads the beginning of a file, and no more than that.
func imageReadHead(ctx context.Context, v vfs.VFS, path string, limit int) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("no file system")
	}
	f, err := v.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if size := f.Size(); size > 0 && size < int64(limit) {
		limit = int(size)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("file is empty")
	}
	buf := make([]byte, limit)
	n, err := f.ReadAt(ctx, buf, 0)
	if n <= 0 {
		if err == nil {
			err = fmt.Errorf("nothing could be read")
		}
		return nil, err
	}
	return buf[:n], nil
}

// exifThumbnail finds the small copy a camera stored in a JPEG file.
func exifThumbnail(data []byte) ([]byte, error) {
	exif, err := jpegExifSegment(data)
	if err != nil {
		return nil, err
	}
	return tiffThumbnail(exif)
}

// jpegExifSegment returns the contents of the Exif segment, without the
// marker that identifies it.
func jpegExifSegment(data []byte) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, fmt.Errorf("not a JPEG file")
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return nil, fmt.Errorf("the segment structure is broken")
		}
		marker := data[i+1]
		if marker == 0xFF {
			// Padding between segments.
			i++
			continue
		}
		if marker >= 0xD0 && marker <= 0xD9 {
			// Markers that carry nothing after them.
			i += 2
			continue
		}
		if marker == 0xDA {
			// The compressed image begins; the metadata is behind us.
			break
		}
		length := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if length < 2 || i+2+length > len(data) {
			return nil, fmt.Errorf("the segment structure is broken")
		}
		payload := data[i+4 : i+2+length]
		if marker == 0xE1 && len(payload) > 6 && string(payload[:6]) == "Exif\x00\x00" {
			return payload[6:], nil
		}
		i += 2 + length
	}
	return nil, fmt.Errorf("the file carries no Exif block")
}

// tiffThumbnail walks the Exif directories to the second one, which is the
// one describing the thumbnail, and returns the bytes it points at.
func tiffThumbnail(tiff []byte) ([]byte, error) {
	if len(tiff) < 8 {
		return nil, fmt.Errorf("the Exif block is too short")
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, fmt.Errorf("unknown byte order")
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return nil, fmt.Errorf("the Exif block has no TIFF header")
	}

	// The first directory describes the photograph, the second one its
	// thumbnail.
	second, err := tiffNextIFD(tiff, order, int(order.Uint32(tiff[4:8])))
	if err != nil {
		return nil, err
	}
	if second <= 0 || second >= len(tiff) {
		return nil, fmt.Errorf("there is no thumbnail directory")
	}

	offset, length := 0, 0
	if err := tiffWalkIFD(tiff, order, second, func(tag uint16, value uint32) {
		switch tag {
		case 0x0201:
			offset = int(value)
		case 0x0202:
			length = int(value)
		}
	}); err != nil {
		return nil, err
	}
	if offset <= 0 || length <= 0 || offset+length > len(tiff) {
		return nil, fmt.Errorf("the thumbnail lies outside the Exif block")
	}

	thumb := tiff[offset : offset+length]
	if len(thumb) < 2 || thumb[0] != 0xFF || thumb[1] != 0xD8 {
		return nil, fmt.Errorf("the thumbnail is not a JPEG image")
	}
	return thumb, nil
}

// tiffWalkIFD visits the entries of one directory, reporting the values that
// fit into the entry itself. That is all a thumbnail is described by.
func tiffWalkIFD(tiff []byte, order binary.ByteOrder, at int, fn func(tag uint16, value uint32)) error {
	count, err := tiffEntryCount(tiff, order, at)
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		entry := tiff[at+2+i*12:]
		fn(order.Uint16(entry[:2]), order.Uint32(entry[8:12]))
	}
	return nil
}

// tiffNextIFD returns where the directory following this one begins.
func tiffNextIFD(tiff []byte, order binary.ByteOrder, at int) (int, error) {
	count, err := tiffEntryCount(tiff, order, at)
	if err != nil {
		return 0, err
	}
	end := at + 2 + count*12
	return int(order.Uint32(tiff[end : end+4])), nil
}

// tiffEntryCount reads how many entries a directory has and makes sure the
// whole of it, including the pointer to the next one, is really there.
func tiffEntryCount(tiff []byte, order binary.ByteOrder, at int) (int, error) {
	if at < 0 || at+2 > len(tiff) {
		return 0, fmt.Errorf("the directory lies outside the Exif block")
	}
	count := int(order.Uint16(tiff[at : at+2]))
	if at+2+count*12+4 > len(tiff) {
		return 0, fmt.Errorf("the directory is truncated")
	}
	return count, nil
}
