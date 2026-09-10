package editor

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
	"strings"
	"unicode"
)

var errBase64NoSelection = errors.New("select text first")

// transformBase64Selection replaces one ordinary text selection as a single
// undoable editor edit. Rectangular selections are deliberately rejected:
// treating several visual columns as one byte stream would make the result
// surprising and could not be reversed without inventing a new shape.
func (ev *EditorView) TransformBase64Selection(encode bool) error {
	if ev.RectSelActive {
		return errors.New("Base64 transformation requires a linear selection")
	}
	min, max := ev.GetSelectionRange()
	if max <= min {
		return errBase64NoSelection
	}
	data, err := ev.Pt.GetRange(min, max-min)
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

// ShowPluginsMenu opens the editor's F11 operations menu.
func (ev *EditorView) ShowPluginsMenu() {
	if vtui.FrameManager == nil {
		return
	}
	menu := vtui.NewVMenu(i18n.Msg("Editor.Plugins.Title"))
	menu.AddItem(vtui.MenuItem{Text: i18n.Msg("Action.Editor.Base64Encode")})
	menu.AddItem(vtui.MenuItem{Text: i18n.Msg("Action.Editor.Base64Decode")})
	menu.AddSeparator()
	menu.AddItem(vtui.MenuItem{Text: i18n.Msg("Action.Editor.SortLines")})

	screenW, screenH := vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight()
	w := vtui.StringWidth(i18n.Msg("Editor.Plugins.Title")) + 8
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
		switch index {
		case 0, 1:
			if err := ev.TransformBase64Selection(index == 0); err != nil {
				message := err.Error()
				if errors.Is(err, errBase64NoSelection) {
					message = i18n.Msg("Editor.Base64.NoSelection")
				}
				vtui.ShowMessage(i18n.Msg("Editor.Plugins.Title"), message, []string{i18n.Msg("vtui.Ok")})
			}
		case 3:
			ev.ShowSortDialog()
		}
	}
	vtui.FrameManager.Push(menu)
}

// ShowBase64Menu is kept as a compatibility alias for editor integrations
// that used the old method name before the F11 menu gained other operations.
func (ev *EditorView) ShowBase64Menu() { ev.ShowPluginsMenu() }
