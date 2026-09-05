package mediainfo

import (
	"fmt"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type reportWindow struct {
	*vtui.Window
	onEditor func()
}

func (window *reportWindow) ProcessKey(event *vtinput.InputEvent) bool {
	if event != nil && event.KeyDown && event.VirtualKeyCode == vtinput.VK_F4 {
		if window.onEditor != nil {
			window.onEditor()
		}
		return true
	}
	return window.Window.ProcessKey(event)
}

func (plugin *Plugin) showReportDialog(app vfs.App, fs vfs.VFS, sourcePath, editorPath string, report Report, text string, styleFieldNames bool) {
	width, height := 88, 24
	minWidth, minHeight := 54, 12
	if vtui.FrameManager != nil {
		if maximum := vtui.FrameManager.GetScreenSize() - 2; maximum > 20 && width > maximum {
			width = maximum
		}
		if maximum := vtui.FrameManager.GetScreenHeight() - 2; maximum > 8 && height > maximum {
			height = maximum
		}
	}
	if minWidth > width {
		minWidth = width
	}
	if minHeight > height {
		minHeight = height
	}
	title := fmt.Sprintf(" %s: %s ", plugin.text("MediaInfo.Title", "MediaInfo", "MediaInfo"), fs.Base(sourcePath))
	if report.General.Format != "" {
		title = fmt.Sprintf(" %s: %s [%s] ", plugin.text("MediaInfo.Title", "MediaInfo", "MediaInfo"), fs.Base(sourcePath), report.General.Format)
	}
	window := &reportWindow{Window: vtui.NewCenteredDialog(width, height, vtui.TruncateMiddle(title, width-4))}
	window.ShowClose = true
	window.ShowZoom = true
	window.MinW = minWidth
	window.MinH = minHeight

	lines, _ := splitRenderedTextLines(text, renderedTruncationNotice(plugin.reportLanguage()))
	// The view reserves its last column for the conditional scrollbar. Put
	// that column over the dialog's right border, like Far's native dialogs.
	list := newReportTextView(window.X1+2, window.Y1+2, width-2, height-6, lines, styleFieldNames)
	list.SetGrowMode(vtui.GrowHiX | vtui.GrowHiY)
	window.AddItem(list)

	toEditor := vtui.NewButton(0, 0, plugin.text("MediaInfo.ToEditor", "&To editor (F4)", "&В редактор (F4)"))
	copyButton := vtui.NewButton(0, 0, plugin.text("MediaInfo.Copy", "&Copy", "&Копировать"))
	aboutButton := vtui.NewButton(0, 0, plugin.text("MediaInfo.About", "&About", "&О программе"))
	closeButton := vtui.NewButton(0, 0, plugin.text("MediaInfo.Close", "Close", "Закрыть"))
	closeButton.IsDefault = true
	window.AddItem(toEditor)
	window.AddItem(copyButton)
	window.AddItem(aboutButton)
	window.AddItem(closeButton)
	buttons := vtui.NewHBoxLayout(window.X1+2, window.Y2-1, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 1
	buttons.Add(toEditor, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(copyButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(aboutButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(closeButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Apply()
	for _, button := range []*vtui.Button{toEditor, copyButton, aboutButton, closeButton} {
		button.SetGrowMode(vtui.GrowAll)
	}

	openEditor := func() {
		if plugin.openReportEditor(app, fs, editorPath, text) {
			window.Close()
		}
	}
	window.onEditor = openEditor
	toEditor.OnClick = openEditor
	copyButton.OnClick = func() {
		go vtui.SetClipboard(text)
	}
	aboutButton.OnClick = func() {
		message := plugin.text("MediaInfo.AboutText",
			"Pure-Go media metadata reader for f4.\n\nFast Quick View and bounded detailed analysis work directly through local and remote VFS sources. No native MediaInfo library or CGO is required.",
			"Чистая Go-реализация чтения метаданных для f4.\n\nБыстрый просмотр и ограниченный подробный анализ работают напрямую с локальными и удалёнными VFS. Нативная библиотека MediaInfo и CGO не требуются.")
		vtui.ShowMessageOn(window.Window, " "+plugin.text("MediaInfo.About", "About", "О программе")+" ", message, []string{plugin.text("MediaInfo.OK", "&OK", "&ОК")})
	}
	closeButton.OnClick = window.Close
	window.SetFocusedItem(list)
	if anchor, ok := app.(vtui.Frame); ok {
		vtui.FrameManager.PushToFrameScreen(anchor, window)
	} else {
		vtui.FrameManager.Push(window)
	}
}
