package vtui

import (
	"bytes"
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
	if !strings.Contains(second, "\x1b_Ga=d,d=i,") {
		t.Error("the previous placement must be removed first")
	}
}

func TestKittyDifferentCropUploadsSeparately(t *testing.T) {
	surf := NewImageSurface(8, 8)
	k := newKittyEncoder()

	kittyRender(k, []ImagePlacement{{Surface: surf, SrcW: 4, SrcH: 4, Cols: 2, Rows: 2}})
	out := kittyRender(k, []ImagePlacement{{Surface: surf, SrcX: 4, SrcY: 4, SrcW: 4, SrcH: 4, Cols: 2, Rows: 2}})
	if !strings.Contains(out, "a=t,") {
		t.Error("a different crop of the same surface is a different image")
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
