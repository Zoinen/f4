package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/unxed/vtui"
)

var errBase64NoSelection = errors.New("select text first")

// transformBase64Selection replaces one ordinary text selection as a single
// undoable editor edit. Rectangular selections are deliberately rejected:
// treating several visual columns as one byte stream would make the result
// surprising and could not be reversed without inventing a new shape.
func (ev *EditorView) transformBase64Selection(encode bool) error {
	if ev.rectSelActive {
		return errors.New("Base64 transformation requires a linear selection")
	}
	min, max := ev.getSelectionRange()
	if max <= min {
		return errBase64NoSelection
	}
	data, err := ev.pt.GetRange(min, max-min)
	if err != nil {
		return fmt.Errorf("read selection: %w", err)
	}

	var replacement []byte
	if encode {
		replacement = []byte(base64.StdEncoding.EncodeToString(data))
	} else {
		// Accept wrapped Base64 copied from a mail or a terminal while keeping
		// all non-whitespace characters subject to strict Base64 validation.
		compact := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, string(data))
		decoded, decodeErr := base64.StdEncoding.DecodeString(compact)
		if decodeErr != nil {
			// RawStdEncoding is useful for URL/query snippets that omit the
			// optional trailing padding; it remains strict about the alphabet.
			decoded, rawErr := base64.RawStdEncoding.DecodeString(compact)
			if rawErr != nil {
				return fmt.Errorf("invalid Base64 text: %w", decodeErr)
			}
			replacement = decoded
		} else {
			replacement = decoded
		}
	}

	ev.replaceRange(min, max, replacement)
	return nil
}

func (ev *EditorView) showBase64Menu() {
	if vtui.FrameManager == nil {
		return
	}
	menu := vtui.NewVMenu(Msg("Editor.Base64.Title"))
	menu.AddItem(vtui.MenuItem{Text: Msg("Action.Editor.Base64Encode")})
	menu.AddItem(vtui.MenuItem{Text: Msg("Action.Editor.Base64Decode")})

	screenW, screenH := vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight()
	w := vtui.StringWidth(Msg("Editor.Base64.Title")) + 8
	for _, item := range menu.Items {
		if itemW := vtui.StringWidth(item.Text) + 6; itemW > w {
			w = itemW
		}
	}
	h := len(menu.Items) + 2
	x := (screenW - w) / 2
	y := (screenH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	menu.SetPosition(x, y, x+w-1, y+h-1)
	menu.OnAction = func(index int) {
		menu.Close()
		if index < 0 || index >= len(menu.Items) {
			return
		}
		encode := index == 0
		if err := ev.transformBase64Selection(encode); err != nil {
			message := err.Error()
			if errors.Is(err, errBase64NoSelection) {
				message = Msg("Editor.Base64.NoSelection")
			}
			vtui.ShowMessage(Msg("Editor.Base64.Title"), message, []string{Msg("vtui.Ok")})
		}
	}
	vtui.FrameManager.Push(menu)
}
