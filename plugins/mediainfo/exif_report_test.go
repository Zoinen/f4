package mediainfo

import (
	"context"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

func TestTIFFUsefulEXIFIsParsedAndRendered(t *testing.T) {
	b := make([]byte, 560)
	copy(b, "II*\x00")
	binary.LittleEndian.PutUint32(b[4:8], 8)
	putIFD := func(off int, entries [][4]uint16) {
		var entryCount uint16
		for range entries {
			entryCount++
		}
		binary.LittleEndian.PutUint16(b[off:off+2], entryCount)
		for i, entry := range entries {
			pos := off + 2 + i*12
			binary.LittleEndian.PutUint16(b[pos:pos+2], entry[0])
			binary.LittleEndian.PutUint16(b[pos+2:pos+4], entry[1])
			binary.LittleEndian.PutUint32(b[pos+4:pos+8], uint32(entry[2]))
			binary.LittleEndian.PutUint32(b[pos+8:pos+12], uint32(entry[3]))
		}
	}
	putIFD(8, [][4]uint16{
		{0x010f, 2, 6, 320}, {0x0110, 2, 8, 340}, {0x0112, 3, 1, 6},
		{0x8769, 4, 1, 100}, {0x8825, 4, 1, 200},
	})
	putIFD(100, [][4]uint16{
		{0x829a, 5, 1, 420}, {0x829d, 5, 1, 428}, {0x8827, 3, 1, 400},
		{0x9003, 2, 20, 360}, {0x920a, 5, 1, 436}, {0xa434, 2, 8, 390},
	})
	putIFD(200, [][4]uint16{
		{0x0001, 2, 2, 'N'}, {0x0002, 5, 3, 444},
		{0x0003, 2, 2, 'W'}, {0x0004, 5, 3, 468},
		{0x0005, 1, 1, 0}, {0x0006, 5, 1, 492},
	})
	copy(b[320:], "Canon\x00")
	copy(b[340:], "EOS R5\x00")
	copy(b[360:], "2026:08:10 12:34:56\x00")
	copy(b[390:], "RF50mm\x00")
	// Inline SHORT value for ISO 400.
	binary.LittleEndian.PutUint16(b[100+2+2*12+8:100+2+2*12+10], 400)
	putRational := func(off int, numerator, denominator uint32) {
		binary.LittleEndian.PutUint32(b[off:off+4], numerator)
		binary.LittleEndian.PutUint32(b[off+4:off+8], denominator)
	}
	putRational(420, 1, 250)
	putRational(428, 28, 10)
	putRational(436, 50, 1)
	for i, pair := range [][2]uint32{{55, 1}, {45, 1}, {30, 1}} {
		putRational(444+i*8, pair[0], pair[1])
	}
	for i, pair := range [][2]uint32{{37, 1}, {36, 1}, {0, 1}} {
		putRational(468+i*8, pair[0], pair[1])
	}
	putRational(492, 1234, 10)

	opts := DefaultOptions(ModeDetailed)
	p, err := newProbe(context.Background(), Source{Name: "photo.tiff", Size: int64(len(b)), Reader: memorySource(b)}, opts)
	if err != nil {
		t.Fatal(err)
	}
	im := &Image{Width: 8192, Height: 5464, BitDepth: 14, ColorModel: "RGB", Compression: "JPEG"}
	parseTIFFMeta(p, b, im)
	if im.CameraMake != "Canon" || im.CameraModel != "EOS R5" || im.LensModel != "RF50mm" || im.Orientation != 6 {
		t.Fatalf("basic EXIF = %#v", im)
	}
	if im.TakenAt == nil || im.Latitude == nil || im.Longitude == nil || im.GPSAltitude == nil {
		t.Fatalf("date/GPS was not decoded: %#v", im)
	}
	if math.Abs(*im.Latitude-55.758333) > 0.00001 || math.Abs(*im.Longitude+37.6) > 0.00001 || math.Abs(*im.GPSAltitude-123.4) > 0.001 {
		t.Fatalf("GPS = lat %v lon %v alt %v", *im.Latitude, *im.Longitude, *im.GPSAltitude)
	}
	values := map[string]string{}
	for _, field := range im.EXIF {
		values[field.Name] = field.Value
	}
	for name, want := range map[string]string{"Exposure time": "1/250 s", "F-number": "f/2.8", "ISO speed": "400", "Focal length": "50 mm"} {
		if values[name] != want {
			t.Errorf("%s = %q, want %q", name, values[name], want)
		}
	}

	report := Report{General: General{FileName: "photo.tiff", Format: "TIFF"}, Streams: []Stream{{Kind: StreamImage, Format: "TIFF", Image: im}}}
	detailed := RenderText(report, RenderOptions{})
	for _, value := range []string{"Lens model", "Captured at", "Rotated 90° clockwise", "GPS latitude", "GPS altitude", "Exposure time", "F-number", "ISO speed", "Focal length"} {
		if !strings.Contains(detailed, value) {
			t.Errorf("detailed report does not contain %q:\n%s", value, detailed)
		}
	}
	compact := RenderText(report, RenderOptions{Compact: true})
	if strings.Contains(compact, "Exposure time") || strings.Contains(compact, "GPS latitude") {
		t.Fatalf("compact report unexpectedly contains detailed EXIF:\n%s", compact)
	}
}
