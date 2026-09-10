package f4settings

import (
	"errors"
	"testing"
)

func TestLocalizedErrorPreservesCause(t *testing.T) {
	cause := errors.New("disk full")
	err := Error("saving failed: %w", cause)
	if !errors.Is(err, cause) {
		t.Fatal("error chain lost")
	}
	text := err.(interface {
		Localized(string, func(string) string) string
	}).Localized("ru", func(key string) string { return "Ошибка сохранения: %w" })
	if text != "Ошибка сохранения: disk full" || err.Error() != "saving failed: disk full" {
		t.Fatal("localized error corrupted diagnostic")
	}
}

func TestTextResourceFallbackAndLiteralValues(t *testing.T) {
	english := "Source: %s"
	lookup := func(key string) string {
		if key == ResourceKey(english) {
			return "Источник: %s"
		}
		return "{" + key + "}"
	}
	value := Text{English: english, Args: []any{"C:/profiles/100%/config"}}
	if got := value.Resolve("ru", lookup); got != "Источник: C:/profiles/100%/config" {
		t.Fatalf("formatted translation: %q", got)
	}
	value.Translations = map[string]string{"ru": "Профиль: %s"}
	if got := value.Resolve("ru", lookup); got != "Профиль: C:/profiles/100%/config" {
		t.Fatal("provider translation lost priority")
	}
	value.Literal = true
	if got := value.Resolve("ru", lookup); got != english {
		t.Fatal("literal user value was translated or formatted")
	}
	if got := (Text{English: "An external provider label"}).Resolve("ru", lookup); got != "An external provider label" {
		t.Fatal("English fallback lost")
	}
}
