package theme

import (
	"fmt"
	"time"

	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtui"
)

// ColorValidationRules is the vtui.ColorRules FinishColors checks the active
// palette against, once every layer (built-in defaults, the named style,
// any farcolors.ini overrides) is in place and AdjustContrastLevels has had
// its turn.
//
// MinContrastRatio is disabled (0): AdjustContrastLevels (colorspace.go)
// already self-corrects contrast, using far2l's own, deliberately looser
// tolerance (reach ΔE2000 30, or failing that 20, by moving L* only — not
// strict WCAG 4.5; see that file's doc comment for the concrete example it
// calls out). Re-checking WCAG here on top of that would just re-flag
// decorative pairs a shipped f4 theme intentionally leaves under 4.5:1.
// What nothing else in this package checks for is a harsh chroma/hue
// clash — two highly saturated, very different-hued colors sitting right
// next to each other — which is the capability vtui.ValidateColors adds
// (f4#363), so that half of DefaultColorRules is kept as-is.
var ColorValidationRules = func() vtui.ColorRules {
	rules := vtui.DefaultColorRules
	rules.MinContrastRatio = 0
	return rules
}()

// colorValidationToastDuration is how long the toast raised by
// notifyColorIssues stays up. A var, like the other toast durations in this
// codebase (processEnvironmentFailureToastDuration in internal/panel,
// colorerSchemeFailureToastDuration in internal/editor), so a test can
// shorten it.
var colorValidationToastDuration = 4 * time.Second

// colorSlotNames returns a vtui.PaletteColorPairs name table, indexed the
// same way vtui.Palette is, built from every slot ColorSlots knows about
// (f4's own plus the vtui-native ones it reuses).
func colorSlotNames() []string {
	names := make([]string, len(vtui.Palette))
	for _, slot := range ColorSlots {
		if slot.Index >= 0 && slot.Index < len(names) {
			names[slot.Index] = slot.Canonical
		}
	}
	return names
}

// ValidateActiveColors runs vtui.ValidateColorsWithRules (f4#363) over the
// palette FinishColors just finished building, using ColorValidationRules.
// It is exported so a test — or a future caller that wants to show the
// results somewhere other than a toast — can call it directly.
func ValidateActiveColors() []error {
	pairs := vtui.PaletteColorPairs(vtui.Palette, colorSlotNames())
	return vtui.ValidateColorsWithRules(pairs, ColorValidationRules)
}

// notifyColorIssues surfaces color-scheme problems ValidateActiveColors
// found. This mirrors how notifyColorerSchemeFailure
// (internal/editor/colorer.go) surfaces a syntax-colorer failure and
// processEnvironmentFailureToast (internal/panel/frame_procenv.go) surfaces
// an environment-sync failure: a toast is the only visible sign anything
// needs attention, so debug.log gets every message in full right alongside
// it, and the toast itself carries a short, actionable count rather than
// every message (which can run long for a heavily edited Custom scheme).
func notifyColorIssues(errs []error) {
	if len(errs) == 0 {
		return
	}
	for _, e := range errs {
		vtui.DebugLog("COLORS: %v", e)
	}
	plural := "s"
	if len(errs) == 1 {
		plural = ""
	}
	toast.Show(fmt.Sprintf("Color scheme: %d issue%s found, see debug.log for details", len(errs), plural), colorValidationToastDuration)
}
