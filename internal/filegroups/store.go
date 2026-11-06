package filegroups

import (
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

//go:embed *.ini
var factoryFiles embed.FS

func FactoryGroups() ([]Group, error) {
	data, err := factoryFiles.ReadFile("highlight.default.ini")
	if err != nil {
		return nil, err
	}
	d, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return d.Groups()
}

var storeWriteMu sync.Mutex
var ErrConflict = errors.New("file groups changed outside this editor; reload or preserve your draft")

// Store has no renderer dependencies. Defaults seed a new working set and remain
// available for explicit reset. Saved snapshots are independent editable lists.
type Store struct {
	Path, AssociationsPath string
	Defaults               []Group
	ThemeDefaults          []ThemeSet
	ColorDefaults          []ColorTheme
	ColorStyle             string
	Write                  func(string, []byte, os.FileMode) error
}

type Transaction struct {
	store                         Store
	document                      *Document
	baseline, associationBaseline [32]byte
	original                      []byte
	migrated                      bool
	iconScopesMigrated            bool
	catalogGroupsMigrated         bool
	palettesMigrated              bool
	legacyAssociations            bool
	complete                      bool
	groups                        []Group
	colors                        []ColorTheme
	colorTheme                    string
	hasColors, colorsMigrated     bool
	currentFoldersMigrated        bool
}

func readOptional(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func (s Store) Begin() (*Transaction, error) {
	data, err := readOptional(s.Path)
	if err != nil {
		return nil, fmt.Errorf("read highlighting: %w", err)
	}
	doc, err := Parse(data)
	if err != nil {
		return nil, err
	}
	mode := doc.Section("FileGroups")["Mode"]
	if mode != "" && mode != "snapshot" {
		return nil, fmt.Errorf("unsupported file groups mode %q", mode)
	}
	if mode == "snapshot" && doc.version != 3 {
		return nil, fmt.Errorf("snapshot mode requires file groups version 3")
	}
	t := &Transaction{store: s, document: doc, baseline: sha256.Sum256(data), original: data,
		migrated: doc.version != 3, legacyAssociations: !doc.Unified(), complete: mode == "snapshot"}
	if doc.Unified() {
		t.groups, err = doc.Groups()
	} else {
		associationData, readErr := readOptional(s.AssociationsPath)
		if readErr != nil {
			return nil, readErr
		}
		assoc, parseErr := Parse(associationData)
		if parseErr != nil {
			return nil, parseErr
		}
		t.associationBaseline = sha256.Sum256(associationData)
		t.groups = ImportLegacy(doc, assoc)
	}
	if err != nil {
		return nil, err
	}
	if doc.version == 2 {
		t.groups, err = migrateThemeScopes(s.Defaults, t.groups)
		if err != nil {
			return nil, err
		}
	}
	oldGroupCount := len(t.groups)
	t.groups, t.catalogGroupsMigrated, err = migrateCatalogGroups(s.Defaults, t.groups)
	if err != nil {
		return nil, err
	}
	if t.catalogGroupsMigrated {
		vtui.DebugLog("FILEGROUPS: shared catalog migration overrides=%d -> %d (in memory)", oldGroupCount, len(t.groups))
	}
	t.groups, t.palettesMigrated, err = migratePaletteGroups(s.Defaults, t.groups)
	if err != nil {
		return nil, err
	}
	if t.catalogGroupsMigrated || t.palettesMigrated {
		t.groups = discardMigratedFactoryOrder(s.Defaults, t.groups)
	}
	t.iconScopesMigrated = migrateLegacyIconScopes(t.groups, s.ThemeDefaults)
	t.migrated = t.migrated || t.iconScopesMigrated || t.catalogGroupsMigrated || t.palettesMigrated
	colors, err := doc.ColorThemes()
	if err != nil {
		return nil, err
	}
	selected, colorMetadata := doc.Section("FileGroups")["ColorTheme"]
	t.hasColors = colorMetadata || len(colors) > 0 || s.ColorDefaults != nil
	t.colors, t.colorTheme = cloneColorThemes(colors), selected
	if !colorMetadata {
		t.colors = mergeColorThemes(s.ColorDefaults, colors)
	}
	if s.ColorDefaults != nil && !colorMetadata && len(colors) == 0 {
		var current ColorTheme
		t.groups, current = captureGroupColors(t.Groups(), s.ColorStyle)
		t.complete = true
		found := false
		for i := range t.colors {
			if t.colors[i].ID == current.ID {
				t.colors[i] = current
				found = true
				break
			}
		}
		if !found {
			t.colors = append(t.colors, current)
		}
		t.colorTheme, t.colorsMigrated, t.migrated = "current", true, true
	}
	t.colors, t.currentFoldersMigrated = migrateCurrentFolders(s.ColorDefaults, t.colors)
	t.migrated = t.migrated || t.currentFoldersMigrated
	if err := ValidateColorThemes(t.colors); err != nil {
		return nil, err
	}
	if t.colorTheme != "" {
		if err := validID(t.colorTheme); err != nil {
			return nil, err
		}
	}
	vtui.DebugLog("FILEGROUPS: begin groups=%d migrated=%v", len(t.groups), t.migrated)
	return t, nil
}

func (t *Transaction) Groups() []Group {
	if !t.complete {
		return Merge(t.store.Defaults, t.groups)
	}
	groups := make([]Group, 0, len(t.groups))
	for _, group := range t.groups {
		copy := group.Clone()
		copy.Commands = mergeCommands(nil, copy.Commands)
		groups = append(groups, copy)
	}
	return groups
}
func (t *Transaction) Overrides() []Group {
	result := []Group{}
	for _, g := range t.groups {
		result = append(result, g.Clone())
	}
	return result
}
func (t *Transaction) Migrated() bool { return t.migrated }

func (t *Transaction) ColorThemes() []ColorTheme { return cloneColorThemes(t.colors) }
func (t *Transaction) ColorThemeID() string      { return t.colorTheme }

func (t *Transaction) Themes() []ThemeSet {
	overrides, _ := t.document.Themes()
	return MergeThemes(append(FactoryThemes(), t.store.ThemeDefaults...), overrides, t.Groups())
}

// Commit validates before touching disk and updates its baseline only after
// publication succeeds. A failed Apply remains retryable and keeps the draft.
func (t *Transaction) Commit(groups []Group, themeSets ...[]ThemeSet) error {
	return t.commit(groups, t.colors, t.colorTheme, t.hasColors, themeSets...)
}

func (t *Transaction) CommitColors(groups []Group, iconThemes []ThemeSet, colorThemes []ColorTheme, selected string) error {
	groups = NormalizeColorGroups(groups, t.store.ColorStyle)
	return t.commit(groups, colorThemes, selected, true, iconThemes)
}

func (t *Transaction) commit(groups []Group, colors []ColorTheme, selected string, hasColors bool, themeSets ...[]ThemeSet) error {
	if _, err := Compile(groups); err != nil {
		return err
	}
	themes, _ := t.document.Themes()
	if len(themeSets) > 0 {
		if err := ValidateThemes(themeSets[0]); err != nil {
			return err
		}
		themes = themeDifferences(append(FactoryThemes(), t.store.ThemeDefaults...), themeSets[0])
	}
	data, err := t.document.encodeSnapshot(groups, themes)
	if hasColors {
		data, err = t.document.encodeSnapshotColors(groups, themes, colors, selected)
	}
	if err != nil {
		return err
	}
	data, err = refreshDocumentHelp(data)
	if err != nil {
		vtui.DebugLog("[FIX:file-groups] Apply help refresh failed: %v", err)
		return err
	}
	storeWriteMu.Lock()
	defer storeWriteMu.Unlock()
	current, err := readOptional(t.store.Path)
	if err != nil {
		return err
	}
	if sha256.Sum256(current) != t.baseline {
		return ErrConflict
	}
	if t.legacyAssociations {
		associations, err := readOptional(t.store.AssociationsPath)
		if err != nil {
			return err
		}
		if sha256.Sum256(associations) != t.associationBaseline {
			return ErrConflict
		}
	}
	createdPaletteBackup := ""
	if t.migrated || !t.complete {
		if len(t.original) > 0 {
			backup := t.store.Path + ".pre-filegroups"
			if !t.migrated {
				backup = t.store.Path + ".pre-filegroups-snapshot"
			}
			if t.document.version == 3 && t.iconScopesMigrated {
				backup = t.store.Path + ".pre-filegroups-icon-scopes"
			}
			if t.document.version == 3 && t.catalogGroupsMigrated {
				backup = t.store.Path + ".pre-filegroups-group-icons"
			}
			if t.document.version == 2 {
				backup = t.store.Path + ".pre-filegroups-v3"
			}
			if t.palettesMigrated {
				backup = t.store.Path + ".pre-filegroups-palettes"
			}
			if t.colorsMigrated {
				backup = t.store.Path + ".pre-filegroups-colors"
			}
			if t.currentFoldersMigrated {
				backup = t.store.Path + ".pre-filegroups-current-folders"
			}
			old, err := readOptional(backup)
			if err != nil {
				return err
			}
			if len(old) > 0 && !reflect.DeepEqual(old, t.original) {
				return fmt.Errorf("migration backup already exists with different content: %s", backup)
			}
			if len(old) == 0 {
				if t.palettesMigrated || t.colorsMigrated || t.currentFoldersMigrated {
					// An existing empty backup is still user-owned. Do not
					// overwrite it or remove it if publication later fails.
					if _, statErr := os.Stat(backup); statErr == nil {
						return fmt.Errorf("migration backup already exists with different content: %s", backup)
					} else if !errors.Is(statErr, os.ErrNotExist) {
						return statErr
					}
				}
				if err := config.WriteUserFileAtomically(backup, t.original, 0600); err != nil {
					return fmt.Errorf("back up highlighting: %w", err)
				}
				if t.palettesMigrated || t.colorsMigrated || t.currentFoldersMigrated {
					createdPaletteBackup = backup
				}
			}
		}
	}
	writer := t.store.Write
	if writer == nil {
		writer = config.WriteUserFileAtomically
	}
	if err := writer(t.store.Path, data, 0600); err != nil {
		vtui.DebugLog("[FIX:file-groups] publish with refreshed help failed: %v", err)
		if createdPaletteBackup != "" {
			if removeErr := os.Remove(createdPaletteBackup); removeErr != nil {
				return errors.Join(err, fmt.Errorf("remove uncommitted palette backup: %w", removeErr))
			}
		}
		return err
	}
	t.document, err = Parse(data)
	if err != nil {
		return err
	}
	t.baseline = sha256.Sum256(data)
	t.original = data
	t.groups, err = t.document.Groups()
	if err != nil {
		return err
	}
	t.complete = true
	t.colors, t.colorTheme, t.hasColors = cloneColorThemes(colors), selected, hasColors
	t.colorsMigrated = false
	t.currentFoldersMigrated = false
	t.migrated = false
	t.palettesMigrated = false
	t.legacyAssociations = false
	vtui.DebugLog("[FIX:file-groups] committed working set groups=%d mode=snapshot", len(groups))
	vtui.DebugLog("[FIX:file-groups] Apply published current help with user comments preserved")
	return nil
}

// Differences stores only overrides, including tombstones for removed defaults.
func Differences(defaults, groups []Group) []Group {
	base := map[string]Group{}
	for _, g := range defaults {
		base[g.ID] = g
	}
	result := []Group{}
	present := map[string]bool{}
	for _, g := range groups {
		present[g.ID] = true
		previous, exists := base[g.ID]
		if !exists {
			result = append(result, g.Clone())
			continue
		}
		patch := Group{ID: g.ID, Fields: map[string]string{}}
		for key, value := range g.Fields {
			old, ok := previous.Fields[key]
			if isBooleanField(key) {
				defaultValue := paletteFieldKey(key) == "Enabled"
				if booleanValue(old, defaultValue) == booleanValue(value, defaultValue) {
					continue
				}
			}
			if !ok || old != value {
				patch.Fields[key] = value
			}
		}
		if !(len(g.Commands) == 0 && len(previous.Commands) == 0) && !reflect.DeepEqual(g.Commands, previous.Commands) {
			patch.Commands = commandDifferences(previous.Commands, g.Clone().Commands)
			if patch.Commands == nil {
				patch.Commands = []CommandSet{}
			}
		}
		// An identity-only patch retains explicit user order without freezing values.
		result = append(result, patch)
	}
	for _, g := range defaults {
		if !present[g.ID] {
			result = append(result, Group{ID: g.ID, Fields: map[string]string{"Enabled": "false"}})
		}
	}
	return result
}

func isBooleanField(key string) bool {
	key = paletteFieldKey(key)
	return key == "Enabled" || strings.HasSuffix(key, ".Enabled") || key == "OverrideNativeIcon" || key == "DateRelative" || key == "UseDefaults" || key == "ContinueProcessing"
}

func paletteFieldKey(key string) string {
	if rest, palette := strings.CutPrefix(key, "Palette."); palette {
		if _, field, ok := strings.Cut(rest, "."); ok {
			return field
		}
	}
	return key
}

func booleanValue(raw string, fallback bool) bool {
	if raw == "" {
		return fallback
	}
	return raw == "true" || raw == "1"
}

func sortNumericSections(names []string, prefix string) {
	sort.SliceStable(names, func(i, j int) bool {
		a, _ := strconv.Atoi(strings.TrimPrefix(strings.ToLower(names[i]), prefix))
		b, _ := strconv.Atoi(strings.TrimPrefix(strings.ToLower(names[j]), prefix))
		return a < b
	})
}

// OverlayFields returns an independent field map for editor reset operations.
func OverlayFields(base, patch map[string]string) map[string]string {
	result := maps.Clone(base)
	if result == nil {
		result = map[string]string{}
	}
	maps.Copy(result, patch)
	return result
}
