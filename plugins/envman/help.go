package envman

import (
	"github.com/unxed/f4/vfs"
)

const helpTextEnglish = "# Environment Manager\n\n" +
	"Named sets of environment variables (profiles) that f4 applies on top of the environment it started with. Enabled profiles are evaluated from top to bottom; a later assignment wins, and `NAME=` with an empty value removes the variable.\n\n" +
	"## Keys\n\n" +
	"- **Ins** - add a profile; **Del** - delete it\n" +
	"- **F4** - edit the profile in the pane on the right; **Ctrl+Enter** saves, **Esc** cancels\n" +
	"- **F5** - copy the profile\n" +
	"- **Space** - enable or disable the profile\n" +
	"- **Ctrl+Up** / **Ctrl+Down** - move the profile in the list\n" +
	"- **F2** - plugin settings (ignored variables, editor, import from Far Manager 3 on Windows)\n" +
	"- **F1** - this help; **Esc** closes the manager\n\n" +
	"## Values\n\n" +
	"Each line is `NAME=value`. `%NAME%` (and `$NAME`, `${NAME}` on Unix-like systems) expand to values set earlier; write `%%` or `$$` for a literal marker.\n\n" +
	"## From the command line\n\n" +
	"- `envman:` opens this manager\n" +
	"- `envman:+Name`, `envman:-Name`, `envman:*Name` enable, disable or toggle a profile\n" +
	"- `envman:<file` imports `NAME=value` lines, `envman:>file` exports the managed environment\n" +
	"- `envman:e` shows the complete managed environment in the editor\n"

const helpTextRussian = "# Менеджер окружения\n\n" +
	"Именованные наборы переменных окружения (профили), которые f4 накладывает на окружение, с которым он запущен. Включённые профили применяются сверху вниз; более поздняя строка перекрывает раннюю, а `NAME=` с пустым значением удаляет переменную.\n\n" +
	"## Клавиши\n\n" +
	"- **Ins** - добавить профиль; **Del** - удалить\n" +
	"- **F4** - править профиль в правой части; **Ctrl+Enter** сохраняет, **Esc** отменяет\n" +
	"- **F5** - скопировать профиль\n" +
	"- **Space** - включить или выключить профиль\n" +
	"- **Ctrl+Up** / **Ctrl+Down** - переместить профиль по списку\n" +
	"- **F2** - настройки плагина (игнорируемые переменные, редактор, импорт из Far Manager 3 на Windows)\n" +
	"- **F1** - эта справка; **Esc** закрывает менеджер\n\n" +
	"## Значения\n\n" +
	"Каждая строка имеет вид `NAME=value`. `%NAME%` (а в Unix-подобных системах и `$NAME`, `${NAME}`) подставляет значение, заданное раньше; `%%` или `$$` - буквальный знак.\n\n" +
	"## Из командной строки\n\n" +
	"- `envman:` открывает этот менеджер\n" +
	"- `envman:+Имя`, `envman:-Имя`, `envman:*Имя` включают, выключают или переключают профиль\n" +
	"- `envman:<файл` импортирует строки `NAME=value`, `envman:>файл` экспортирует получившееся окружение\n" +
	"- `envman:e` показывает всё управляемое окружение в редакторе\n"

// showHelp is F1 in the profile manager (f4#272): the plugin's own help in
// f4's Markdown viewer, in the interface language.
func (plugin *Plugin) showHelp() {
	title := plugin.text("EnvMan.HelpTitle", "Environment Manager", "Менеджер окружения")
	vfs.ShowPanelHelp(title, plugin.text("EnvMan.Help", helpTextEnglish, helpTextRussian))
}
