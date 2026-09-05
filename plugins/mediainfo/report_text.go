package mediainfo

import "strings"

// splitRenderedTextLines bounds the number of string headers retained by UI
// consumers. A byte ceiling alone is insufficient: a hostile metadata value
// or a multiline Inform template can fit millions of empty lines in 2 MiB.
// The final slot is replaced with a deterministic notice when more rows exist.
func splitRenderedTextLines(text, notice string) ([]string, bool) {
	text = strings.TrimSuffix(text, "\n")
	lines := strings.SplitN(text, "\n", maxRenderedTextLines)
	if len(lines) == maxRenderedTextLines && strings.Contains(lines[len(lines)-1], "\n") {
		lines[len(lines)-1] = notice
		return lines, true
	}
	return lines, false
}

func (plugin *Plugin) render(report Report, technical bool) string {
	text, _ := plugin.renderReport(report, technical)
	return text
}

// renderReport also reports whether the text came from the canonical default
// renderer. The dialog uses that bit to style known field-name spans without
// guessing at the structure of an arbitrary user-supplied Inform template.
func (plugin *Plugin) renderReport(report Report, technical bool) (string, bool) {
	settings := plugin.settings()
	text := ""
	defaultLayout := false
	if settings.Template != "" {
		var err error
		text, err = ExecuteTemplate(report, settings.Template)
		if err != nil {
			plugin.log("MediaInfo template failed: %v", err)
			text = ""
		}
	}
	if text == "" {
		defaultLayout = true
		text = RenderText(report, RenderOptions{
			Language:  plugin.reportLanguage(),
			Technical: technical,
			Compact:   !technical,
		})
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text, defaultLayout
}
