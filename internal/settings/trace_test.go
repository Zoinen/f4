package settings

import (
	"strings"
	"testing"

	"github.com/unxed/f4/sdk/f4settings"
)

func TestSettingsTraceHidesSecretAndMultilineValues(t *testing.T) {
	for _, kind := range []f4settings.Kind{f4settings.Secret, f4settings.Multiline} {
		got := settingsTraceValue(f4settings.Field{Kind: kind}, "TOKEN=abc")
		if strings.Contains(got, "abc") {
			t.Errorf("kind %v leaked the value: %s", kind, got)
		}
	}
	if got := settingsTraceValue(f4settings.Field{Kind: f4settings.Integer}, "4"); got != `"4"` {
		t.Errorf("integer traced as %s", got)
	}
}

func TestSettingsTraceRecordDiffNamesKeysNotValues(t *testing.T) {
	baseline := []f4settings.Record{{ID: "a", Values: map[string]string{"n": "x", "v": "SECRET"}}}
	rows := []f4settings.Record{{ID: "a", Values: map[string]string{"n": "x", "v": "OTHER"}}, {ID: "new:1", Values: map[string]string{"n": ""}}}
	got := settingsTraceRecordDiff(rows, baseline)
	if !strings.Contains(got, "a changed v") || !strings.Contains(got, "added new:1") || strings.Contains(got, "SECRET") || strings.Contains(got, "OTHER") {
		t.Fatalf("diff = %q", got)
	}
}
