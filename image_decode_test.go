package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/unxed/vtui"
)

func TestLoadImageRejectsNilVFS(t *testing.T) {
	if _, _, err := LoadImage(context.Background(), nil, "test.png"); err == nil {
		t.Fatal("nil VFS must return an error")
	}
}

func makeTestPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("cannot build the test image: %v", err)
	}
	return buf.Bytes()
}

func TestImageExtensionDetection(t *testing.T) {
	cases := map[string]bool{
		"photo.png":          true,
		"PHOTO.PNG":          true,
		"a/b/c.jpeg":         true,
		"archive.tar.gz":     false,
		"notes.txt":          false,
		"noextension":        false,
		"trailingdot.":       false,
		"dir.png/inner.txt":  false,
		"C:\\pics\\shot.JPG": true,
	}
	for path, want := range cases {
		if got := IsImageFile(path); got != want {
			t.Errorf("%q: got %v, want %v", path, got, want)
		}
	}
}

func TestDecodeImagePNG(t *testing.T) {
	data := makeTestPNG(t, 5, 3, color.RGBA{R: 10, G: 20, B: 30, A: 255})

	surf, name, err := DecodeImage("shot.png", data)
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	if name != "go-std" {
		t.Errorf("unexpected decoder %q", name)
	}
	if surf.Width != 5 || surf.Height != 3 {
		t.Fatalf("wrong geometry %dx%d", surf.Width, surf.Height)
	}
	r, g, b, a := surf.PixelAt(2, 1)
	if r != 10 || g != 20 || b != 30 || a != 255 {
		t.Errorf("wrong pixel %d,%d,%d,%d", r, g, b, a)
	}
}

func TestDecodeImageRejectsGarbage(t *testing.T) {
	if _, _, err := DecodeImage("shot.png", []byte("not a picture")); err == nil {
		t.Error("garbage must not decode")
	}
	if _, _, err := DecodeImage("notes.txt", makeTestPNG(t, 2, 2, color.RGBA{A: 255})); err == nil {
		t.Error("an unclaimed extension must be refused")
	}
}

func TestImageDecoderPriorityAndOverride(t *testing.T) {
	saved := imageDecoders
	defer func() { imageDecoders = saved }()

	called := ""
	RegisterImageDecoder(ImageDecoder{
		Name:       "test-high",
		Priority:   100,
		Extensions: []string{"png"},
		Decode: func(data []byte) (*vtui.ImageSurface, error) {
			called = "test-high"
			return vtui.NewImageSurface(1, 1), nil
		},
	})

	list := ImageDecodersFor("a.png")
	if len(list) != 2 || list[0].Name != "test-high" {
		t.Fatalf("priority order is wrong: %v", list)
	}

	if _, name, err := DecodeImage("a.png", nil); err != nil || name != "test-high" {
		t.Fatalf("the highest priority decoder must win, got %q %v", name, err)
	}
	if called != "test-high" {
		t.Error("the decoder was not actually invoked")
	}

	// Registering the same name again replaces it rather than duplicating.
	RegisterImageDecoder(ImageDecoder{
		Name:       "test-high",
		Priority:   100,
		Extensions: []string{"png"},
		Decode:     func(data []byte) (*vtui.ImageSurface, error) { return nil, nil },
	})
	if len(ImageDecodersFor("a.png")) != 2 {
		t.Error("re-registering a name must replace the old entry")
	}
}

func TestParseImageDecoderPriorities(t *testing.T) {
	got := ParseImageDecoderPriorities("go-std:5 | external:-10 ; nonsense ; bad:x")
	if len(got) != 2 || got["go-std"] != 5 || got["external"] != -10 {
		t.Fatalf("parsed %v", got)
	}
	if ParseImageDecoderPriorities("") != nil {
		t.Error("an empty setting must produce no overrides at all")
	}
}

func TestImageDecoderPrioritiesFromConfiguration(t *testing.T) {
	saved := imageDecoders
	t.Cleanup(func() {
		imageDecoders = saved
		SetImageDecoderPriorities(nil)
	})

	RegisterImageDecoder(ImageDecoder{
		Name:       "test-low",
		Priority:   -50,
		Extensions: []string{"png"},
		Decode:     func([]byte) (*vtui.ImageSurface, error) { return nil, nil },
	})

	if list := ImageDecodersFor("a.png"); list[0].Name != "go-std" {
		t.Fatalf("without an override go-std wins, got %q", list[0].Name)
	}
	SetImageDecoderPriorities(map[string]int{"test-low": 99})
	if list := ImageDecodersFor("a.png"); list[0].Name != "test-low" {
		t.Fatalf("the override must reorder the decoders, got %q", list[0].Name)
	}
	SetImageDecoderPriorities(nil)
	if list := ImageDecodersFor("a.png"); list[0].Name != "go-std" {
		t.Fatalf("clearing the overrides must restore the order, got %q", list[0].Name)
	}
}

func TestDecodeImageFallsBackToTheNextDecoder(t *testing.T) {
	saved := imageDecoders
	defer func() { imageDecoders = saved }()

	RegisterImageDecoder(ImageDecoder{
		Name:       "test-broken",
		Priority:   100,
		Extensions: []string{"png"},
		Decode:     func(data []byte) (*vtui.ImageSurface, error) { return nil, nil },
	})

	data := makeTestPNG(t, 2, 2, color.RGBA{R: 1, A: 255})
	surf, name, err := DecodeImage("a.png", data)
	if err != nil {
		t.Fatalf("the fallback decoder should have succeeded: %v", err)
	}
	if name != "go-std" || surf.Width != 2 {
		t.Errorf("got %q %v", name, surf)
	}
}
