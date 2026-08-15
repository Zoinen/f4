package vtui

import (
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

func TestFallbackFace_Delegation(t *testing.T) {
	face := &fallbackFace{
		faces: []font.Face{basicfont.Face7x13},
	}
	defer face.Close()

	adv, ok := face.GlyphAdvance('A')
	if !ok || adv <= 0 {
		t.Errorf("Expected positive glyph advance for 'A', got %v, ok=%v", adv, ok)
	}

	metrics := face.Metrics()
	if metrics.Height <= 0 {
		t.Errorf("Expected valid font metrics height, got %v", metrics.Height)
	}
}

func TestFallbackFontPaths_Sane(t *testing.T) {
	if len(fallbackFontPaths) == 0 {
		t.Fatal("fallbackFontPaths must not be empty")
	}

	seen := make(map[string]bool, len(fallbackFontPaths))
	for _, path := range fallbackFontPaths {
		if seen[path] {
			t.Errorf("duplicate fallback font path: %s", path)
		}
		seen[path] = true

		unix := strings.HasPrefix(path, "/")
		windows := strings.HasPrefix(path, `C:\`)
		if !unix && !windows {
			t.Errorf("fallback font path must be absolute: %s", path)
		}
	}
}

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
