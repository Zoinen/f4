package plughost

import (
	testing "testing"
)

import ()

func TestExtUiRenderer_CoveredTerminalRedrawDeferralRequiresNegotiatedCoveredScene(t *testing.T) {
	tests := []struct {
		name       string
		negotiated bool
		suppressed bool
		mutate     func(map[string]any)
		want       bool
	}{
		{name: "owned native panels", suppressed: true, want: true},
		{name: "legacy cell surface", suppressed: false, want: false},
		{
			name: "text presentation", negotiated: true, suppressed: false,
			mutate: func(scene map[string]any) { scene["presentation"] = "text" },
			want:   false,
		},
		{
			name: "negotiated fallback scene", negotiated: true, suppressed: false,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["fallback"] = true
			},
			want: true,
		},
		{
			name: "legacy fallback scene", suppressed: false,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["fallback"] = true
			},
			want: false,
		},
		{
			name: "terminal revealed beside one panel", suppressed: true,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["showRightPanel"] = false
			},
			want: false,
		},
		{
			name: "terminal revealed below shortened panel", suppressed: true,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["panelLayout"] = map[string]any{
					"columns": 100, "splitColumn": 50,
					"leftBottomInsetRows": 1, "rightBottomInsetRows": 0,
				}
			},
			want: false,
		},
		{
			name: "wide panel exposes terminal area", suppressed: true,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["wide"] = true
			},
			want: false,
		},
		{
			name: "terminal mode", suppressed: true,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["mode"] = "terminal"
			},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scene := terminalOutputRedrawTestScene()
			if tc.mutate != nil {
				tc.mutate(scene)
			}
			renderer := &ExtUiRenderer{
				lastScene:                    scene,
				NativeSemanticSurfaceEnabled: tc.negotiated,
				nativeCellFrameSuppressed:    tc.suppressed,
			}
			if got := renderer.CanDeferCoveredTerminalRedraw(); got != tc.want {
				t.Fatalf("CanDeferCoveredTerminalRedraw() = %v, want %v", got, tc.want)
			}
		})
	}
}

func terminalOutputRedrawTestScene() map[string]any {
	return panelActivationFastPathScene(0, `Panels: D:\Code\f4`)
}
