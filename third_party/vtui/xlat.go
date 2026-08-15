package vtui

// LangState представляет текущее предполагаемое состояние раскладки
type LangState int

const (
	LangOther LangState = iota
	LangLatin
	LangLocal
)

// Xlator инкапсулирует логику транслитерации символов между латинской и локальной раскладками
type Xlator struct {
	curLang    LangState
	lat2loc    map[rune]rune
	loc2lat    map[rune]rune
	afterLatin map[rune]rune
	afterLocal map[rune]rune
	latinChars map[rune]bool
	localChars map[rune]bool
}

// GlobalXlator — глобальный экземпляр для прозрачного использования в UI
var GlobalXlator *Xlator

func init() {
	GlobalXlator = NewXlator()
}

func NewXlator() *Xlator {
	x := &Xlator{
		lat2loc:    make(map[rune]rune),
		loc2lat:    make(map[rune]rune),
		afterLatin: make(map[rune]rune),
		afterLocal: make(map[rune]rune),
		latinChars: make(map[rune]bool),
		localChars: make(map[rune]bool),
	}
	x.LoadConfigs(DefaultXLatConfigs)
	return x
}

// LoadConfigs загружает конфигурации раскладок в память.
func (x *Xlator) LoadConfigs(configs []XLatLayoutConfig) {
	for _, cfg := range configs {
		lat := []rune(cfg.Latin)
		loc := []rune(cfg.Local)

		for i := 0; i < len(lat) && i < len(loc); i++ {
			x.lat2loc[lat[i]] = loc[i]
			x.loc2lat[loc[i]] = lat[i]
			x.latinChars[lat[i]] = true
			x.localChars[loc[i]] = true
		}

		for k, v := range cfg.AfterLatin {
			x.afterLatin[k] = v
		}
		for k, v := range cfg.AfterLocal {
			x.afterLocal[k] = v
		}
	}
}

// Track динамически определяет текущую раскладку клавиатуры.
// Если символ не найден в таблицах алфавитов (например, цифра), контекст не меняется.
func (x *Xlator) Track(r rune) {
	if x.latinChars[r] {
		x.curLang = LangLatin
	} else if x.localChars[r] {
		x.curLang = LangLocal
	}
}

// Translate возвращает символ в альтернативной раскладке
func (x *Xlator) Translate(r rune) rune {
	// 1. Применяем правила только если мы уверены в текущем языке.
	// Приоритет отдается целевой таблице, чтобы предотвратить коллизии знаков препинания.
	if x.curLang == LangLocal {
		if val, ok := x.afterLocal[r]; ok {
			return val
		}
		if val, ok := x.loc2lat[r]; ok {
			return val
		}
	} else if x.curLang == LangLatin {
		if val, ok := x.afterLatin[r]; ok {
			return val
		}
		if val, ok := x.lat2loc[r]; ok {
			return val
		}
	}

	// 2. Фолбэк, если контекст неизвестен
	if val, ok := x.loc2lat[r]; ok {
		return val
	}
	if val, ok := x.lat2loc[r]; ok {
		return val
	}
	return r
}

// TranscodeString транслитерирует всю строку
func (x *Xlator) TranscodeString(s string) string {
	runes := []rune(s)
	res := make([]rune, 0, len(runes))
	for _, r := range runes {
		res = append(res, x.Translate(r))
	}
	return string(res)
}
