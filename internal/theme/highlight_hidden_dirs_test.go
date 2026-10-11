package theme

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

// firstMatchingNormalColor is the NormalColor of the rule that decides how the
// item is painted: the first one it matches, as in the panel.
func firstMatchingNormalColor(rules []HighlightRule, item *vfs.VFSItem) string {
	for i := range rules {
		if rules[i].Match(item) {
			return rules[i].NormalStr
		}
	}
	return ""
}

// A hidden folder used to match the hidden-files rule and came out the same
// colour as a hidden file, so only the status line told them apart (f4#1793).
func TestBuiltInStylesPaintHiddenDirectoriesApartFromHiddenFiles(t *testing.T) {
	styles := loadStylesFromFS(builtInStyles, "styles/*.ini")
	if len(styles) == 0 {
		t.Fatal("no built-in styles loaded")
	}

	hiddenFile := vfs.VFSItem{Name: ".config.txt", IsHidden: true}
	hiddenDir := vfs.VFSItem{Name: ".config", IsDir: true, IsHidden: true}
	plainDir := vfs.VFSItem{Name: "config", IsDir: true}
	for _, style := range styles {
		rules := ParseHighlightRules(style.ini)
		file := firstMatchingNormalColor(rules, &hiddenFile)
		dir := firstMatchingNormalColor(rules, &hiddenDir)
		plain := firstMatchingNormalColor(rules, &plainDir)
		if file == "" || dir == "" || plain == "" {
			t.Errorf("built-in style %q leaves a hidden file, a hidden directory or a directory unpainted: %q, %q, %q", style.Name, file, dir, plain)
			continue
		}
		if dir == file {
			t.Errorf("built-in style %q paints a hidden directory like a hidden file: %s", style.Name, dir)
		}
		if dir == plain {
			t.Errorf("built-in style %q paints a hidden directory like an ordinary one: %s", style.Name, dir)
		}
	}
}
