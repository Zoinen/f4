package vtui

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func kittyRender(k *kittyEncoder, list []ImagePlacement) string {
	var sb strings.Builder
	k.Render(&sb, list)
	return sb.String()
}

func TestKittyUploadAndPlace(t *testing.T) {
	surf := NewImageSurface(4, 4)
	surf.SetPixel(0, 0, 1, 2, 3, 255)

	k := newKittyEncoder()
	out := kittyRender(k, []ImagePlacement{{
		Surface: surf, Col: 2, Row: 3, Cols: 6, Rows: 4, ZIndex: 1,
	}})

	if !strings.Contains(out, "\x1b_Ga=t,q=2,f=32,t=d,i=") {
		t.Errorf("missing transmission command: %q", out)
	}
	if !strings.Contains(out, ",s=4,v=4,") {
		t.Errorf("missing pixel geometry: %q", out)
	}
	if !strings.Contains(out, "\x1b[4;3H") {
		t.Errorf("image must be placed at row 4, column 3: %q", out)
	}
	if !strings.Contains(out, ",c=6,r=4,z=1") {
		t.Errorf("missing cell geometry: %q", out)
	}
	if !strings.Contains(out, "C=1") {
		t.Error("placement must not move the cursor")
	}
	if !strings.HasSuffix(out, "\x1b\\") {
		t.Error("every graphics command must be terminated by ST")
	}
}

func TestKittyChunksLargePayload(t *testing.T) {
	surf := NewImageSurface(40, 40)
	k := newKittyEncoder()
	out := kittyRender(k, []ImagePlacement{{Surface: surf, Cols: 10, Rows: 5}})

	if n := strings.Count(out, "\x1b_Ga=t,"); n != 1 {
		t.Errorf("expected exactly one transmission header, got %d", n)
	}
	if n := strings.Count(out, "\x1b_Gm="); n != 2 {
		t.Errorf("expected two continuation chunks, got %d", n)
	}
	if !strings.Contains(out, "m=0;") {
		t.Error("the last chunk must be marked with m=0")
	}
	for _, part := range strings.Split(out, "\x1b_G")[1:] {
		body := part
		if i := strings.Index(body, ";"); i >= 0 {
			body = body[i+1:]
		}
		body = strings.TrimSuffix(body, "\x1b\\")
		if len(body) > kittyChunkSize {
			t.Fatalf("chunk of %d bytes exceeds the protocol limit", len(body))
		}
	}
}

func TestKittyUploadRoundTrip(t *testing.T) {
	const w, h = 100, 30
	surf := NewImageSurface(w, h)
	for i := range surf.Pix {
		surf.Pix[i] = byte(i*31 + i/3)
	}
	// A padded stride exercises the row walk inside upload.
	padded := NewImageSurfaceFromPix(w, h, w*4+8, make([]byte, (w*4+8)*(h-1)+w*4+8))
	if padded == nil {
		t.Fatal("padded surface must wrap")
	}
	for y := 0; y < h; y++ {
		copy(padded.Pix[y*padded.Stride:y*padded.Stride+w*4], surf.Pix[y*surf.Stride:y*surf.Stride+w*4])
	}

	for _, tc := range []struct {
		name string
		s    *ImageSurface
	}{
		{"tight stride", surf},
		{"padded stride", padded},
	} {
		k := newKittyEncoder()
		out := kittyRender(k, []ImagePlacement{{Surface: tc.s, Cols: 2, Rows: 3}})

		var payload strings.Builder
		for _, part := range strings.Split(out, "\x1b_G")[1:] {
			if i := strings.Index(part, "\x1b\\"); i >= 0 {
				part = part[:i]
			}
			if !strings.HasPrefix(part, "a=t,") && !strings.HasPrefix(part, "m=") {
				continue
			}
			if i := strings.IndexByte(part, ';'); i >= 0 {
				part = part[i+1:]
			}
			payload.WriteString(part)
		}
		raw, err := base64.StdEncoding.DecodeString(payload.String())
		if err != nil {
			t.Fatalf("%s: decode: %v", tc.name, err)
		}
		if len(raw) != w*h*4 {
			t.Fatalf("%s: decoded %d bytes, want %d", tc.name, len(raw), w*h*4)
		}
		for y := 0; y < h; y++ {
			if !bytes.Equal(raw[y*w*4:(y+1)*w*4], tc.s.Pix[y*tc.s.Stride:y*tc.s.Stride+w*4]) {
				t.Fatalf("%s: row %d mismatch", tc.name, y)
			}
		}
	}
}

func TestKittyReusesUploadedImage(t *testing.T) {
	surf := NewImageSurface(4, 4)
	k := newKittyEncoder()

	first := kittyRender(k, []ImagePlacement{{Surface: surf, Cols: 2, Rows: 2}})
	if !strings.Contains(first, "a=t,") {
		t.Fatal("first render must upload the image")
	}

	second := kittyRender(k, []ImagePlacement{{Surface: surf, Col: 5, Cols: 2, Rows: 2}})
	if strings.Contains(second, "a=t,") {
		t.Errorf("moving an image must not re-upload it: %q", second)
	}
	if !strings.Contains(second, "a=p,") {
		t.Error("moving an image must emit a new placement")
	}
	if !strings.Contains(second, "\x1b_Ga=d,d=a,") {
		t.Error("the previous placement must be cleared first")
	}
}

func TestKittyNavClearsBeforeNewImage(t *testing.T) {
	a := NewImageSurface(4, 4)
	b := NewImageSurface(4, 4)
	b.SetPixel(0, 0, 255, 0, 0, 255)

	k := newKittyEncoder()
	kittyRender(k, []ImagePlacement{{Surface: a, Cols: 2, Rows: 2}})
	out := kittyRender(k, []ImagePlacement{{Surface: b, Cols: 2, Rows: 2}})

	clearAt := strings.Index(out, "\x1b_Ga=d,d=a,")
	uploadAt := strings.Index(out, "\x1b_Ga=t,")
	if clearAt < 0 || uploadAt < 0 || clearAt > uploadAt {
		t.Errorf("the clear must precede the upload: %q", out)
	}
	if !k.hasPlaced {
		t.Error("a placement must be live after rendering")
	}
}

func TestKittyPanDoesNotReupload(t *testing.T) {
	surf := NewImageSurface(8, 8)
	k := newKittyEncoder()

	kittyRender(k, []ImagePlacement{{Surface: surf, SrcW: 4, SrcH: 4, Cols: 2, Rows: 2}})
	out := kittyRender(k, []ImagePlacement{{Surface: surf, SrcX: 4, SrcY: 4, SrcW: 4, SrcH: 4, Cols: 2, Rows: 2}})
	if strings.Contains(out, "a=t,") {
		t.Errorf("panning must reuse the uploaded surface, got %q", out)
	}
	if !strings.Contains(out, "\x1b_Ga=p,") || !strings.Contains(out, ",x=4,y=4,w=4,h=4") {
		t.Errorf("panning must re-place with a source rectangle, got %q", out)
	}
}

func TestKittySkipsInvalidPlacements(t *testing.T) {
	k := newKittyEncoder()
	out := kittyRender(k, []ImagePlacement{
		{Surface: nil, Cols: 2, Rows: 2},
		{Surface: NewImageSurface(4, 4), Cols: 0, Rows: 2},
		{Surface: NewImageSurface(0, 0), Cols: 2, Rows: 2},
	})
	if out != "" {
		t.Errorf("nothing drawable should produce no output, got %q", out)
	}
}

func TestKittyResetDropsEverything(t *testing.T) {
	surf := NewImageSurface(4, 4)
	k := newKittyEncoder()
	kittyRender(k, []ImagePlacement{{Surface: surf, Cols: 2, Rows: 2}})

	var sb strings.Builder
	k.Reset(&sb)
	if !strings.Contains(sb.String(), "\x1b_Ga=d,d=A,") {
		t.Errorf("reset must ask the terminal to drop every image, got %q", sb.String())
	}

	sb.Reset()
	k.Reset(&sb)
	if sb.String() != "" {
		t.Error("resetting an already empty encoder must be silent")
	}

	out := kittyRender(k, []ImagePlacement{{Surface: surf, Cols: 2, Rows: 2}})
	if !strings.Contains(out, "a=t,") {
		t.Error("after a reset the image has to be uploaded again")
	}
}

func TestKittyEvictsOldImages(t *testing.T) {
	k := newKittyEncoder()
	var sb strings.Builder
	for i := 0; i < kittyCacheLimit+3; i++ {
		surf := NewImageSurface(2, 2)
		surf.SetPixel(0, 0, byte(i), byte(i>>8), 0, 255)
		sb.Reset()
		k.Render(&sb, []ImagePlacement{{Surface: surf, Cols: 1, Rows: 1}})
	}
	if len(k.uploaded) > kittyCacheLimit {
		t.Errorf("cache grew to %d entries", len(k.uploaded))
	}
	if !strings.Contains(sb.String(), "\x1b_Ga=d,d=I,") {
		t.Error("evicted images must be freed on the terminal side")
	}
}

func newGraphicsTestScreen(t *testing.T) (*ScreenBuf, *bytes.Buffer) {
	t.Helper()
	scr := NewSilentScreenBuf()
	out := &bytes.Buffer{}
	scr.Writer = out
	scr.AllocBuf(20, 10)
	scr.Renderer = &AnsiRenderer{parent: scr}
	scr.Graphics().SetProtocol(GraphicsKitty)
	return scr, out
}

func TestScreenBufFlushEmitsGraphics(t *testing.T) {
	scr, out := newGraphicsTestScreen(t)
	surf := NewImageSurface(4, 4)
	scr.Graphics().Add(ImagePlacement{Surface: surf, Col: 1, Row: 1, Cols: 4, Rows: 2})

	scr.Flush()
	if !strings.Contains(out.String(), "\x1b_Ga=t,") {
		t.Fatalf("first flush must upload the image: %q", out.String())
	}

	out.Reset()
	scr.Flush()
	if strings.Contains(out.String(), "\x1b_G") {
		t.Errorf("an unchanged frame must not re-send graphics: %q", out.String())
	}

	out.Reset()
	scr.Write(2, 1, StringToCharInfo("hi", 0))
	scr.Flush()
	body := out.String()
	if !strings.Contains(body, "\x1b_Ga=p,") {
		t.Errorf("dirty background must re-place the image: %q", body)
	}
	if strings.Contains(body, "\x1b_Ga=t,") {
		t.Errorf("re-placement must not re-upload pixels: %q", body)
	}
}

func TestScreenBufFlushRespectsProtocolNone(t *testing.T) {
	scr, out := newGraphicsTestScreen(t)
	scr.Graphics().SetProtocol(GraphicsNone)
	scr.Graphics().Add(ImagePlacement{Surface: NewImageSurface(4, 4), Cols: 4, Rows: 2})

	scr.Flush()
	if strings.Contains(out.String(), "\x1b_G") {
		t.Errorf("no graphics protocol means no graphics output: %q", out.String())
	}
	if scr.SupportsGraphics() {
		t.Error("SupportsGraphics must follow the layer protocol")
	}
}

func TestScreenBufHardResetInvalidatesGraphics(t *testing.T) {
	scr, out := newGraphicsTestScreen(t)
	scr.Graphics().Add(ImagePlacement{Surface: NewImageSurface(4, 4), Cols: 4, Rows: 2})
	scr.Flush()

	out.Reset()
	scr.HardReset()
	scr.Flush()
	body := out.String()
	if !strings.Contains(body, "\x1b_Ga=d,d=A,") {
		t.Errorf("hard reset must drop stale terminal state: %q", body)
	}
	if !strings.Contains(body, "\x1b_Ga=t,") {
		t.Errorf("hard reset must re-upload the image: %q", body)
	}
}

// kittyPayloadBytes decodes the base64 payload of the single transmission in
// out (small test images fit in one chunk) so tests can check exact bytes.
func kittyPayloadBytes(t *testing.T, out string) []byte {
	t.Helper()
	body := out
	if i := strings.Index(body, "\x1b_Ga=t,"); i >= 0 {
		body = body[i:]
	}
	if j := strings.IndexByte(body, ';'); j >= 0 {
		body = body[j+1:]
	}
	if j := strings.Index(body, "\x1b\\"); j >= 0 {
		body = body[:j]
	}
	dec, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("bad base64 payload: %v (%q)", err, body)
	}
	return dec
}

func TestKittyOpaqueUploadsRGB(t *testing.T) {
	surf := NewImageSurface(2, 2)
	surf.SetPixel(0, 0, 10, 20, 30, 255)
	surf.SetPixel(1, 0, 40, 50, 60, 255)
	surf.SetPixel(0, 1, 70, 80, 90, 255)
	surf.SetPixel(1, 1, 100, 110, 120, 255)
	surf.Opaque = true

	k := newKittyEncoder()
	out := kittyRender(k, []ImagePlacement{{Surface: surf, Cols: 2, Rows: 2}})

	if !strings.Contains(out, "f=24") {
		t.Fatalf("opaque image must upload as RGB f=24: %q", out)
	}
	if strings.Contains(out, "f=32") {
		t.Fatalf("opaque image must not upload as RGBA f=32: %q", out)
	}
	want := []byte{
		10, 20, 30, 40, 50, 60,
		70, 80, 90, 100, 110, 120,
	}
	if got := kittyPayloadBytes(t, out); !bytes.Equal(got, want) {
		t.Errorf("payload mismatch: got %v want %v", got, want)
	}
}

func TestKittyAlphaUploadsRGBA(t *testing.T) {
	surf := NewImageSurface(2, 2)
	surf.SetPixel(0, 0, 10, 20, 30, 128)

	k := newKittyEncoder()
	out := kittyRender(k, []ImagePlacement{{Surface: surf, Cols: 2, Rows: 2}})

	if !strings.Contains(out, "f=32") {
		t.Fatalf("alpha image must upload as RGBA f=32: %q", out)
	}
	if strings.Contains(out, "f=24") {
		t.Fatalf("alpha image must not upload as RGB f=24: %q", out)
	}
	want := []byte{
		10, 20, 30, 128,
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0,
	}
	if got := kittyPayloadBytes(t, out); !bytes.Equal(got, want) {
		t.Errorf("payload mismatch: got %v want %v", got, want)
	}
}

func TestKittyOpaqueRGBChunksRoundTrip(t *testing.T) {
	const w, h = 64, 48
	surf := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			surf.SetPixel(x, y, byte(x), byte(y), byte(x^y), 255)
		}
	}
	surf.Opaque = true

	k := newKittyEncoder()
	out := kittyRender(k, []ImagePlacement{{Surface: surf, Cols: 10, Rows: 5}})

	// 64x48x3 = 9216 payload bytes across several chunks; the first chunk
	// boundary splits a row, so this exercises the RGB continuation path.
	var payload []byte
	for _, part := range strings.Split(out, "\x1b_G")[1:] {
		if !strings.HasPrefix(part, "a=t,") && !strings.HasPrefix(part, "m=") {
			continue // placement or delete command, no payload
		}
		body := part
		if i := strings.IndexByte(body, ';'); i >= 0 {
			body = body[i+1:]
		}
		if j := strings.Index(body, "\x1b\\"); j >= 0 {
			body = body[:j]
		}
		dec, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			t.Fatalf("bad chunk payload: %v (%q)", err, part)
		}
		payload = append(payload, dec...)
	}

	if len(payload) != w*h*3 {
		t.Fatalf("expected %d payload bytes, got %d", w*h*3, len(payload))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := (y*w + x) * 3
			if payload[o] != byte(x) || payload[o+1] != byte(y) || payload[o+2] != byte(x^y) {
				t.Fatalf("pixel (%d,%d) mangled: got %v", x, y, payload[o:o+3])
			}
		}
	}
}
