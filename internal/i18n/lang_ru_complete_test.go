package i18n

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stringKeys lists the [Strings] keys of a language file in file order.
func stringKeys(t *testing.T, file string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	var keys []string
	inStrings := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inStrings = strings.HasPrefix(trimmed, "[Strings]")
			continue
		}
		if !inStrings || trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if idx := strings.IndexByte(trimmed, '='); idx > 0 {
			keys = append(keys, strings.TrimSpace(trimmed[:idx]))
		}
	}
	return keys
}

// Russian is the project's primary interface language. A key missing from
// ru.lng silently falls back to English, which is how the Alt+F1/F2 key bar
// captions and the Player, Queue and Attributes dialogs ended up half
// translated (#1218). Every new English string must come with its Russian one.
func TestRussianTranslationIsComplete(t *testing.T) {
	have := make(map[string]bool)
	for _, key := range stringKeys(t, filepath.Join("lang", "ru.lng")) {
		have[key] = true
	}
	var missing []string
	for _, key := range stringKeys(t, filepath.Join("lang", "en.lng")) {
		if !have[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Errorf("ru.lng lacks %d keys present in en.lng: %s", len(missing), strings.Join(missing, ", "))
	}
}
