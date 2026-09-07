package mediainfo

import (
	"fmt"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func (plugin *Plugin) configure(app vfs.App) {
	settings := plugin.settings()
	width, height := 76, 17
	if vtui.FrameManager != nil {
		if maximum := vtui.FrameManager.GetScreenSize() - 2; maximum > 20 && width > maximum {
			width = maximum
		}
		if maximum := vtui.FrameManager.GetScreenHeight() - 2; maximum > 10 && height > maximum {
			height = maximum
		}
	}
	dialog := vtui.NewCenteredDialog(width, height, " "+plugin.text("MediaInfo.SettingsTitle", "MediaInfo settings", "Настройки MediaInfo")+" ")
	dialog.ShowClose = true

	x := dialog.X1 + 2
	y := dialog.Y1 + 2
	showMenu := vtui.NewCheckbox(x, y, plugin.text("MediaInfo.ShowMenu", "Show in &Plugins menu", "Показывать в меню &Плагины"), false)
	showMenu.State = boolInt(settings.ShowInPluginMenu)
	y++
	quickView := vtui.NewCheckbox(x, y, plugin.text("MediaInfo.EnableQuick", "Enable &Quick View provider", "Включить провайдер &быстрого просмотра"), false)
	quickView.State = boolInt(settings.EnableQuickView)
	y++
	useEditor := vtui.NewCheckbox(x, y, plugin.text("MediaInfo.UseEditor", "Open detailed reports in the &editor", "Открывать подробные отчёты в &редакторе"), false)
	useEditor.State = boolInt(settings.UseEditor)
	y += 2

	labelWidth := 18
	dialog.AddItem(newMediaInfoDialogLabel(x, y, plugin.text("MediaInfo.Prefix", "Command prefix:", "Префикс команды:")))
	prefix := vtui.NewEdit(x+labelWidth, y, width-labelWidth-4, settings.Prefix)
	prefix.SetGrowMode(vtui.GrowHiX)
	y += 2

	dialog.AddItem(newMediaInfoDialogLabel(x, y, plugin.text("MediaInfo.Language", "Report language:", "Язык отчёта:")))
	languages := []string{
		plugin.text("MediaInfo.LanguageAuto", "Automatic", "Автоматически"),
		plugin.text("MediaInfo.LanguageEnglish", "English", "Английский"),
		plugin.text("MediaInfo.LanguageRussian", "Russian", "Русский"),
	}
	language := vtui.NewComboBox(x+labelWidth, y, width-labelWidth-4, languages)
	language.DropdownOnly = true
	selectedLanguage := 0
	switch settings.Language {
	case "en":
		selectedLanguage = 1
	case "ru":
		selectedLanguage = 2
	}
	language.Menu.SetSelectPos(selectedLanguage)
	language.Edit.SetText(languages[selectedLanguage])
	language.SetGrowMode(vtui.GrowHiX)
	y += 2

	dialog.AddItem(newMediaInfoDialogLabel(x, y, plugin.text("MediaInfo.Template", "Inform template:", "Шаблон Inform:")))
	template := vtui.NewEdit(x+labelWidth, y, width-labelWidth-4, settings.Template)
	template.SetGrowMode(vtui.GrowHiX)
	dialog.AddItem(showMenu)
	dialog.AddItem(quickView)
	dialog.AddItem(useEditor)
	dialog.AddItem(prefix)
	dialog.AddItem(language)
	dialog.AddItem(template)

	saveButton := vtui.NewButton(0, 0, plugin.text("MediaInfo.Save", "&Save", "&Сохранить"))
	cancelButton := vtui.NewButton(0, 0, plugin.text("MediaInfo.Cancel", "Cancel", "Отмена"))
	saveButton.IsDefault = true
	dialog.AddItem(saveButton)
	dialog.AddItem(cancelButton)
	buttons := vtui.NewHBoxLayout(dialog.X1+2, dialog.Y2-1, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(saveButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(cancelButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Apply()
	saveButton.SetGrowMode(vtui.GrowAll)
	cancelButton.SetGrowMode(vtui.GrowAll)

	showSaveError := func(err error) {
		vtui.ShowMessageOn(dialog, " "+plugin.text("MediaInfo.SettingsError", "Settings error", "Ошибка настроек")+" ", err.Error(), []string{plugin.text("MediaInfo.OK", "&OK", "&ОК")})
	}
	saveButton.OnClick = func() {
		languageCode := "auto"
		switch language.Menu.SelectPos {
		case 1:
			languageCode = "en"
		case 2:
			languageCode = "ru"
		}
		next := Settings{
			ShowInPluginMenu: showMenu.State == 1,
			EnableQuickView:  quickView.State == 1,
			UseEditor:        useEditor.State == 1,
			Prefix:           prefix.GetText(),
			Template:         template.GetText(),
			Language:         languageCode,
		}
		next = normalizeSettings(next)
		if err := next.validate(); err != nil {
			showSaveError(err)
			return
		}

		plugin.mu.Lock()
		prefixRegistration := plugin.prefix
		store := plugin.store
		plugin.mu.Unlock()
		if prefixRegistration == nil || store == nil {
			showSaveError(fmt.Errorf("MediaInfo is not fully initialized"))
			return
		}
		old := store.snapshot()
		if err := prefixRegistration.SetPrefix(next.Prefix); err != nil {
			showSaveError(err)
			return
		}
		if err := store.save(next); err != nil {
			if rollbackErr := prefixRegistration.SetPrefix(old.Prefix); rollbackErr != nil {
				plugin.log("MediaInfo prefix rollback failed: %v", rollbackErr)
			}
			showSaveError(err)
			return
		}
		dialog.Close()
	}
	cancelButton.OnClick = dialog.Close
	if anchor, ok := app.(vtui.Frame); ok {
		vtui.FrameManager.PushToFrameScreen(anchor, dialog)
	} else {
		vtui.FrameManager.Push(dialog)
	}
}

// A zero explicit color tells vtui.Text to resolve ColDialogText and its
// highlight from the current palette every time it draws. Passing the current
// palette value here would freeze that snapshot if the user switches themes
// while the dialog is open.
func newMediaInfoDialogLabel(x, y int, text string) *vtui.Text {
	return vtui.NewText(x, y, text, 0)
}
