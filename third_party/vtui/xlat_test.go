package vtui

import "testing"

func TestXlator_Translate(t *testing.T) {
	x := NewXlator()

	// Базовая трансляция (без контекста)
	// В LangOther приоритет у loc2lat, поэтому 'q' может сработать не так идеально,
	// как в явном контексте, но для букв без коллизий работает.
	if x.Translate('й') != 'q' {
		t.Errorf("Expected 'q', got '%c'", x.Translate('й'))
	}

	// Контекстно-зависимая трансляция
	x.Track('a') // Включаем латинский контекст
	if x.curLang != LangLatin {
		t.Errorf("Expected LangLatin state")
	}
	if x.Translate('q') != 'й' {
		t.Errorf("Expected 'й', got '%c'", x.Translate('q'))
	}
	if x.Translate('/') != '.' {
		t.Errorf("Expected '.' (after latin), got '%c'", x.Translate('/'))
	}

	x.Track('ф') // Включаем локальный контекст
	if x.curLang != LangLocal {
		t.Errorf("Expected LangLocal state")
	}
	if x.Translate('.') != '/' {
		t.Errorf("Expected '/' (after local), got '%c'", x.Translate('.'))
	}

	// Трансляция строки с учетом контекста
	x.Track('g') // Симулируем начало набора на латинице
	if x.TranscodeString("ghbdtn") != "привет" {
		t.Errorf("Expected 'привет', got %q", x.TranscodeString("ghbdtn"))
	}
}

func TestXlator_EdgeCases(t *testing.T) {
	x := NewXlator()

	// 1. Регистр (Case preservation)
	if x.Translate('R') != 'К' {
		t.Errorf("Upper case failed: got %c", x.Translate('R'))
	}
	if x.Translate('к') != 'r' {
		t.Errorf("Lower case failed: got %c", x.Translate('к'))
	}

	// 2. Неизвестные символы (Numbers, Emoji)
	if x.Translate('5') != '5' {
		t.Errorf("Numbers should remain unchanged, got %c", x.Translate('5'))
	}
	if x.Translate('🍏') != '🍏' {
		t.Errorf("Unicode should remain unchanged, got %c", x.Translate('🍏'))
	}

	// 3. Пустая строка
	if x.TranscodeString("") != "" {
		t.Error("Empty string should result in empty string")
	}

	// 4. Смена контекста (Context Switching Stress)
	x.Track('a') // Latin
	if x.Translate('/') != '.' {
		t.Error("Context Latin failed")
	}

	x.Track('1') // Neutral - should NOT change context
	if x.curLang != LangLatin {
		t.Error("Neutral char changed context")
	}
	if x.Translate('/') != '.' {
		t.Error("Context Latin lost after neutral char")
	}

	x.Track('ф') // Local
	if x.Translate('.') != '/' {
		t.Error("Context Local failed")
	}
}
