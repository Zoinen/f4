//go:build darwin && cgo

package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestAppleImageIODecoderProducesTopDownRGBA(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	source.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	source.SetRGBA(1, 0, color.RGBA{G: 255, A: 255})
	source.SetRGBA(0, 1, color.RGBA{B: 255, A: 255})
	source.SetRGBA(1, 1, color.RGBA{R: 255, G: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}

	surface, err := decodeImageWithAppleImageIO(context.Background(), encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if surface.Width != 2 || surface.Height != 2 {
		t.Fatalf("geometry = %dx%d, want 2x2", surface.Width, surface.Height)
	}
	want := []byte{
		255, 0, 0, 255, 0, 255, 0, 255,
		0, 0, 255, 255, 255, 255, 0, 255,
	}
	if !bytes.Equal(surface.Pix, want) {
		t.Fatalf("pixels = %v, want top-down RGBA %v", surface.Pix, want)
	}
}

func TestAppleImageIODecoderPrecedesExternalHEICDecoder(t *testing.T) {
	decoders := ImageDecodersFor("photo.HEIC")
	if len(decoders) == 0 || decoders[0].Name != appleImageDecoder {
		t.Fatalf("HEIC decoder order = %#v, want %q first", decoders, appleImageDecoder)
	}
}
