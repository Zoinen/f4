package mediainfo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var errMediaDirectory = errors.New("selected item is a directory")

func (plugin *Plugin) openCurrent(app vfs.App) {
	if app == nil || app.GetActivePanelVFS() == nil {
		plugin.showError(plugin.text("MediaInfo.NoPanel", "No active file panel is available.", "Нет активной файловой панели."))
		return
	}
	name := app.GetSelectedName()
	if name == "" || name == ".." {
		plugin.showError(plugin.text("MediaInfo.NoSelection", "Select a media file first.", "Сначала выберите медиафайл."))
		return
	}
	fs := app.GetActivePanelVFS()
	plugin.openPath(app, fs, fs.Join(fs.GetPath(), name))
}

func (plugin *Plugin) handlePrefix(app vfs.App, rawArgument string) {
	if strings.TrimSpace(rawArgument) == "" {
		plugin.openCurrent(app)
		return
	}
	if app == nil || app.GetActivePanelVFS() == nil {
		plugin.showError(plugin.text("MediaInfo.NoPanel", "No active file panel is available.", "Нет активной файловой панели."))
		return
	}
	fs := app.GetActivePanelVFS()
	path, err := resolveMediaPath(fs, rawArgument)
	if err != nil {
		plugin.showError(err.Error())
		return
	}
	plugin.openPath(app, fs, path)
}

func resolveMediaPath(fs vfs.VFS, raw string) (string, error) {
	if fs == nil {
		return "", errors.New("no active VFS")
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("media path is empty")
	}
	if value[0] == '"' || value[0] == '\'' {
		quote := value[0]
		if len(value) < 2 || value[len(value)-1] != quote {
			return "", errors.New("invalid quoted media path")
		}
		value = value[1 : len(value)-1]
		if quote == '\'' {
			value = strings.ReplaceAll(value, "''", "'")
		} else {
			value = strings.ReplaceAll(value, `\"`, `"`)
		}
	}
	if _, ok := fs.(*vfs.OSVFS); ok {
		value = expandOSPathEnvironment(value)
	}
	if strings.TrimSpace(value) == "" {
		return "", errors.New("media path is empty")
	}
	if !fs.IsAbs(value) {
		value = fs.Join(fs.GetPath(), value)
	}
	absolute, err := fs.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve media path: %w", err)
	}
	return absolute, nil
}

func expandOSPathEnvironment(value string) string {
	value = os.ExpandEnv(value)
	// FAR and cmd.exe users commonly write %NAME%, while Go's ExpandEnv
	// intentionally recognizes only $NAME. Accept both on local panels.
	var expanded strings.Builder
	for offset := 0; offset < len(value); {
		startRelative := strings.IndexByte(value[offset:], '%')
		if startRelative < 0 {
			expanded.WriteString(value[offset:])
			break
		}
		start := offset + startRelative
		expanded.WriteString(value[offset:start])
		endRelative := strings.IndexByte(value[start+1:], '%')
		if endRelative < 0 {
			expanded.WriteString(value[start:])
			break
		}
		end := start + 1 + endRelative
		name := value[start+1 : end]
		if name == "" {
			expanded.WriteByte('%')
			offset = end + 1
			continue
		}
		replacement, exists := os.LookupEnv(name)
		if !exists {
			// Match os.ExpandEnv: an unknown variable expands to empty.
			replacement = ""
		}
		expanded.WriteString(replacement)
		offset = end + 1
	}
	return expanded.String()
}

func (plugin *Plugin) openPath(app vfs.App, fs vfs.VFS, path string) {
	var (
		report     Report
		item       vfs.VFSItem
		editorPath string
	)
	pluginSettings := plugin.settings()
	title := " " + plugin.text("MediaInfo.Title", "MediaInfo", "MediaInfo") + " "
	app.RunProgressTask(title,
		plugin.text("MediaInfo.Opening", "Opening media file...", "Открытие медиафайла..."), false,
		func(ctx context.Context, update func(string, int)) error {
			var err error
			item, err = fs.Stat(ctx, path)
			if err != nil {
				return err
			}
			if item.IsDir {
				return errMediaDirectory
			}
			update(plugin.text("MediaInfo.Analyzing", "Reading media metadata...", "Чтение метаданных..."), -1)
			report, err = plugin.analyzePath(ctx, fs, path, item, ModeDetailed)
			if err != nil {
				return err
			}
			editorPath, err = availableReportPath(ctx, fs, path)
			if pluginSettings.UseEditor {
				return err
			}
			// A read-only VFS can still show and copy the report. Defer a
			// destination error until the user explicitly requests F4.
			return nil
		},
		func(err error) {
			if err != nil {
				plugin.presentAnalysisError(path, err)
				return
			}
			text, defaultLayout := plugin.renderReport(report, true)
			if pluginSettings.UseEditor {
				plugin.openReportEditor(app, fs, editorPath, text)
				return
			}
			plugin.showReportDialog(app, fs, path, editorPath, report, text, defaultLayout)
		})
}

func availableReportPath(ctx context.Context, fs vfs.VFS, sourcePath string) (string, error) {
	name := fs.Base(sourcePath)
	extension := filepath.Ext(name)
	base := strings.TrimSuffix(name, extension)
	if base == "" {
		base = name
	}
	directory := fs.Dir(sourcePath)
	for index := 1; index <= 999; index++ {
		suffix := ""
		if index > 1 {
			suffix = fmt.Sprintf(" (%d)", index)
		}
		candidate := fs.Join(directory, base+".MediaInfo"+suffix+".txt")
		_, err := fs.Stat(ctx, candidate)
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("check report destination: %w", err)
		}
	}
	return "", errors.New("could not find a free MediaInfo report name")
}

func (plugin *Plugin) openReportEditor(app vfs.App, fs vfs.VFS, outputPath, text string) bool {
	if outputPath == "" {
		plugin.showError(plugin.text("MediaInfo.ReportNameFailed", "Could not choose a collision-free report name on this VFS.", "Не удалось выбрать свободное имя отчёта в этой VFS."))
		return false
	}
	host, ok := app.(vfs.TextEditorHost)
	if !ok {
		plugin.showError(plugin.text("MediaInfo.EditorUnsupported", "This host cannot open an in-memory report in the editor.", "Хост не может открыть отчёт в редакторе."))
		return false
	}
	request := vfs.TextEditorRequest{
		VFS:               fs,
		Path:              outputPath,
		DisplayTitle:      fs.Base(outputPath),
		Content:           []byte(text),
		Modified:          true,
		Temporary:         false,
		TargetKnownAbsent: true,
	}
	if err := host.OpenTextEditor(request); err != nil {
		plugin.showError(fmt.Sprintf("%s\n\n%v", plugin.text("MediaInfo.EditorFailed", "Could not open the report in the editor.", "Не удалось открыть отчёт в редакторе."), err))
		return false
	}
	return true
}

func (plugin *Plugin) presentAnalysisError(path string, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	var parseError *ParseError
	message := ""
	switch {
	case errors.Is(err, errMediaDirectory):
		message = plugin.text("MediaInfo.Directory", "The selected item is a directory.", "Выбранный элемент является каталогом.")
	case errors.Is(err, ErrUnsupported):
		message = plugin.text("MediaInfo.Unsupported", "The selected file format is not supported.", "Формат выбранного файла не поддерживается.")
	case errors.As(err, &parseError):
		message = plugin.text("MediaInfo.Corrupt", "The media container is damaged or malformed.", "Медиаконтейнер повреждён или имеет неверную структуру.")
	default:
		message = plugin.text("MediaInfo.ReadFailed", "Could not read the selected file.", "Не удалось прочитать выбранный файл.")
	}
	plugin.showError(fmt.Sprintf("%s\n\n%s\n%v", message, path, err))
}

func (plugin *Plugin) showError(message string) {
	vtui.ShowMessage(" "+plugin.text("MediaInfo.ErrorTitle", "MediaInfo error", "Ошибка MediaInfo")+" ", message, []string{plugin.text("MediaInfo.OK", "&OK", "&ОК")})
}
