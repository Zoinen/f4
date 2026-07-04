package vtui

// XLatLayoutConfig декларативно описывает правила транслитерации между латинской
// раскладкой и национальной. Архитектура аналогична секциям xlats.ini в far2l.
type XLatLayoutConfig struct {
	Name       string
	Latin      string
	Local      string
	AfterLatin map[rune]rune
	AfterLocal map[rune]rune
}

// DefaultXLatConfigs содержит встроенные раскладки по умолчанию.
// В будущем эту структуру можно будет дополнять из внешнего ini-файла.
var DefaultXLatConfigs = []XLatLayoutConfig{
	{
		Name:  "ru:qwerty-йцукен",
		Latin: "qwertyuiop[]asdfghjkl;'zxcvbnm,./QWERTYUIOP{}ASDFGHJKL:\"ZXCVBNM<>?`~@#$^&|",
		Local: "йцукенгшщзхъфывапролджэячсмитьбю.ЙЦУКЕНГШЩЗХЪФЫВАПРОЛДЖЭЯЧСМИТЬБЮ,ёЁ\"№;:?/",
		// Знаки препинания, имеющие разные физические клавиши в зависимости от раскладки
		AfterLatin: map[rune]rune{'/': '.', '?': ','},
		AfterLocal: map[rune]rune{'.': '/', ',': '?'},
	},
}
