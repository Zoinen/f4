package testutil

import (
	"github.com/unxed/vtui"
	"strings"
	"testing"
)

func ClickDialogButton(t *testing.T, dlg vtui.Container, btnText string) {
	t.Helper()
	var buttons []string
	var message []string
	for _, itm := range dlg.GetChildren() {
		if b, ok := itm.(*vtui.Button); ok {
			text := GetCleanText(b)
			buttons = append(buttons, text)
			if text == btnText {
				if b.OnClick != nil {
					b.OnClick()
					return
				}
			}
		}
		if line, ok := itm.(*vtui.Text); ok {
			message = append(message, line.GetText())
		}
	}
	title := ""
	if titled, ok := dlg.(interface{ GetTitle() string }); ok {
		title = titled.GetTitle()
	}
	t.Fatalf("Button %q not found in dialog %q; message: %q; buttons: %q", btnText, title, message, buttons)
}

func GetCleanText(item vtui.UIElement) string {
	switch v := item.(type) {
	case *vtui.Button:
		clean, _, _ := vtui.ParseAmpersandString(v.GetText())
		return strings.TrimSpace(strings.Trim(clean, "[]"))
	case *vtui.Checkbox:
		clean, _, _ := vtui.ParseAmpersandString(v.GetText())
		return strings.TrimSpace(clean)
	}
	return ""
}
