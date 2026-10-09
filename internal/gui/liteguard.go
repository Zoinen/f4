//go:build lite && !(vtui_noebiten && vtui_nogogpu)

package gui

// A lite build must also pass vtui_noebiten and vtui_nogogpu: without them
// vtui links Ebitengine and gogpu anyway (about 9 MB), and BackendBuilt
// would claim they are absent while they are not. The name below is left
// undefined on purpose, so the compiler stops with it:
//
//	go build -tags lite,vtui_noebiten,vtui_nogogpu ./cmd/f4
var _ = liteBuildNeedsTagsVtuiNoebitenAndVtuiNogogpu
