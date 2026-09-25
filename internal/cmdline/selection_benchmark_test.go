package cmdline

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func BenchmarkNativeBlockSelectionProjection(b *testing.B) {
	oldMultiline, oldWrap := config.App.CommandLineMultiline, config.App.CommandLineWordWrap
	config.App.CommandLineMultiline, config.App.CommandLineWordWrap = true, true
	b.Cleanup(func() {
		config.App.CommandLineMultiline, config.App.CommandLineWordWrap = oldMultiline, oldWrap
	})
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	for _, size := range []struct {
		name    string
		repeats int
	}{{"1KB", 100}, {"100KB", 10000}} {
		b.Run(size.name, func(b *testing.B) {
			cl := NewCommandLine("$ ")
			cl.SetPosition(0, 0, 99, 5)
			cl.Edit.SetText(strings.Repeat("argument x ", size.repeats))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cl.SetNativeBlockSelectionAt(1, 100, vtui.MultilineBlockSelectionGeometry{
					AnchorRow: 0, AnchorColumn: 1, FocusRow: 2, FocusColumn: 2 + i%30, WrapWidth: 80,
				})
				cl.SemanticModel(&vtui.SemanticContext{})
			}
		})
	}
}
