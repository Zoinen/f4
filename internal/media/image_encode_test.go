package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestClipboardImagePNGPreservesPixelsAndAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(3, 4, 5, 6))
	img.SetNRGBA(3, 4, color.NRGBA{R: 90, G: 120, B: 33, A: 71})
	img.SetNRGBA(4, 5, color.NRGBA{R: 255, A: 255})
	for _, compression := range []string{"none", "speed", "default", "best"} {
		t.Run(compression, func(t *testing.T) {
			var data bytes.Buffer
			if err := EncodeClipboardImage(&data, img, "png", compression, 90); err != nil {
				t.Fatal(err)
			}
			decoded, err := png.Decode(&data)
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 2; y++ {
				for x := 0; x < 2; x++ {
					if got, want := color.NRGBAModel.Convert(decoded.At(x, y)), color.NRGBAModel.Convert(img.At(x+3, y+4)); got != want {
						t.Fatalf("pixel %d,%d = %v, want %v", x, y, got, want)
					}
				}
			}
		})
	}
}

func TestClipboardImageJPEGFlattensOnWhite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	var data bytes.Buffer
	if err := EncodeClipboardImage(&data, img, "jpeg", "default", 100); err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(&data)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := decoded.At(0, 0).RGBA()
	if r < 65000 || g < 65000 || b < 65000 {
		t.Fatalf("transparent pixel became %d,%d,%d", r, g, b)
	}
	for _, quality := range []int{0, 101} {
		if err := EncodeClipboardImage(&data, img, "jpeg", "default", quality); err == nil {
			t.Fatal("invalid quality accepted")
		}
	}
	if err := EncodeClipboardImage(&data, img, "png", "invalid", 90); err == nil {
		t.Fatal("invalid compression accepted")
	}
	if err := EncodeClipboardImage(&data, img, "invalid", "default", 90); err == nil {
		t.Fatal("invalid format accepted")
	}
}
