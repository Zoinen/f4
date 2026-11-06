package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var fileGroupStore filegroups.Store
var fileGroupRuntime *filegroups.Runtime
var fileGroupContribution filegroups.Contribution
var fileGroupActiveTheme = "generic"

// SetFileGroupIconTheme is called on the UI thread (or before UI startup).
// It changes context, never the draft document or the selected colour style.
func SetFileGroupIconTheme(id string) {
	if id == "" {
		id = "generic"
	}
	if fileGroupActiveTheme == id {
		return
	}
	fileGroupActiveTheme = id
	if fileGroupRuntime != nil {
		fileGroupRuntime.SetIconTheme(id)
	}
	if vtui.FrameManager == nil {
		return
	}
	for _, screen := range vtui.FrameManager.Screens {
		for _, frame := range screen.Frames {
			if center, ok := frame.(*settingsCenter); ok && !center.closed {
				center.fileGroupMatchResult = ""
				center.rebuildCategory()
			}
		}
	}
}

func fileGroupThemeName(id string) string {
	for _, theme := range fileGroupContribution.Themes {
		if theme.ID == id && theme.Name != "" {
			return theme.Name
		}
	}
	if id == "system" {
		return "Native"
	}
	return id
}

// ConfigureFileGroupCatalog accepts an optional startup contribution. It does
// not depend on a Qt transport or a particular catalog implementation.
func ConfigureFileGroupCatalog(data string) error {
	contribution, err := filegroups.ParseContribution([]byte(data))
	if err != nil {
		return err
	}
	fileGroupContribution = contribution
	return nil
}

// FileGroupThemeContributions returns detached frontend defaults for the store.
func FileGroupThemeContributions() []filegroups.ThemeSet {
	result := []filegroups.ThemeSet{}
	for _, set := range fileGroupContribution.ThemeSets {
		result = append(result, filegroups.ThemeSet{ID: set.ID, Fields: maps.Clone(set.Fields)})
	}
	return result
}

func FileGroupFactoryContributions() []filegroups.Group {
	result := make([]filegroups.Group, 0, len(fileGroupContribution.Groups))
	for _, group := range fileGroupContribution.Groups {
		result = append(result, group.Clone())
	}
	return result
}

// ConfigureFileGroups receives the application's domain runtime, never a Qt
// object. The Go Settings provider owns drafts and persistence for every UI.
func ConfigureFileGroups(store filegroups.Store, runtime *filegroups.Runtime) {
	fileGroupStore = store
	fileGroupRuntime = runtime
	if runtime != nil {
		runtime.SetIconTheme(fileGroupActiveTheme)
	}
}

func CommittedFileGroups() *filegroups.Snapshot {
	if fileGroupRuntime == nil || fileGroupStore.Path != filepath.Join(config.GetF4ConfigDir(), "highlight.ini") {
		return nil
	}
	return fileGroupRuntime.Committed()
}

type fileGroupsProvider struct{}

func groupField(key, label, description string, kind f4settings.Kind) f4settings.Field {
	f := recordField("group."+key, label, description, kind)
	f.Timing = "preview"
	return f
}

func groupChoice(key, label string, choices ...string) f4settings.Field {
	f := groupField(key, label, "Choose the matching behaviour for this group.", f4settings.ChoiceKind)
	for _, value := range choices {
		f.Choices = append(f.Choices, f4settings.Choice{Value: value, Label: f4settings.Text{English: value, Literal: true}})
	}
	return f
}

func (fileGroupsProvider) Catalog() f4settings.Catalog {
	var diagnostic []f4settings.Field
	if _, err := currentFileGroupStore().Begin(); err != nil {
		field := recordField("filegroups.diagnostic", "Configuration error", "The source file has been preserved. Correct it externally, then reload; no empty replacement will be saved.", f4settings.Multiline)
		field.Category = "filegroups"
		field.Group = "Configuration"
		field.Unavailable = err.Error()
		diagnostic = append(diagnostic, field)
	}
	fields := []f4settings.Field{
		groupField("Name", "Group name", "Also selects theme artwork: Archives matches archives.svg. Filename matching uses the conditions below.", f4settings.String),
		groupChoice("IconTheme", "Group scope", "", fileGroupActiveTheme),
		groupField("Enabled", "Enable group", "Disabled groups preserve their settings but contribute no features.", f4settings.Boolean),
		groupField("Mask", "Filename masks", "Far masks support comma/semicolon lists, exclusions with |, and /regular expressions/.", f4settings.String),
		groupChoice("MaskSyntax", "Mask syntax", "far", "legacy-glob"),
		groupChoice("Case", "Case matching", "insensitive", "sensitive", "platform"),
		groupField("IncludeAttributes", "Required attributes", "Directory, Hidden, Executable, ReadOnly, System, Archive, Symlink, Junction.", f4settings.String),
		groupField("ExcludeAttributes", "Excluded attributes", "The same attribute cannot be both required and excluded.", f4settings.String),
		groupField("SizeAbove", "Minimum bytes", "Inclusive lower limit; zero means no limit.", f4settings.Integer),
		groupField("SizeBelow", "Maximum bytes", "Inclusive upper limit; zero means no limit.", f4settings.Integer),
		groupChoice("DateType", "Timestamp", "Modified", "Created", "Accessed"),
		groupField("DateRelative", "Relative dates", "Use durations such as 48h, 30m or 2d instead of absolute timestamps.", f4settings.Boolean),
		groupField("DateAfter", "After", "YYYY-MM-DD HH:MM:SS or a duration for relative dates.", f4settings.String),
		groupField("DateBefore", "Before", "YYYY-MM-DD HH:MM:SS or a duration for relative dates.", f4settings.String),
		groupField("Highlight.Enabled", "Enable highlighting", "Only enabled highlighting participates in the colour cascade.", f4settings.Boolean),
		groupField("NormalColor", "Normal file colour", "foreground:<palette name or #RRGGBB> | background:<palette name or #RRGGBB>.", f4settings.Color),
		groupField("SelectedColor", "Selected file colour", "Independent colour expression for marked files.", f4settings.Color),
		groupField("CursorColor", "Cursor colour", "Independent colour expression for the focused file.", f4settings.Color),
		groupField("SelectedCursorColor", "Selected cursor colour", "Independent colour expression for a marked focused file.", f4settings.Color),
		groupField("Mark", "Filename marker", "Optional text or glyph before the filename.", f4settings.String),
		groupField("ContinueProcessing", "Continue highlighting", "Continue the existing colour cascade after this rule matches.", f4settings.Boolean),
		groupField("UseDefaults", "Override attribute defaults", "Preserve existing Directory/Hidden/Symlink/Junction override precedence.", f4settings.Boolean),
		groupField("Icons.Enabled", "Enable icons", "Use the theme artwork named after this group. Optional overrides are independent of colours and commands.", f4settings.Boolean),
		groupField("OverrideNativeIcon", "Override native/plugin icon", "Off by default. Native OS lookup is used only in the Native icon theme.", f4settings.Boolean),
		groupField("Icon.*", "Any-theme icon", "asset:<name>, file:<path>, native:<name>, or none. A theme-specific choice takes precedence within this group.", f4settings.String),
		groupField("Sort.Enabled", "Enable sort group", "Changes only panels with sort groups enabled.", f4settings.Boolean),
		groupField("Sort.Number", "Sort group number", "Unclassified files use 10000. Larger numbers place this group below them.", f4settings.Integer),
		groupField("Associations.Enabled", "Enable associations", "Commands remain committed-only until Apply; they are never executed by a preview.", f4settings.Boolean),
	}
	for i := range fields {
		if fields[i].ID == "group.Icon.*" {
			fields[i].IconTheme = "*"
		}
		if fields[i].ID == "group.Enabled" {
			fields[i].Default = "true"
		}
		if fields[i].ID == "group.Icons.Enabled" {
			fields[i].Default = "true"
		}
		if fields[i].ID == "group.Mask" {
			fields[i].Default = "*"
		}
	}
	// Every saved theme is surfaced, including themes this frontend cannot render.
	themes := map[string]bool{}
	for _, theme := range fileGroupContribution.Themes {
		themes[theme.ID] = true
	}
	if tx, err := currentFileGroupStore().Begin(); err == nil {
		for _, g := range tx.Groups() {
			for key := range g.Fields {
				if strings.HasPrefix(key, "Icon.") && key != "Icon.*" {
					themes[strings.TrimPrefix(key, "Icon.")] = true
				}
			}
		}
	}
	themeIDs := make([]string, 0, len(themes))
	for themeID := range themes {
		themeIDs = append(themeIDs, themeID)
	}
	slices.Sort(themeIDs)
	for _, themeID := range themeIDs {
		field := groupField("Icon."+themeID, "Icon: "+themeID, "Editable reference: choose a catalog asset or enter file:<path>, native:<name>, or none. Unavailable themes are retained.", f4settings.String)
		field.IconTheme = themeID
		for _, theme := range fileGroupContribution.Themes {
			if theme.ID != themeID {
				continue
			}
			field.Kind = f4settings.ChoiceKind
			field.AllowCustom = true
			field.ChoicePresentation = "dropdown"
			field.Choices = []f4settings.Choice{{Value: "", Label: f4settings.Text{English: "No explicit icon"}}}
			for _, asset := range theme.Assets {
				field.Choices = append(field.Choices, f4settings.Choice{Value: "asset:" + asset, Label: f4settings.Text{English: asset, Literal: true}})
			}
		}
		fields = append(fields, field)
	}
	groups := recordCollection("filegroups", "filegroups", "File groups", "Independent highlighting, icons, sorting and application associations. Changes preview live; Apply saves.", "group.Name", fields)
	commands := recordCollection("filegroups.commands", "filegroups", "Applications", "Ordered named command sets belonging to the selected file group.", "command.Name", nil)
	commands.Fields = append(commands.Fields,
		recordField("command.Group", "File group", "The stable identity of the owning file group.", f4settings.ChoiceKind),
		recordField("command.Name", "Application name", "Shown when several applications match.", f4settings.String))
	for _, slot := range filegroups.CommandNames {
		commands.Fields = append(commands.Fields, recordField("command."+slot+".Enabled", "Enable "+slot, "Commands execute only after Apply.", f4settings.Boolean), recordField("command."+slot, slot+" command", "Existing Far user-menu substitutions are preserved. Multiline commands round-trip without changing Windows paths.", f4settings.Multiline))
	}
	groups.Actions = []f4settings.RecordCommand{{ID: "filegroups.reset", Label: f4settings.Text{English: "Reset group"}, Description: f4settings.Text{English: "Restore standard fields where available, otherwise clear the group's features."}, Run: func(_ context.Context, r f4settings.Record) (map[string]string, error) {
		values := maps.Clone(r.Values)
		for _, f := range fields {
			values[f.ID] = ""
			if f.Kind == f4settings.Boolean {
				values[f.ID] = "false"
			}
			if f.Kind == f4settings.Integer {
				values[f.ID] = "0"
			}
		}
		values["group.Enabled"] = "true"
		values["group.Mask"] = "*"
		values["group.MaskSyntax"] = "far"
		values["group.Case"] = "insensitive"
		for _, g := range fileGroupStore.StandardGroups() {
			if g.ID == r.ID {
				for key, value := range g.Fields {
					values["group."+key] = value
				}
				break
			}
		}
		return values, nil
	}}}
	return f4settings.Catalog{ID: "core.filegroups", Fields: diagnostic, Categories: []f4settings.Category{{ID: "filegroups", Label: f4settings.Text{English: "File groups"}}}, Collections: []f4settings.Collection{groups, commands}}
}

func currentFileGroupStore() filegroups.Store {
	if fileGroupStore.Path == filepath.Join(config.GetF4ConfigDir(), "highlight.ini") {
		store := fileGroupStore
		store.ThemeDefaults = filegroups.MergeThemes(store.ThemeDefaults, fileGroupContribution.ThemeSets, nil)
		return store
	}
	return filegroups.Store{Path: filepath.Join(config.GetF4ConfigDir(), "highlight.ini"), AssociationsPath: panel.AssociationsFilePath()}
}

func groupsToRecords(groups []filegroups.Group) map[string][]f4settings.Record {
	rows := map[string][]f4settings.Record{"filegroups": {}, "filegroups.commands": {}}
	for _, g := range groups {
		values := map[string]string{}
		for key, value := range g.Fields {
			if strings.HasPrefix(key, "Palette.") {
				switch fileGroupFieldKey(key) {
				case "Highlight.Enabled", "DateRelative", "ContinueProcessing", "UseDefaults":
					value = strconv.FormatBool(value == "true" || value == "1")
				}
			}
			values["group."+key] = value
		}
		for _, key := range []string{"DateRelative", "ContinueProcessing", "UseDefaults", "OverrideNativeIcon", "Highlight.Enabled", "Icons.Enabled", "Sort.Enabled", "Associations.Enabled"} {
			if raw, exists := g.Fields[key]; exists {
				values["group."+key] = strconv.FormatBool(raw == "1" || raw == "true")
			}
		}
		if raw, exists := g.Fields["Enabled"]; exists {
			values["group.Enabled"] = strconv.FormatBool(raw != "0" && raw != "false")
		}
		if g.Commands != nil {
			values["group.CommandOrder"] = ""
		}
		rows["filegroups"] = append(rows["filegroups"], f4settings.Record{ID: g.ID, Values: values})
		for _, command := range g.Commands {
			if command.Extra["Disabled"] == "true" {
				continue
			}
			v := map[string]string{"command.Group": g.ID, "command.Name": command.Name}
			for k, value := range command.Extra {
				v["extra."+k] = value
			}
			for slot, name := range filegroups.CommandNames {
				v["command."+name] = command.Commands[slot]
				v["command."+name+".Enabled"] = strconv.FormatBool(command.Enabled[slot])
			}
			rows["filegroups.commands"] = append(rows["filegroups.commands"], f4settings.Record{ID: g.ID + "/" + command.ID, Values: v})
		}
	}
	return rows
}

func recordsToGroups(rows map[string][]f4settings.Record) ([]filegroups.Group, error) {
	groups := []filegroups.Group{}
	index := map[string]int{}
	for _, r := range rows["filegroups"] {
		g := filegroups.Group{ID: r.ID, Fields: map[string]string{}}
		if _, exists := r.Values["group.CommandOrder"]; exists {
			g.Commands = []filegroups.CommandSet{}
		}
		for key, value := range r.Values {
			if strings.HasPrefix(key, "group.") {
				g.Fields[strings.TrimPrefix(key, "group.")] = value
			}
		}
		index[g.ID] = len(groups)
		groups = append(groups, g)
	}
	for _, r := range rows["filegroups.commands"] {
		groupID := r.Values["command.Group"]
		i, ok := index[groupID]
		if !ok {
			return nil, fmt.Errorf("application %q has no existing file group", r.Values["command.Name"])
		}
		id := r.ID
		if _, suffix, ok := strings.Cut(id, "/"); ok {
			id = suffix
		}
		command := filegroups.CommandSet{ID: id, Name: r.Values["command.Name"], Extra: map[string]string{}}
		for key, value := range r.Values {
			if strings.HasPrefix(key, "extra.") {
				command.Extra[strings.TrimPrefix(key, "extra.")] = value
			}
		}
		for slot, name := range filegroups.CommandNames {
			command.Commands[slot] = r.Values["command."+name]
			command.Enabled[slot] = r.Values["command."+name+".Enabled"] == "true"
		}
		groups[i].Commands = append(groups[i].Commands, command)
	}
	return groups, nil
}

func (fileGroupsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	store := currentFileGroupStore()
	tx, err := store.Begin()
	if err != nil {
		// A broken highlighting document must not make unrelated Settings
		// inaccessible, and must never be replaced by an empty draft.
		draft := f4settings.NewDraft(map[string]string{"filegroups.diagnostic": err.Error()}, nil)
		draft.ValidateFunc = func(*f4settings.Draft) map[string]error { return map[string]error{"filegroups": err} }
		draft.CommitFunc = func(context.Context, *f4settings.Draft) f4settings.Result {
			return f4settings.Result{Errors: map[string]error{"filegroups": err}}
		}
		return draft, nil
	}
	draft := f4settings.NewDraft(nil, groupsToRecords(tx.Groups()))
	draft.Values[fileGroupColorThemeKey] = tx.ColorThemeID()
	colorRecords := []f4settings.Record{}
	for _, set := range tx.ColorThemes() {
		colorRecords = append(colorRecords, f4settings.Record{ID: set.ID, Values: maps.Clone(set.Fields)})
	}
	draft.Records["filegroups.colorthemes"] = colorRecords
	themeRecords := []f4settings.Record{}
	for _, set := range tx.Themes() {
		themeRecords = append(themeRecords, f4settings.Record{ID: set.ID, Values: maps.Clone(set.Fields)})
	}
	draft.Records["filegroups.themes"] = themeRecords
	// Initialize the dirty baseline after adding declarative theme records.
	draft.Accept(f4settings.Result{Applied: []string{"filegroups.themes", "filegroups.colorthemes", fileGroupColorThemeKey}})
	var scope *filegroups.PreviewSession
	runtime := fileGroupRuntime
	if CommittedFileGroups() != nil {
		scope = runtime.BeginPreview()
	}
	var timer *time.Timer
	var generation atomic.Uint64
	var closed atomic.Bool
	lastPreview := ""
	var lastPreviewError error
	stopPreview := func() {
		generation.Add(1)
		if timer != nil {
			timer.Stop()
			timer = nil
		}
	}
	draft.ValidateFunc = func(d *f4settings.Draft) map[string]error {
		_, err := recordsToGroups(d.Records)
		if err == nil {
			err = filegroups.ValidateThemes(fileGroupDraftThemes(d))
		}
		if err == nil {
			_, err = compileFileGroupDraft(d)
		}
		if err != nil {
			return map[string]error{"filegroups": err}
		}
		return nil
	}
	draft.PreviewFunc = func(d *f4settings.Draft) error {
		if scope == nil {
			return nil
		}
		groupsClean := !d.Dirty("filegroups") && !d.Dirty("filegroups.commands")
		themesClean := !d.Dirty("filegroups.themes") && !d.Dirty("filegroups.colorthemes") && !d.Dirty(fileGroupColorThemeKey)
		if groupsClean && themesClean {
			stopPreview()
			scope.Cancel()
			lastPreview, lastPreviewError = "", nil
			return nil
		}
		encoded, err := json.Marshal(struct {
			Records    map[string][]f4settings.Record
			ColorTheme string
		}{Records: d.Records, ColorTheme: fileGroupDraftColorThemeID(d)})
		if err != nil {
			return err
		}
		if string(encoded) == lastPreview {
			return lastPreviewError
		}
		stopPreview()
		lastPreview = string(encoded)
		snapshot, err := compileFileGroupDraft(d)
		lastPreviewError = err
		if err != nil {
			return err
		}
		if vtui.FrameManager == nil {
			scope.Publish(snapshot)
			return nil
		}
		frameManager := vtui.FrameManager
		current := generation.Load()
		timer = time.AfterFunc(60*time.Millisecond, func() {
			frameManager.PostTask(func() {
				if !closed.Load() && generation.Load() == current {
					scope.Publish(snapshot)
				}
			})
		})
		return nil
	}
	draft.CloseFunc = func() {
		closed.Store(true)
		stopPreview()
		if scope != nil {
			scope.Cancel()
		}
	}
	draft.CommitFunc = func(_ context.Context, d *f4settings.Draft) f4settings.Result {
		stopPreview()
		lastPreview, lastPreviewError = "", nil
		groups, err := recordsToGroups(d.Records)
		if err == nil {
			if len(fileGroupDraftColorThemes(d)) == 0 && fileGroupDraftColorThemeID(d) == "" {
				err = tx.Commit(groups, fileGroupDraftThemes(d))
			} else {
				err = tx.CommitColors(groups, fileGroupDraftThemes(d), fileGroupDraftColorThemes(d), fileGroupDraftColorThemeID(d))
			}
		}
		if err != nil {
			return f4settings.Result{Errors: map[string]error{"filegroups": err}}
		}
		if scope != nil {
			_ = runtime.ApplyColors(tx.Groups(), tx.Themes(), tx.ColorThemes(), tx.ColorThemeID())
		}
		return f4settings.Result{Applied: []string{"filegroups", "filegroups.commands", "filegroups.themes", "filegroups.colorthemes", fileGroupColorThemeKey}}
	}
	return draft, nil
}

// previewMatchText evaluates draft data only; it never dispatches a command.
func previewMatchText(groups []filegroups.Group, name string) (string, error) {
	return previewMatchEntry(groups, &vfs.VFSItem{Name: name}, false)
}

func previewMatchEntry(groups []filegroups.Group, item *vfs.VFSItem, known bool) (string, error) {
	s, err := filegroups.Compile(groups)
	if err != nil {
		return "", err
	}
	s = s.ForIconTheme(fileGroupActiveTheme)
	parts := []string{}
	for _, facet := range []string{"Highlight", "Icons", "Sort", "Associations"} {
		labels := []string{}
		for _, g := range s.Matching(item, known, facet, time.Now()) {
			labels = append(labels, g.Label())
			if facet == "Associations" {
				for _, command := range g.Commands {
					slots := []string{}
					for slot, enabled := range command.Enabled {
						if enabled && command.Commands[slot] != "" {
							slots = append(slots, filegroups.CommandNames[slot])
						}
					}
					if len(slots) > 0 {
						labels = append(labels, command.Name+" ("+strings.Join(slots, ", ")+")")
					}
				}
			}
		}
		parts = append(parts, facet+": "+strings.Join(labels, ", "))
	}
	highlighter := &theme.FileHighlighter{}
	if err := highlighter.LoadFileGroups(s); err != nil {
		return "", err
	}
	_, style := highlighter.SemanticStyle(item, known)
	parts = append(parts, fmt.Sprintf("Effective normal: %s / %s; marker: %s", style.Normal.Foreground, style.Normal.Background, style.Marker))
	if matches := s.Matching(item, known, "Sort", time.Now()); len(matches) > 0 {
		parts = append(parts, "Effective sort group: "+matches[0].Fields["Sort.Number"])
	} else {
		parts = append(parts, "Effective sort group: 10000")
	}
	for theme, choice := range style.IconChoices {
		parts = append(parts, fmt.Sprintf("Icon %s: %s:%s (%s)", theme, choice.Kind, choice.Value, choice.GroupID))
	}
	return strings.Join(parts, "\n"), nil
}
