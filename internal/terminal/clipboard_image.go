package terminal

import (
	"context"
	"errors"
	"image"
	"os/exec"

	"github.com/unxed/goclip"
	"github.com/unxed/vtui"
)

// ClipboardContents captures the representations offered for one paste.
type ClipboardContents struct {
	Image    image.Image
	Text     string
	Files    []string
	FilesCut bool
}

// imageReaderRejected reports that goclip's image reader ran and exited with
// failure instead of failing to start. On macOS goclip reads the image through
// osascript, which exits 1 whenever the pasteboard holds no image (text, copied
// files, nothing): that is "no image", not a broken clipboard. Verified on a
// macOS runner for all three cases (f4#2007).
func imageReaderRejected(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit)
}

func ReadClipboardContents(ctx context.Context) (ClipboardContents, error) {
	contents, err := goclip.ReadContents(ctx)
	img := contents.Image
	if err != nil && !errors.Is(err, goclip.ErrNoImage) && !errors.Is(err, goclip.ErrUnsupportedFormat) && !errors.Is(err, goclip.ErrUnavailable) && !imageReaderRejected(err) {
		return ClipboardContents{}, err
	}
	if err := ctx.Err(); err != nil {
		return ClipboardContents{}, err
	}
	if img != nil {
		files, cut, _ := readNativeFileClipboard(ctx)
		if len(files) == 0 {
			payload := vtui.ParseURIList(contents.Text)
			files = payload.Paths
		}
		return ClipboardContents{Image: img, Text: contents.Text, Files: files, FilesCut: cut}, nil
	}

	// Keep vtui's far2l, authorization and terminal integrations for text.
	text := vtui.GetClipboard()
	files, cut, _ := readNativeFileClipboard(ctx)
	if len(files) == 0 {
		payload := vtui.ParseURIList(contents.Text)
		if len(payload.Paths) == 0 {
			payload = vtui.ParseURIList(text)
		}
		files = payload.Paths
	}
	return ClipboardContents{Image: img, Text: text, Files: files, FilesCut: cut}, nil
}
