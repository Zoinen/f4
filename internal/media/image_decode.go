package media

import (
	"bytes"
	"context"
	"fmt"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"github.com/woozymasta/png"
	"image"
	_ "image/jpeg"
	"io"
	"sort"
	"strings"
	"sync"
)

// ImageDecoder describes one way of turning file bytes into pixels. Several
// decoders may claim the same extension; the one with the highest priority is
// tried first and the rest act as fallbacks. This is the seam through which
// platform decoders and subplugins will be added later.
type ImageDecoder struct {
	Name       string
	Priority   int
	Extensions []string
	Decode     func(data []byte) (*vtui.ImageSurface, error)

	// DecodeCtx, when set, is used instead of Decode. A decoder that leaves
	// the process has to be told when nobody is waiting for its answer any
	// more; one that only reads bytes never needs this and keeps the
	// simpler signature.
	DecodeCtx func(ctx context.Context, data []byte) (*vtui.ImageSurface, error)
}

// decode calls whichever of the two functions the decoder provided.
func (d ImageDecoder) decode(ctx context.Context, data []byte) (*vtui.ImageSurface, error) {
	if d.DecodeCtx != nil {
		return d.DecodeCtx(ctx, data)
	}
	return d.Decode(data)
}

// The registry is read from the decoding workers, so it is guarded: a plugin
// registering a decoder while a picture is being decoded is otherwise a race.
var (
	imageDecodersMu sync.RWMutex
	imageDecoders   []ImageDecoder
)

// allImageDecoders returns a snapshot of the registry.
func AllImageDecoders() []ImageDecoder {
	imageDecodersMu.RLock()
	out := append([]ImageDecoder(nil), imageDecoders...)
	imageDecodersMu.RUnlock()

	// The priorities from the settings are applied here rather than stored
	// in the registry, so that clearing the setting brings the built-in
	// order back without a second copy of the built-in numbers.
	for i := range out {
		out[i].Priority = config.ImageDecoderPriorityOf(out[i].Name, out[i].Priority)
	}
	return out
}

// RegisterImageDecoder adds a decoder, replacing an earlier one of the same
// name so that a plugin can override a built-in.
func RegisterImageDecoder(d ImageDecoder) {
	if d.Name == "" || (d.Decode == nil && d.DecodeCtx == nil) {
		return
	}
	imageDecodersMu.Lock()
	defer imageDecodersMu.Unlock()
	for i := range imageDecoders {
		if imageDecoders[i].Name == d.Name {
			imageDecoders[i] = d
			return
		}
	}
	imageDecoders = append(imageDecoders, d)
}

// UnregisterImageDecoder takes a decoder out of the registry. It exists for
// the external converter, which registers itself only when there is a
// converter on the PATH and so has to be able to change its mind.
func UnregisterImageDecoder(name string) {
	imageDecodersMu.Lock()
	defer imageDecodersMu.Unlock()
	for i := range imageDecoders {
		if imageDecoders[i].Name == name {
			imageDecoders = append(imageDecoders[:i], imageDecoders[i+1:]...)
			return
		}
	}
}

func ImageExtension(Path string) string {
	base := Path
	if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
		base = base[i+1:]
	}
	dot := strings.LastIndex(base, ".")
	if dot < 0 || dot == len(base)-1 {
		return ""
	}
	return strings.ToLower(base[dot+1:])
}

// ImageDecodersFor returns the decoders claiming this file, best first.
func ImageDecodersFor(Path string) []ImageDecoder {
	ext := ImageExtension(Path)
	if ext == "" {
		return nil
	}
	var out []ImageDecoder
	for _, d := range AllImageDecoders() {
		for _, e := range d.Extensions {
			if strings.ToLower(e) == ext {
				out = append(out, d)
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority > out[j].Priority
	})
	return out
}

// IsImageFile reports whether anything at all can decode this file.
func IsImageFile(Path string) bool {
	return len(ImageDecodersFor(Path)) > 0
}

// DecodeImage walks the decoders in priority order and returns the first
// result, together with the name of the decoder that produced it. The ones
// claiming the extension go first; when they all fail the remaining ones are
// offered the file too, because a picture saved under the wrong picture name
// is still a picture. A name no decoder claims at all is refused outright:
// the extension is what says the file is meant to be an image, and sniffing
// every file that is opened would promise something quite different.
func DecodeImage(Path string, data []byte) (*vtui.ImageSurface, string, error) {
	return DecodeImageContext(context.Background(), Path, data)
}

// DecodeImageContext is DecodeImage with a way of saying that the answer is
// no longer wanted. Only a decoder that leaves the process can act on it, but
// that is exactly the decoder that can take seconds.
func DecodeImageContext(ctx context.Context, Path string, data []byte) (*vtui.ImageSurface, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	decoders := ImageDecodersFor(Path)
	if len(decoders) == 0 {
		return nil, "", fmt.Errorf("no image decoder for %q", Path)
	}

	claimed := make(map[string]bool, len(decoders))
	for _, d := range decoders {
		claimed[d.Name] = true
	}
	rest := make([]ImageDecoder, 0, len(claimed))
	for _, d := range AllImageDecoders() {
		if !claimed[d.Name] {
			rest = append(rest, d)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool {
		return rest[i].Priority > rest[j].Priority
	})
	decoders = append(decoders, rest...)

	var lastErr error
	for _, d := range decoders {
		surf, err := d.decode(ctx, data)
		if err == nil && surf.Valid() {
			return surf, d.Name, nil
		}
		if err == nil {
			err = fmt.Errorf("decoder %s produced an empty image", d.Name)
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no image decoder for %q", Path)
	}
	return nil, "", lastErr
}

// maxImageFileSize guards against loading a multi gigabyte file into memory.
// Tiled decoding of huge images is a separate job.
const maxImageFileSize = 128 << 20

// imageMaxPixels bounds the geometry a decoder will honour, whatever the
// file claims about itself.
const imageMaxPixels = 64 << 20

// LoadImage reads a file through the VFS and decodes it.
func LoadImage(ctx context.Context, v vfs.VFS, Path string) (*vtui.ImageSurface, string, error) {
	if v == nil {
		return nil, "", fmt.Errorf("no filesystem provider")
	}
	f, err := v.Open(ctx, Path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	size := f.Size()
	if size <= 0 {
		return nil, "", fmt.Errorf("file is empty")
	}
	if size > maxImageFileSize {
		return nil, "", fmt.Errorf("image is too large: %d bytes", size)
	}

	data := make([]byte, size)
	n, err := f.ReadAt(ctx, data, 0)
	if n <= 0 {
		if err == nil {
			err = fmt.Errorf("nothing could be read")
		}
		return nil, "", err
	}
	return DecodeImageContext(ctx, Path, data[:n])
}

func decodeStdImageStream(r io.Reader) (*vtui.ImageSurface, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	return surfaceFromDecodedImage(img)
}

// surfaceFromDecodedImage maps 8-bit RGBA/NRGBA pixels straight to the
// surface, skipping the copy+unpremultiply pass; others go via the generic
// converter.
func surfaceFromDecodedImage(img image.Image) (*vtui.ImageSurface, error) {
	var dx, dy, stride int
	var pix []byte
	switch m := img.(type) {
	case *image.NRGBA:
		dx, dy, stride, pix = m.Rect.Dx(), m.Rect.Dy(), m.Stride, m.Pix
	case *image.RGBA:
		dx, dy, stride, pix = m.Rect.Dx(), m.Rect.Dy(), m.Stride, m.Pix
	}
	if dx > 0 && dy > 0 {
		if surf := vtui.NewImageSurfaceFromPix(dx, dy, stride, pix); surf != nil && surf.Valid() {
			return surf, nil
		}
	}
	surf := vtui.NewImageSurfaceFromImage(img)
	if surf == nil {
		return nil, fmt.Errorf("unsupported image geometry")
	}
	return surf, nil
}

// DecodeImageWithStdlib routes PNG to a faster decoder; the magic bytes,
// not the extension, decide.
func DecodeImageWithStdlib(data []byte) (*vtui.ImageSurface, error) {
	if isPNG(data) {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return surfaceFromDecodedImage(img)
	}
	return decodeStdImageStream(bytes.NewReader(data))
}

var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

func isPNG(data []byte) bool {
	return len(data) >= len(pngSignature) && bytes.Equal(data[:len(pngSignature)], pngSignature)
}

func init() {
	RegisterImageDecoder(ImageDecoder{
		Name:       "go-std",
		Priority:   0,
		Extensions: []string{"png", "jpg", "jpeg", "jfif", "gif"},
		Decode:     DecodeImageWithStdlib,
	})
}
