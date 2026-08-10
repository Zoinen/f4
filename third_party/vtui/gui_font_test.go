package vtui

import (
	"strings"
	"testing"
)

func TestGetFontCandidates(t *testing.T) {
	// 1. Тестируем пустой шрифт (должен вернуть встроенные дефолты)
	defaults := getFontCandidates("")
	if len(defaults) == 0 {
		t.Fatal("Expected default font candidates when fontName is empty")
	}

	foundDefault := false
	for _, path := range defaults {
		if strings.Contains(path, "consola") || strings.Contains(path, "DejaVuSansMono") {
			foundDefault = true
			break
		}
	}
	if !foundDefault {
		t.Error("Default candidates list is missing expected monospace fonts")
	}

	// 2. Тестируем имя кастомного шрифта
	candidates := getFontCandidates("MyCustomFont")
	if len(candidates) <= len(defaults) {
		t.Error("Expected additional candidates when a fontName is specified")
	}

	// Первый кандидат должен быть самим именем и именем с расширением
	if candidates[0] != "MyCustomFont" {
		t.Errorf("Expected first candidate to be 'MyCustomFont', got %q", candidates[0])
	}
	if candidates[1] != "MyCustomFont.ttf" {
		t.Errorf("Expected second candidate to be 'MyCustomFont.ttf', got %q", candidates[1])
	}

	// Проверяем, что сгенерировались пути с системными директориями
	foundDirCandidate := false
	for _, path := range candidates {
		if strings.Contains(path, "MyCustomFont") && (strings.Contains(path, "Fonts") || strings.Contains(path, "fonts")) {
			foundDirCandidate = true
			break
		}
	}
	if !foundDirCandidate {
		t.Error("Expected directory-specific candidates for custom font")
	}
}

func TestGetFontCandidatesResolvesPlatformFamilies(t *testing.T) {
	tests := []struct {
		family string
		path   string
	}{
		{family: "Consolas", path: `C:\Windows\Fonts\consola.ttf`},
		{family: "Menlo", path: "/System/Library/Fonts/Menlo.ttc"},
		{family: "Monaco", path: "/System/Library/Fonts/Monaco.ttf"},
	}
	for _, test := range tests {
		candidates := getFontCandidates(test.family)
		if len(candidates) == 0 || candidates[0] != test.path {
			t.Fatalf("getFontCandidates(%q)[0] = %q, want %q", test.family, candidates[0], test.path)
		}
	}
}

func TestGetFontCandidatesIncludesCollectionAndOpenTypeFiles(t *testing.T) {
	candidates := getFontCandidates("Menlo")
	joined := strings.Join(candidates, "\n")
	for _, suffix := range []string{"Menlo.ttc", "Menlo.otf"} {
		if !strings.Contains(joined, suffix) {
			t.Fatalf("font candidates do not include %q", suffix)
		}
	}
}

func TestLoadBestFont_Fallback(t *testing.T) {
	// При передаче несуществующего шрифта загрузчик обязан корректно откатиться на стандартный шрифт
	face, w, h := loadBestFont("NonExistentFontAtAll", 12.0, 96.0)
	if face == nil {
		t.Error("loadBestFont should never return nil, must fall back safely")
	}
	if w <= 0 || h <= 0 {
		t.Errorf("Invalid font dimensions: %dx%d", w, h)
	}
}
