package theme

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

// styleKnownColorIssues names the only color-validation warning any built-in
// style is allowed to still raise. vtui#140 added a harsh chroma/hue clash
// check; #143 and #144 then tuned it against exactly this kind of
// exhaustive, real-scheme sweep, fixing several false positives it
// originally had on f4's own shipped defaults (f4#363). Default Dark's
// Menu.Highlight.Selected and HMenu.Highlight.Selected used to be left
// flagged even after those fixes landed: a saturated red directly on a
// saturated olive green, both around L*≈50 — genuinely near-isoluminant, at
// 1.7:1 WCAG contrast (see vtui#144's
// TestValidateColors_FlagsNearIsoluminantSaturatedClash for the same
// combination as a dedicated vtui-level regression). f4#1622 reported this
// startup warning as user-visible confusion, so default_dark.ini now gives
// both slots the same light foreground Dialog.Button.Highlight.Selected
// already used on the identical green background, clearing the clash; the
// map is empty until a future style needs its own accepted exception.
var styleKnownColorIssues = map[string][]string{}

// TestValidateActiveColors_ShippedStylesPassClean applies every built-in
// style (the actual .ini files under styles/, embedded and shipped with f4)
// and checks that ValidateActiveColors comes back with nothing beyond the
// one known, real issue styleKnownColorIssues documents for that style
// (none, for every style except Default Dark). This is the regression that
// would catch either a validator false positive or a genuine wiring
// regression coming back.
func TestValidateActiveColors_ShippedStylesPassClean(t *testing.T) {
	saved := append([]uint64(nil), vtui.Palette...)
	defer func() { vtui.Palette = saved }()
	oldConfig := config.App
	defer func() { config.App = oldConfig }()
	oldPath := UserColorOverridesPath
	UserColorOverridesPath = func() string { return filepath.Join(t.TempDir(), "none.ini") }
	defer func() { UserColorOverridesPath = oldPath }()

	for _, style := range AvailableColorStyles() {
		if err := ApplyColorStyle(style.Name); err != nil {
			t.Fatalf("ApplyColorStyle(%q): %v", style.Name, err)
		}
		errs := ValidateActiveColors()
		wantSlots := styleKnownColorIssues[style.Name]
		if len(errs) != len(wantSlots) {
			t.Errorf("style %q: expected exactly the known issue(s) %v, got %d: %v", style.Name, wantSlots, len(errs), errs)
			continue
		}
		for i, e := range errs {
			if !strings.Contains(e.Error(), "["+wantSlots[i]+"]") {
				t.Errorf("style %q: issue %d = %v, expected it to name %q", style.Name, i, e, wantSlots[i])
			}
		}
	}
}

// TestValidateActiveColors_FlagsSyntheticClash confirms the wiring itself
// actually surfaces something when a scheme genuinely has a harsh clash: a
// saturated yellow panel on a saturated blue one, the same combination
// vtui's own TestValidateColors_FlagsHarshSaturatedClash uses as its
// genuine-clash regression.
func TestValidateActiveColors_FlagsSyntheticClash(t *testing.T) {
	oldPath := UserColorOverridesPath
	UserColorOverridesPath = func() string { return filepath.Join(t.TempDir(), "none.ini") }
	defer func() { UserColorOverridesPath = oldPath }()
	if err := ApplyColorStyle("Modern"); err != nil {
		t.Fatalf("ApplyColorStyle(%q): %v", "Modern", err)
	}
	saved := append([]uint64(nil), vtui.Palette...)
	defer func() { vtui.Palette = saved }()

	vtui.Palette[ColPanelText] = vtui.SetRGBBoth(0, 0xFFFF00, 0x0000FF)

	errs := ValidateActiveColors()
	if len(errs) == 0 {
		t.Fatal("expected a saturated yellow-on-blue Panel.Text to be flagged as a harsh clash")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "harsh color clash") && strings.Contains(e.Error(), "Panel.Text") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a harsh-color-clash error naming Panel.Text, got: %v", errs)
	}
}

// TestNotifyColorIssues_RaisesARealToast exercises notifyColorIssues against
// a real (silent) FrameManager, the same way
// TestColorer_SchemeFailureAlsoShowsAToast (internal/editor) checks the
// analogous Colorer failure toast: it must stay silent with no errors, and
// actually raise a toast — not just log — otherwise.
func TestNotifyColorIssues_RaisesARealToast(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	previousDuration := colorValidationToastDuration
	colorValidationToastDuration = 2 * time.Second
	t.Cleanup(func() { colorValidationToastDuration = previousDuration })

	notifyColorIssues(nil)
	testutil.DrainUITasks()
	if got := vtui.FrameManager.GetActiveToast(); got != "" {
		t.Errorf("expected no toast for zero errors, got: %q", got)
	}

	notifyColorIssues([]error{vtui.ColorError{Message: "[Panel.Text] harsh color clash: synthetic"}})
	testutil.DrainUITasks()
	if got := vtui.FrameManager.GetActiveToast(); got == "" {
		t.Fatal("expected notifyColorIssues to raise a toast for a nonempty error list")
	}
}
