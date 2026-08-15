package vtui

// UIStrings holds default strings used by the UI framework itself.
// The application can overwrite these during initialization for localization.
var UIStrings = struct {
	ButtonBrackets [2]rune
	CloseBrackets  [2]rune
	CloseSymbol    rune
	ZoomSymbol     rune
	DefaultHelp    string
}{
	ButtonBrackets: [2]rune{'[', ']'},
	CloseBrackets:  [2]rune{'[', ']'},
	CloseSymbol:    '×',
	ZoomSymbol:     '↕',
	DefaultHelp:    "Contents",
}
