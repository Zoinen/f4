package envman

import (
	"strings"
	"testing"
)

func TestHelpTextsMatchAndMentionTheManagerKeys(t *testing.T) {
	if strings.Count(helpTextEnglish, "\n- ") != strings.Count(helpTextRussian, "\n- ") {
		t.Error("the English and Russian help list a different number of items")
	}
	for _, key := range []string{"**Ins**", "**Del**", "**F4**", "**F5**", "**Space**", "**F2**", "**F1**", "envman:"} {
		if !strings.Contains(helpTextEnglish, key) || !strings.Contains(helpTextRussian, key) {
			t.Errorf("help does not mention %s", key)
		}
	}
	plugin := &Plugin{}
	if got := plugin.text("EnvMan.NoSuchHelpKey", helpTextEnglish, helpTextRussian); got != helpTextEnglish {
		t.Error("the fallback help text is not used without a catalog entry")
	}
	plugin.showHelp() // no frame manager in a unit test: must not panic
}
