package media

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
)

// EncodeClipboardImage writes a still image without changing its dimensions.
func EncodeClipboardImage(w io.Writer, img image.Image, format, compression string, quality int) error {
	if img == nil || img.Bounds().Empty() {
		return fmt.Errorf("clipboard image is empty")
	}
	switch format {
	case "png":
		levels := map[string]png.CompressionLevel{"none": png.NoCompression, "speed": png.BestSpeed, "default": png.DefaultCompression, "best": png.BestCompression}
		level, ok := levels[compression]
		if !ok {
			return fmt.Errorf("unknown PNG compression %q", compression)
		}
		encoder := png.Encoder{CompressionLevel: level}
		return encoder.Encode(w, img)
	case "jpeg":
		if quality < 1 || quality > 100 {
			return fmt.Errorf("JPEG quality must be between 1 and 100")
		}
		flat := image.NewRGBA(img.Bounds())
		draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.Draw(flat, flat.Bounds(), img, img.Bounds().Min, draw.Over)
		return jpeg.Encode(w, flat, &jpeg.Options{Quality: quality})
	default:
		return fmt.Errorf("unknown image format %q", format)
	}
}
