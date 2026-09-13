package dialog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/vtui"
)

const far3LocationHistoryID = "far3-import-location"

// Far3HistoryPath accepts an installation directory, profile directory or DB.
// Installation lookup respects Far.exe.ini, including system and local profiles.
func Far3HistoryPath(source string) (string, error) {
	source = strings.Trim(strings.TrimSpace(source), "\"")
	if source == "" {
		return "", fmt.Errorf("%s", i18n.Msg("History.Far3.EmptyPath"))
	}
	source = os.ExpandEnv(source)
	info, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return source, nil
	}
	direct := filepath.Join(source, "history.db")
	if info, err := os.Stat(direct); err == nil && !info.IsDir() {
		return direct, nil
	}
	if _, err := os.Stat(filepath.Join(source, "Far.exe.ini")); err != nil {
		if _, err := os.Stat(filepath.Join(source, "Far.exe")); err != nil {
			return "", fmt.Errorf("%s: %s", source, i18n.Msg("History.Far3.NotFound"))
		}
	}
	settings, err := readFar3INI(filepath.Join(source, "Far.exe.ini"))
	if err != nil {
		return "", err
	}
	get := func(key, fallback string) string { return settings.GetString("General", key, fallback) }
	var profile string
	switch get("UseSystemProfiles", "1") {
	case "0":
		profile = get("UserLocalProfileDir", get("UserProfileDir", "%FARHOME%/Profile"))
	case "2":
		profile = "%APPDATA%/Far Manager"
	default:
		profile = "%LOCALAPPDATA%/Far Manager"
	}
	profile = expandFar3Path(strings.Trim(profile, "\""), source)
	if !filepath.IsAbs(profile) {
		profile = filepath.Join(source, profile)
	}
	path := filepath.Join(profile, "history.db")
	if info, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	} else if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	return path, nil
}

// Far's Windows INI files commonly use UTF-16 with a BOM. Decode before
// parsing so a portable profile cannot silently become a system profile.
func readFar3INI(path string) (*ini.File, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ini.New(), nil // Far defaults to system profiles without an INI.
	}
	if err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) {
		order = binary.LittleEndian
	} else if bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		order = binary.BigEndian
	}
	if order != nil {
		if len(data)%2 != 0 {
			return nil, fmt.Errorf("%s: incomplete UTF-16 character", path)
		}
		units := make([]uint16, (len(data)-2)/2)
		for i := range units {
			units[i] = order.Uint16(data[2+i*2:])
		}
		data = []byte(string(utf16.Decode(units)))
	}
	return ini.Parse(bytes.NewReader(data)), nil
}

func expandFar3Path(value, home string) string {
	for start := strings.IndexByte(value, '%'); start >= 0; {
		end := strings.IndexByte(value[start+1:], '%')
		if end < 0 {
			break
		}
		end += start + 1
		key := value[start+1 : end]
		replacement := os.Getenv(key)
		if strings.EqualFold(key, "FARHOME") {
			replacement = home
		}
		value = value[:start] + replacement + value[end+1:]
		next := strings.IndexByte(value[start+len(replacement):], '%')
		if next < 0 {
			break
		}
		start += len(replacement) + next
	}
	return os.ExpandEnv(value)
}

// ShowFar3HistoryImport uses the shared dialog controls in both console and Qt.
func ShowFar3HistoryImport(hp *history.F4HistoryProvider, onImported func()) *FileDialog {
	initial := defaultFar3Location()
	if saved := hp.LoadHistory(far3LocationHistoryID); len(saved) > 0 {
		initial = saved[0]
	}
	dlg := NewFileDialog(i18n.Msg("History.Far3.Title"), 11)
	edit := vtui.NewEdit(0, 0, 10, initial)
	label := vtui.NewLabel(0, 0, i18n.Msg("History.Far3.Path"), edit)
	note := vtui.NewLabel(0, 0, i18n.Msg("History.Far3.Merge"), nil)
	importButton := vtui.NewButton(0, 0, i18n.Msg("History.Far3.Import"))
	importButton.IsDefault = true
	cancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	for _, item := range []vtui.UIElement{label, edit, note, importButton, cancel} {
		dlg.AddItem(item)
	}
	width, height := dlg.Size()
	layout := vtui.NewAutoLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	layout.PinTop(label, 0).PinLeft(label, 0).
		StackVertical(1, label, edit, note).FillWidth(edit, 0, 0).PinLeft(note, 0).
		PinBottom(importButton, 0).PinBottom(cancel, 0).
		StackHorizontal(2, importButton, cancel).CenterHorizontalGroup(importButton, cancel)
	dlg.SetLayout(func() { layout.SetPosition(dlg.X1+2, dlg.Y1+2, dlg.X2-2, dlg.Y2-2) })
	busy := false
	cancel.OnClick = func() {
		if !busy {
			dlg.Close()
		}
	}
	importButton.OnClick = func() {
		if busy {
			return
		}
		source := edit.GetText()
		busy = true
		importButton.SetDisabled(true)
		edit.SetDisabled(true)
		cancel.SetDisabled(true)
		vtui.RunAsync(func(task *vtui.TaskContext) {
			path, err := Far3HistoryPath(source)
			var snapshot history.Far3History
			if err == nil {
				snapshot, err = history.ReadFar3History(task.Context, path)
			}
			var counts history.Far3ImportCounts
			if err == nil {
				counts = hp.MergeFar3History(snapshot)
				hp.SaveHistory(far3LocationHistoryID, []string{source})
				err = hp.Flush()
			}
			task.RunOnUI(func() {
				busy = false
				importButton.SetDisabled(false)
				edit.SetDisabled(false)
				cancel.SetDisabled(false)
				if err != nil {
					vtui.ShowMessage(i18n.Msg("History.Far3.Title"), err.Error(), []string{i18n.Msg("vtui.Ok")})
					return
				}
				dlg.Close()
				if onImported != nil {
					onImported()
				}
				vtui.ShowMessage(i18n.Msg("History.Far3.Title"), fmt.Sprintf(i18n.Msg("History.Far3.Result"),
					counts.Commands, counts.Files, counts.Folders, counts.Skipped), []string{i18n.Msg("vtui.Ok")})
			})
		})
	}
	dlg.SetFocusedItem(edit)
	vtui.FrameManager.Push(dlg)
	return dlg
}
