package terminal

import (
	"context"
	"errors"
	"image"
	"testing"

	"github.com/unxed/goclip"
	"github.com/unxed/vtui"
)

type imageClipboardFixture struct {
	contents goclip.Contents
	err      error
}

func (*imageClipboardFixture) Name() string              { return "clipboard-fixture" }
func (*imageClipboardFixture) Available() bool           { return true }
func (*imageClipboardFixture) ReadText() (string, error) { return "stale", nil }
func (*imageClipboardFixture) WriteText(string) error    { return nil }
func (*imageClipboardFixture) Clear() error              { return nil }
func (d *imageClipboardFixture) ReadContents(context.Context) (goclip.Contents, error) {
	return d.contents, d.err
}

func TestReadClipboardContentsUsesActualImageTextAndPreservesTextFallback(t *testing.T) {
	before := goclip.ActiveDriver()
	defer goclip.SetActiveDriver(before)
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	goclip.SetActiveDriver(&imageClipboardFixture{contents: goclip.Contents{Image: img, Text: "actual"}})
	got, err := ReadClipboardContents(context.Background())
	if err != nil || got.Image != img || got.Text != "actual" {
		t.Fatalf("contents = %+v, %v", got, err)
	}
	vtui.SetClipboard("fallback")
	goclip.SetActiveDriver(goclip.NewFileDriver(""))
	got, err = ReadClipboardContents(context.Background())
	if err != nil || got.Image != nil || got.Text != "fallback" {
		t.Fatalf("text fallback = %+v, %v", got, err)
	}
	failure := errors.New("invalid image data")
	goclip.SetActiveDriver(&imageClipboardFixture{err: failure})
	if _, err := ReadClipboardContents(context.Background()); !errors.Is(err, failure) {
		t.Fatal("image read error hidden", err)
	}
}
