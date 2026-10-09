package settings

import (
	"context"
	"strconv"
	"strings"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/sdk/f4settings"
)

// The Tools page of the drive menu's settings window (f4#1148): one check box
// per tool the drive menu can list, ticked when the tool is shown. It lives in
// this window only, not in the Drive chooser category of the main Settings,
// which is left as it was. Apply writes drive-tools-visibility.txt, the file the
// drive menu already reads.

const driveToolFieldPrefix = "drivetool."

type driveToolsProvider struct {
	drives []sysinfo.DriveEntry
}

// driveToolName is the tool as the old window of the tools named it: the name
// without its accelerator mark.
func driveToolName(drv sysinfo.DriveEntry) string {
	return strings.TrimSpace(strings.ReplaceAll(drv.Name, "&", ""))
}

func (p driveToolsProvider) Catalog() f4settings.Catalog {
	cat := f4settings.Catalog{ID: "drive-tools", Categories: driveChooserPages}
	for i, drv := range p.drives {
		cat.Fields = append(cat.Fields, f4settings.Field{
			ID:          driveToolField(i),
			Category:    driveToolsPage,
			Label:       f4settings.Text{English: driveToolName(drv), Literal: true},
			Description: f4settings.Text{Key: "SettingsCenter.DriveTools.Show", English: "Show this tool in the drive menu."},
			Kind:        f4settings.Boolean,
			Default:     "true",
		})
	}
	return cat
}

func driveToolField(i int) string { return driveToolFieldPrefix + strconv.Itoa(i) }

func (p driveToolsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	disabled, err := panel.LoadDisabledDriveTools(panel.DriveToolsVisibilityFilePath())
	if err != nil {
		return nil, err
	}
	hidden := map[string]bool{}
	for _, name := range disabled {
		hidden[name] = true
	}
	values := map[string]string{}
	for i, drv := range p.drives {
		values[driveToolField(i)] = boolText(!hidden[drv.Name])
	}
	d := f4settings.NewDraft(values, nil)
	d.CommitFunc = func(ctx context.Context, d *f4settings.Draft) f4settings.Result {
		result := f4settings.Result{Errors: map[string]error{}}
		changed := false
		shown := make([]bool, len(p.drives))
		for i := range p.drives {
			id := driveToolField(i)
			shown[i] = d.Values[id] != "false"
			if d.Dirty(id) {
				changed = true
			}
		}
		if !changed {
			return result
		}
		// The file is read again: it may have changed since the window opened.
		current, err := panel.LoadDisabledDriveTools(panel.DriveToolsVisibilityFilePath())
		if err != nil {
			result.Errors[driveToolsPage] = err
			return result
		}
		if err := panel.SaveDisabledDriveTools(panel.DriveToolsVisibilityFilePath(), panel.HiddenDriveToolsAfter(current, p.drives, shown)); err != nil {
			result.Errors[driveToolsPage] = err
			return result
		}
		for i := range p.drives {
			if id := driveToolField(i); d.Dirty(id) {
				result.Applied = append(result.Applied, id)
			}
		}
		return result
	}
	return d, nil
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// newDriveToolsSession is the session of the Tools page for the tools the
// drive menu knows now.
func newDriveToolsSession() (*settingsSession, error) {
	p := driveToolsProvider{drives: sysinfo.DriveRegistrySnapshot()}
	d, err := p.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	return &settingsSession{provider: p, catalog: settingsCatalogChoiceHelp(p.Catalog()), draft: d}, nil
}
