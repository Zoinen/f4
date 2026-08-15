package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/plugins/chroma"
)

// This benchmark is intentionally separate from correctness tests.  It gives
// the QML window implementation a realistic row/run budget: every semantic run
// becomes two QQuickItems in the current Rectangle+Text delegate topology.
func BenchmarkDocumentSurfaceSemanticEditorWindow(b *testing.B) {
	cases := []struct {
		name string
		line func(int) string
	}{
		{
			name: "ordinary_go",
			line: func(index int) string {
				return fmt.Sprintf("func fn%03d() { result := map[string]int{\"alpha\": %d, \"beta\": %d}; println(result) }\n",
					index, index, index+1)
			},
		},
		{
			name: "token_dense_go",
			line: func(index int) string {
				var line strings.Builder
				line.WriteString("func dense() {")
				for token := 0; token < 28; token++ {
					fmt.Fprintf(&line, "x%d:=\"s%d\";", token, index+token)
				}
				line.WriteString("}\n")
				return line.String()
			},
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			var content strings.Builder
			for line := 0; line < 512; line++ {
				content.WriteString(tc.line(line))
			}
			editor := NewEditorView(piecetable.New([]byte(content.String())), nil,
				"document-surface-benchmark.go")
			defer editor.Close()
			editor.highlighter = (&chroma.ChromaProvider{}).Create(
				"document-surface-benchmark.go", "")
			editor.SetPosition(0, 0, 159, 30)
			editor.SetVisible(true)
			editor.ensureEngineWidth()
			editor.ScrollTopRow = 64

			window := editor.semanticWindow()
			styled := semanticStyledEditorWindowRows(editor, window,
				editor.X2-editor.X1)
			runs := 0
			for _, row := range styled {
				runs += len(row.Runs)
			}
			rows := max(1, len(styled))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				editor.ScrollTopRow = 64 + iteration%128
				window = editor.semanticWindow()
				_ = semanticStyledEditorWindowRows(editor, window,
					editor.X2-editor.X1)
			}
			b.ReportMetric(float64(rows), "window_rows")
			b.ReportMetric(float64(runs)/float64(rows), "runs/row")
			b.ReportMetric(float64(1+rows*4+runs*2), "estimated_QQuickItems")
		})
	}
}
