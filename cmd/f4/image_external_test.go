package main

import (
	"context"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

// fakeExternalTools makes findExternalImageTool believe exactly these
// binaries are installed, and puts everything back afterwards. Without it the
// tests would say different things on a machine with ImageMagick and on one
// without.
func fakeExternalTools(t *testing.T, bins ...string) {
	t.Helper()

	present := make(map[string]bool, len(bins))
	for _, bin := range bins {
		present[bin] = true
	}

	savedLook := externalImageLookPath
	savedRun := externalImageRun
	savedTimeout := externalImageTimeout
	savedDecoders := imageDecoders
	savedConfig := AppConfig.ImageExternalTimeout
	t.Cleanup(func() {
		externalImageLookPath = savedLook
		externalImageRun = savedRun
		externalImageTimeout = savedTimeout
		imageDecoders = savedDecoders
		AppConfig.ImageExternalTimeout = savedConfig
		SetImageDecoderPriorities(nil)
	})

	externalImageLookPath = func(bin string) (string, error) {
		if present[bin] {
			return filepath.Join("/opt/bin", bin), nil
		}
		return "", os.ErrNotExist
	}
}

func hasFormat(formats []string, want string) bool {
	for _, f := range formats {
		if f == want {
			return true
		}
	}
	return false
}

func TestSniffImageSuffix(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), ".webp"},
		{"avif", []byte("\x00\x00\x00\x18ftypavif\x00\x00\x00\x00"), ".avif"},
		{"heic", []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"), ".heic"},
		{"jxl container", []byte("\x00\x00\x00\x0cJXL \r\n\x87\n"), ".jxl"},
		{"jxl codestream", []byte("\xff\x0a\x00\x00"), ".jxl"},
		{"tiff little endian", []byte("II*\x00rest of it"), ".tiff"},
		{"tiff big endian", []byte("MM\x00*rest of it"), ".tiff"},
		{"photoshop", []byte("8BPS\x00\x01\x00\x00"), ".psd"},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0d"), ".png"},
		{"drawing", []byte("<?xml version=\"1.0\"?>\n<svg width=\"10\"></svg>"), ".svg"},
		{"a film is not a picture", []byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00"), ""},
		{"nothing recognisable", []byte("just some bytes here"), ""},
		{"too short to tell", []byte("II"), ""},
	}
	for _, c := range cases {
		if got := sniffImageSuffix(c.data); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFindExternalImageToolPrefersImageMagick(t *testing.T) {
	fakeExternalTools(t, "convert", "ffmpeg", "magick")

	tool, ok := findExternalImageTool()
	if !ok {
		t.Fatal("three converters are installed and none was found")
	}
	if filepath.Base(tool.Bin) != "magick" {
		t.Fatalf("chose %q, expected magick", tool.Bin)
	}
	if !hasFormat(tool.Formats, "psd") {
		t.Error("ImageMagick reads psd and should claim it")
	}
}

func TestFindExternalImageToolFallsBackToFFmpeg(t *testing.T) {
	fakeExternalTools(t, "ffmpeg")

	tool, ok := findExternalImageTool()
	if !ok {
		t.Fatal("ffmpeg is installed and was not found")
	}
	if filepath.Base(tool.Bin) != "ffmpeg" {
		t.Fatalf("chose %q, expected ffmpeg", tool.Bin)
	}
	if !hasFormat(tool.Formats, "webp") {
		t.Error("ffmpeg reads webp and should claim it")
	}
	if hasFormat(tool.Formats, "psd") {
		t.Error("ffmpeg must not claim the formats only ImageMagick reads")
	}
}

func TestFindExternalImageToolReportsAnEmptyPath(t *testing.T) {
	fakeExternalTools(t)

	if _, ok := findExternalImageTool(); ok {
		t.Fatal("nothing is installed and something was found")
	}
}

func TestRegisterExternalImageDecoderFollowsThePath(t *testing.T) {
	fakeExternalTools(t, "magick")

	if !registerExternalImageDecoder() {
		t.Fatal("a converter is installed, the decoder must be registered")
	}
	if !IsImageFile("holiday.webp") {
		t.Error("webp must be viewable once a converter is there")
	}
}

func TestRegisterExternalImageDecoderWithoutAConverter(t *testing.T) {
	fakeExternalTools(t)

	if registerExternalImageDecoder() {
		t.Fatal("nothing is installed, nothing must be registered")
	}
	if IsImageFile("holiday.webp") {
		t.Error("webp must not look viewable when nothing can read it")
	}
}

func TestDecodeImageExternallyConvertsThroughATempFile(t *testing.T) {
	fakeExternalTools(t, "magick")
	converted := makeTestPNG(t, 4, 2, color.RGBA{R: 7, G: 8, B: 9, A: 255})
	source := []byte("RIFF\x00\x00\x00\x00WEBPVP8 and the body")

	var seen string
	externalImageRun = func(ctx context.Context, tool externalImageTool, path string) ([]byte, error) {
		seen = path
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("the converter was given a file it cannot read: %v", err)
			return nil, err
		}
		if string(got) != string(source) {
			t.Error("the temporary file does not hold the original bytes")
		}
		return converted, nil
	}

	surf, err := decodeImageExternally(context.Background(), source)
	if err != nil {
		t.Fatalf("the conversion failed: %v", err)
	}
	if surf.Width != 4 || surf.Height != 2 {
		t.Fatalf("wrong geometry %dx%d", surf.Width, surf.Height)
	}
	if !strings.HasSuffix(seen, ".webp") {
		t.Errorf("the temporary file should be named after the format, got %q", seen)
	}
	if _, err := os.Stat(seen); !os.IsNotExist(err) {
		t.Errorf("the temporary file %q outlived the conversion", seen)
	}
}

func TestDecodeImageExternallyPassesTheDeadlineOn(t *testing.T) {
	fakeExternalTools(t, "magick")
	AppConfig.ImageExternalTimeout = 7

	var left time.Duration
	externalImageRun = func(ctx context.Context, tool externalImageTool, path string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("the converter was started without a deadline")
			return nil, errors.New("no deadline")
		}
		left = time.Until(deadline)
		return nil, errors.New("cannot read it")
	}

	if _, err := decodeImageExternally(context.Background(), []byte("II*\x00body")); err == nil {
		t.Fatal("a converter that fails must produce an error")
	}
	if left <= 6*time.Second || left > 7*time.Second {
		t.Errorf("the deadline is %v away, expected about seven seconds", left)
	}
}

func TestDecodeImageExternallyReportsATimeout(t *testing.T) {
	fakeExternalTools(t, "ffmpeg")
	externalImageTimeout = func() time.Duration { return 20 * time.Millisecond }
	externalImageRun = func(ctx context.Context, tool externalImageTool, path string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	_, err := decodeImageExternally(context.Background(), []byte("RIFF\x00\x00\x00\x00WEBPbody"))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout, got %v", err)
	}
}

func TestDecodeImageExternallyIsCancelledWithItsCaller(t *testing.T) {
	fakeExternalTools(t, "magick")
	ctx, cancel := context.WithCancel(context.Background())
	externalImageRun = func(runCtx context.Context, tool externalImageTool, path string) ([]byte, error) {
		cancel()
		<-runCtx.Done()
		return nil, runCtx.Err()
	}

	if _, err := decodeImageExternally(ctx, []byte("8BPS\x00\x01body")); err == nil {
		t.Fatal("a cancelled conversion must produce an error")
	}
}

func TestDecodeImageExternallyRejectsAnEmptyAnswer(t *testing.T) {
	fakeExternalTools(t, "magick")
	externalImageRun = func(context.Context, externalImageTool, string) ([]byte, error) {
		return nil, nil
	}

	if _, err := decodeImageExternally(context.Background(), []byte("8BPS\x00\x01body")); err == nil {
		t.Fatal("an empty answer is not a picture")
	}
}

func TestDecodeImageExternallyWithoutAConverter(t *testing.T) {
	fakeExternalTools(t)

	if _, err := decodeImageExternally(context.Background(), []byte("II*\x00body")); err == nil {
		t.Fatal("there is nothing to convert with and no error was reported")
	}
}

func TestExternalDecoderIsTheLastResort(t *testing.T) {
	fakeExternalTools(t, "magick")
	registerExternalImageDecoder()

	tried := false
	RegisterImageDecoder(ImageDecoder{
		Name:       "test-webp",
		Priority:   5,
		Extensions: []string{"webp"},
		Decode: func(data []byte) (*vtui.ImageSurface, error) {
			tried = true
			return nil, errors.New("not this time")
		},
	})

	converted := makeTestPNG(t, 2, 2, color.RGBA{B: 200, A: 255})
	externalImageRun = func(context.Context, externalImageTool, string) ([]byte, error) {
		return converted, nil
	}

	list := ImageDecodersFor("a.webp")
	if len(list) != 2 || list[0].Name != "test-webp" || list[1].Name != externalImageDecoder {
		t.Fatalf("wrong order: %v", list)
	}

	_, name, err := DecodeImage("a.webp", []byte("RIFF\x00\x00\x00\x00WEBPbody"))
	if err != nil || name != externalImageDecoder {
		t.Fatalf("got %q %v", name, err)
	}
	if !tried {
		t.Error("the built-in decoder must be given the first chance")
	}
}

func TestConfiguredExternalImageTimeout(t *testing.T) {
	saved := AppConfig.ImageExternalTimeout
	t.Cleanup(func() { AppConfig.ImageExternalTimeout = saved })

	AppConfig.ImageExternalTimeout = 3
	if got := configuredExternalImageTimeout(); got != 3*time.Second {
		t.Errorf("got %v, want three seconds", got)
	}
	for _, bad := range []int{0, -1} {
		AppConfig.ImageExternalTimeout = bad
		if got := configuredExternalImageTimeout(); got != defaultImageExternalTimeout*time.Second {
			t.Errorf("%d seconds should fall back to the default, got %v", bad, got)
		}
	}
}
