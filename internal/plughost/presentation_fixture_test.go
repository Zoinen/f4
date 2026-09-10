package plughost

import "github.com/unxed/vtui"

// LegacySceneProjectionForTest is wired by the external integration fixture.
var LegacySceneProjectionForTest func(*vtui.SemanticContext, map[string]any) map[string]any

func projectSceneForTest(ctx *vtui.SemanticContext, scene map[string]any) map[string]any {
	return LegacySceneProjectionForTest(ctx, scene)
}
