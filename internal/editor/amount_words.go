package editor

import (
	"errors"
	"fmt"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/numwords"
)

// AmountInWordsSelection replaces the linear selection - a number or an
// amount of money such as "1234.56 руб." or "$12.50" - with it written in
// words, as one undoable edit (f4#1463). A selection that says no language
// (a bare number, a currency symbol) is written in the interface language when
// that is Russian and in English otherwise.
func (ev *EditorView) AmountInWordsSelection() (string, error) {
	if ev.RectSelActive {
		return "", errors.New("amount in words requires a linear selection")
	}
	from, to := ev.GetSelectionRange()
	if to <= from {
		return "", ErrCalculatorNoSelection
	}
	data, err := ev.Pt.GetRange(from, to-from)
	if err != nil {
		return "", fmt.Errorf("read selection: %w", err)
	}
	def := numwords.EN
	if i18n.Msg("Editor.AmountInWords.Lang") == "ru" {
		def = numwords.RU
	}
	words, err := numwords.Amount(string(data), def)
	if err != nil {
		return "", err
	}
	ev.replaceRange(from, to, []byte(words))
	return words, nil
}
