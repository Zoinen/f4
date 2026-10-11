package terminal

import (
	"context"
	"errors"
)

// ErrImageClipboardUnavailable reports that this session has no system
// clipboard that accepts a picture: a TTY over SSH, a Linux box with neither
// wl-copy nor xclip, an OS f4 has no image driver for.
var ErrImageClipboardUnavailable = errors.New("image clipboard is unavailable")

// SetImageClipboard puts a PNG-encoded picture on the system clipboard, so it
// pastes into an image editor or a messenger as the image itself (#1804).
func SetImageClipboard(ctx context.Context, pngData []byte) error {
	if len(pngData) == 0 {
		return ErrImageClipboardUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return setImageClipboard(ctx, pngData)
}
