//go:build lite

package editor

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestLiteColorerRenderFallbacks(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.EditorHighlighter = "Colorer"
	config.App.EditorColorerBackground = true

	for _, region := range []string{"", "def:Text", ColorerHorzCrossRegion, ColorerVertCrossRegion} {
		t.Run(region, func(t *testing.T) {
			if regionDefine := CachedColorerRegionDefine(region); regionDefine != nil {
				t.Fatalf("lite cached region %q = %+v, want nil", region, regionDefine)
			}
			if regionDefine := ColorerGetRegionDefine(region); regionDefine != nil {
				t.Fatalf("lite region %q = %+v, want nil", region, regionDefine)
			}
		})
	}
	base := vtui.SetRGBBoth(vtui.ForegroundIntensity, 0x123456, 0x789abc)
	if got := ColorerEditorBaseAttr(base); got != base {
		t.Fatalf("lite editor base = %#x, want unchanged %#x", got, base)
	}
	if got := ColorerCrossAttr(ColorerHorzCrossRegion, base); got != base {
		t.Fatalf("lite crosshair = %#x, want unchanged %#x", got, base)
	}
}
