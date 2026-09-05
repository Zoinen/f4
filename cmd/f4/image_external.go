package main

// The decoder of last resort. Instead of a decoder inside f4 for each of the
// dozens of formats that exist, the file is handed to whatever converter the
// machine has and PNG is read back. This is how webp, avif, heic, jxl and the
// rest of the long tail become viewable.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/unxed/vtui"
)

const (
	// externalImageDecoder is the name the interface shows and the name the
	// DecoderPriority setting uses.
	externalImageDecoder = "external"

	// externalImagePriority puts the converter below every built-in
	// decoder: starting a process is dear, so it only runs when nothing
	// inside f4 can read the file.
	externalImagePriority = -10

	// defaultImageExternalTimeout bounds one conversion, in seconds. A raw
	// photograph on a slow machine is a few seconds; a converter that has
	// gone to sleep on a malformed file is forever.
	defaultImageExternalTimeout = 20

	// externalImageStderrLimit is how much of the converter's complaint
	// ends up in the error message the viewer shows in its title.
	externalImageStderrLimit = 200

	// externalImagePathPlaceholder stands for the input file in a command
	// line.
	externalImagePathPlaceholder = "{}"
)

// externalImageTool is one converter: the binary, the command line that makes
// it read a file and write PNG to its standard output, and the formats it can
// be trusted with.
type externalImageTool struct {
	Bin     string
	Args    []string
	Formats []string
}

// imageMagickFormats is what ImageMagick is offered. The raw camera formats
// are there because ImageMagick delegates them to dcraw or libraw, which is
// exactly the kind of thing worth not reimplementing.
var imageMagickFormats = []string{
	"webp", "avif", "heic", "heif", "hif",
	"jxl", "jp2", "j2k", "jpf", "jpx",
	"tif", "tiff", "psd", "xcf", "ico", "cur",
	"tga", "pcx", "ppm", "pgm", "pbm", "pnm",
	"xpm", "svg", "svgz", "dds", "exr", "hdr", "wbmp",
	"cr2", "cr3", "nef", "arw", "dng", "orf", "raf", "rw2", "pef", "srw",
}

// ffmpegFormats is the narrower list ffmpeg can be trusted with. Claiming an
// extension is what makes the panel call a file a picture and the viewer open
// it, so claiming one ffmpeg cannot read would only produce an error message
// where a hex dump used to be.
var ffmpegFormats = []string{
	"webp", "avif", "heic", "heif", "hif",
	"jxl", "jp2", "j2k",
	"tif", "tiff", "dds", "exr", "hdr",
	"tga", "pcx", "ppm", "pgm", "pbm", "pnm", "xpm", "wbmp",
}

// externalImageTools, most wanted first. ImageMagick 7 answers to magick,
// ImageMagick 6 to convert.
var externalImageTools = []externalImageTool{
	{
		Bin:     "magick",
		Args:    []string{externalImagePathPlaceholder, "png:-"},
		Formats: imageMagickFormats,
	},
	{
		Bin:     "convert",
		Args:    []string{externalImagePathPlaceholder, "png:-"},
		Formats: imageMagickFormats,
	},
	{
		Bin: "ffmpeg",
		Args: []string{
			"-nostdin", "-v", "error",
			"-i", externalImagePathPlaceholder,
			"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-",
		},
		Formats: ffmpegFormats,
	},
}

// externalImageLookPath and externalImageRun are the seams the tests replace,
// so that nothing here depends on what happens to be installed.
var (
	externalImageLookPath = exec.LookPath
	externalImageRun      = runExternalImageTool
	externalImageTimeout  = configuredExternalImageTimeout
)

// findExternalImageTool returns the first converter that is actually there.
func findExternalImageTool() (externalImageTool, bool) {
	for _, tool := range externalImageTools {
		// On Windows convert.exe is the file system conversion utility
		// that ships with the system, not ImageMagick. ImageMagick 7
		// answers to magick everywhere, so nothing is lost.
		if tool.Bin == "convert" && runtime.GOOS == "windows" {
			continue
		}
		path, err := externalImageLookPath(tool.Bin)
		if err != nil || path == "" {
			continue
		}
		return externalImageTool{Bin: path, Args: tool.Args, Formats: tool.Formats}, true
	}
	return externalImageTool{}, false
}

// configuredExternalImageTimeout reads the [Images] ExternalTimeout setting.
func configuredExternalImageTimeout() time.Duration {
	seconds := AppConfig.ImageExternalTimeout
	if seconds <= 0 {
		seconds = defaultImageExternalTimeout
	}
	return time.Duration(seconds) * time.Second
}

// runExternalImageTool starts the converter and collects its output. Nothing
// is connected to the terminal: the child gets no standard input at all and
// writes into buffers, so it cannot scribble on the screen f4 is drawing.
func runExternalImageTool(ctx context.Context, tool externalImageTool, path string) ([]byte, error) {
	args := make([]string, len(tool.Args))
	for i, arg := range tool.Args {
		args[i] = strings.ReplaceAll(arg, externalImagePathPlaceholder, path)
	}

	cmd := exec.CommandContext(ctx, tool.Bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdin = nil
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %v%s", filepath.Base(tool.Bin), err,
			externalImageStderrTail(stderr.Bytes()))
	}
	return stdout.Bytes(), nil
}

// externalImageStderrTail returns the last line of what the converter
// complained about, short enough to fit in a title bar.
func externalImageStderrTail(stderr []byte) string {
	text := strings.TrimSpace(string(stderr))
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	text = strings.TrimSpace(lines[len(lines)-1])
	if text == "" {
		return ""
	}
	if runes := []rune(text); len(runes) > externalImageStderrLimit {
		text = string(runes[:externalImageStderrLimit])
	}
	return ": " + text
}

// decodeImageExternally converts the bytes through the found tool.
func decodeImageExternally(ctx context.Context, data []byte) (*vtui.ImageSurface, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("there is nothing to convert")
	}
	tool, ok := findExternalImageTool()
	if !ok {
		return nil, fmt.Errorf("no external image converter on the PATH")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// The bytes go through a file rather than through a pipe. heic, avif
	// and jxl are containers that are read by seeking around them, and both
	// ImageMagick's delegates and ffmpeg refuse a stream they cannot
	// rewind. The whole file is in memory already, so this costs one write.
	file, err := os.CreateTemp("", "f4img-*"+sniffImageSuffix(data))
	if err != nil {
		return nil, err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	limit := externalImageTimeout()
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	out, err := externalImageRun(ctx, tool, name)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("%s timed out after %s", filepath.Base(tool.Bin), limit)
		}
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s produced nothing", filepath.Base(tool.Bin))
	}
	return decodeImageWithStdlib(out)
}

// sniffImageSuffix guesses an extension from the first bytes. The converters
// recognise their formats by content in the end, but ImageMagick picks a
// delegate by the name before it looks inside, so a name that agrees with the
// content saves it a guess. An unrecognised header gets no suffix rather than
// a wrong one.
func sniffImageSuffix(data []byte) string {
	switch {
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return ".webp"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		// A container. mp4 and mov start the same way, and calling one of
		// those a heic would send ImageMagick to the wrong delegate, so
		// only the brands that really are pictures are named.
		switch string(data[8:12]) {
		case "avif", "avis":
			return ".avif"
		case "jxl ":
			return ".jxl"
		case "heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "msf1":
			return ".heic"
		}
		return ""
	case len(data) >= 12 && string(data[0:12]) == "\x00\x00\x00\x0cJXL \r\n\x87\n":
		return ".jxl"
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0x0A:
		return ".jxl"
	case len(data) >= 4 && (string(data[0:4]) == "II*\x00" || string(data[0:4]) == "MM\x00*"):
		return ".tiff"
	case len(data) >= 4 && string(data[0:4]) == "8BPS":
		return ".psd"
	case len(data) >= 4 && string(data[0:4]) == "\x00\x00\x01\x00":
		return ".ico"
	case len(data) >= 8 && string(data[0:8]) == "\x89PNG\r\n\x1a\n":
		return ".png"
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return ".jpg"
	case len(data) >= 6 && (string(data[0:6]) == "GIF87a" || string(data[0:6]) == "GIF89a"):
		return ".gif"
	case looksLikeSVG(data):
		return ".svg"
	}
	return ""
}

// looksLikeSVG tells a drawing from the other things that begin with a tag.
func looksLikeSVG(data []byte) bool {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	text := strings.ToLower(string(head))
	return strings.Contains(text, "<svg")
}

// registerExternalImageDecoder adds the converter to the registry when there
// is one to add, and takes it out again when there is not. Registering it
// unconditionally would make IsImageFile say yes to a webp on a machine that
// cannot open it, and the viewer would open on an error message.
func registerExternalImageDecoder() bool {
	tool, ok := findExternalImageTool()
	if !ok {
		UnregisterImageDecoder(externalImageDecoder)
		return false
	}
	RegisterImageDecoder(ImageDecoder{
		Name:       externalImageDecoder,
		Priority:   externalImagePriority,
		Extensions: tool.Formats,
		DecodeCtx:  decodeImageExternally,
	})
	return true
}

func init() {
	registerExternalImageDecoder()
}
