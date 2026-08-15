package vtui

import (
	"strings"
	"testing"
)

func TestStandardDialogs_LayoutValidation(t *testing.T) {
	SetDefaultPalette()

	// Mock languages to test dynamic resizing with different string lengths.
	// This ensures that long button names or labels don't cause overlaps
	// in dynamically generated dialogs like ShowMessage or InputBox.
	mockPacks := []LanguagePack{
		{Name: "en", Strings: map[string]string{
			"vtui.Ok":     "&Ok",
			"vtui.Cancel": "Cancel",
			"vtui.Yes":    "&Yes",
			"vtui.No":     "&No",
		}},
		{Name: "de-extreme", Strings: map[string]string{
			"vtui.Ok":     "&Einverstanden",
			"vtui.Cancel": "&Abbrechen",
			"vtui.Yes":    "&Ja, natürlich",
			"vtui.No":     "&Nein, auf keinen Fall",
		}},
	}

	// Registry of standard vtui dialog scenarios
	dialogs := map[string]func() Container{
		"ShowMessage_1Button": func() Container {
			return ShowMessage(" Info ", "A short message.", []string{Msg("vtui.Ok")})
		},
		"ShowMessage_3Buttons_LongText": func() Container {
			return ShowMessage(" Warning ", "This is a very long message that spans multiple lines.\nDo you want to proceed anyway?", []string{Msg("vtui.Yes"), Msg("vtui.No"), Msg("vtui.Cancel")})
		},
		"InputBox": func() Container {
			return InputBox(" Rename ", "New name:", "document.txt", nil)
		},
	}

	for name, factory := range dialogs {
		t.Run(name, func(t *testing.T) {
			errs := ValidateLayoutInLanguages(mockPacks, func() Container {
				// Re-init FrameManager and Screen for each language pass
				scr := NewSilentScreenBuf()
				scr.AllocBuf(120, 60)
				FrameManager.Init(scr)

				// factory() typically calls FrameManager.Push() and returns the *Window
				dlg := factory()
				return dlg
			})

			if len(errs) > 0 {
				var msgs []string
				for _, e := range errs {
					msgs = append(msgs, e.Error())
				}
				t.Errorf("Layout validation failed for standard dialog '%s':\n%s", name, strings.Join(msgs, "\n"))
			}
		})
	}
}
