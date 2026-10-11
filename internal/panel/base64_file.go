package panel

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var (
	errBase64FileNoSelection = errors.New("select a file first")
	errBase64FileDirectory   = errors.New("selected entry is a directory")
)

// TransformSelectedFileBase64 reads the active panel's current file and writes
// a sibling without replacing an existing file. Encoded files use the .b64
// suffix; decoding a .b64 file restores its original name, while other input
// names receive the .decoded suffix.
func TransformSelectedFileBase64(pf *PanelsFrame, encode bool) (string, error) {
	if pf == nil {
		return "", errBase64FileNoSelection
	}
	fsp := pf.GetActivePanel()
	if fsp == nil || fsp.Vfs == nil {
		return "", errBase64FileNoSelection
	}
	rawName := fsp.GetRawSelectedName()
	if rawName == "" || rawName == ".." {
		return "", errBase64FileNoSelection
	}
	ctx := context.Background()
	source := fsp.Vfs.Join(fsp.Vfs.GetPath(), rawName)
	item, err := fsp.Vfs.Stat(ctx, source)
	if err != nil {
		return "", fmt.Errorf("stat source file: %w", err)
	}
	if item.IsDir {
		return "", errBase64FileDirectory
	}

	reader, err := fsp.Vfs.Open(ctx, source)
	if err != nil {
		return "", fmt.Errorf("open source file: %w", err)
	}
	data, readErr := io.ReadAll(contextReader{ctx: ctx, reader: reader})
	closeErr := reader.Close()
	if readErr != nil {
		return "", fmt.Errorf("read source file: %w", readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close source file: %w", closeErr)
	}

	result, err := transformBase64FileData(data, encode)
	if err != nil {
		return "", err
	}
	destination := base64FileDestination(fsp.Vfs, source, encode)
	writer, err := fsp.Vfs.Create(vfs.WithDestinationOverwrite(ctx, false), destination)
	if err != nil {
		return "", fmt.Errorf("create destination file: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = writer.Close()
			_ = fsp.Vfs.Remove(ctx, destination)
		}
	}()
	if _, err := writer.Write(result); err != nil {
		return "", fmt.Errorf("write destination file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close destination file: %w", err)
	}
	keep = true
	fsp.Refresh()
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
	return destination, nil
}

type contextReader struct {
	ctx    context.Context
	reader vfs.ReadAtCloser
}

func (r contextReader) Read(p []byte) (int, error) { return r.reader.Read(r.ctx, p) }

func transformBase64FileData(data []byte, encode bool) ([]byte, error) {
	if encode {
		result := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
		base64.StdEncoding.Encode(result, data)
		return result, nil
	}
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, string(data))
	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err == nil {
		return decoded, nil
	}
	decoded, rawErr := base64.RawStdEncoding.DecodeString(compact)
	if rawErr != nil {
		return nil, fmt.Errorf("invalid Base64 file: %w", err)
	}
	return decoded, nil
}

func base64FileDestination(fs vfs.VFS, source string, encode bool) string {
	dir, name := fs.Dir(source), fs.Base(source)
	if encode {
		name += ".b64"
	} else if len(name) >= len(".b64") && strings.EqualFold(name[len(name)-len(".b64"):], ".b64") {
		name = name[:len(name)-len(".b64")]
	} else {
		name += ".decoded"
	}
	return fs.Join(dir, name)
}
