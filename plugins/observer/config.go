package observer

// This file is f4#1563's observer.ini slice: items 1 and 2 of the backlog
// status/1563.md (accounting repository) left after part 8 --
// "observer.ini/observer_user.ini -- конфиг, порядок модулей, фильтры" and
// "выбор МЕЖДУ несколькими модулями Observer" -- turn out to be one and the
// same mechanism, not two: Observer's own host
// (plugin/ModulesController.cpp, OpenStorageFile) tries [Modules] entries in
// file order and the first one whose OpenStorage does not answer
// SORInvalidFile wins. There is no separate "priority" field or tie-break
// rule anywhere in upstream to invent -- file order already is the
// priority -- so giving Provider (provider.go) an ordered slice of
// moduleEntry instead of one hardcoded module answers both items at once.
//
// Neither this file's format nor its merge order is new: both were already
// decided in the ticket's first comment ("Конфиг читается в формате
// observer.ini и observer_user.ini: порядок из [Modules], фильтры из
// [Filters], секции модулей передаются как строка Settings. Так свою
// настройку можно перенести из Far.") and confirmed straight from upstream
// source while writing this part -- plugin/Config.cpp parses each file with
// nothing more exotic than Win32's GetPrivateProfileSection (a plain INI
// reader), and Observer-Far3.cpp's LoadSettings calls cfg.ParseFile twice,
// observer.ini then observer_user.ini, into the same Config. There is no
// ready-made ini-reading library bundled with the module ABI itself (API v6,
// ModuleDef.h, ACTUAL_API_VERSION 6) to just point at a path: the format is
// entirely the *host's* responsibility, upstream's host included, which is
// why this file exists instead of a one-line config option.
//
// What copying a real Far Observer.ini/observer_user.ini pair here gets a
// user is the [Modules]/[Filters]/per-module-section *shape*, not a working
// drop-in: [Modules] values there name a Windows DLL (modules\isoimg.so,
// really a PE DLL despite the extension --see doc.go and the ticket's first
// comment) where this package needs a wasm32 file this host can actually
// run (isoimg.wasm). A row whose FileName does not resolve under
// Provider.modulesDir is silently skipped by moduleBytes returning nil, the
// same "not installed yet" reading provider.go already gave a single
// hardcoded module before this part.
import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// observerConfigFileName and observerUserConfigFileName are read from the
// observer directory Provider.modulesDir sits under (filepath.Dir of it --
// NewPlugin in plugin.go builds modulesDir as
// filepath.Join(configDir, "observer", "modules"), so the ini pair lives at
// configDir/observer/{observer,observer_user}.ini, next to modules/ rather
// than inside it), in that order -- observer.ini first, observer_user.ini
// second -- exactly Observer-Far3.cpp's own LoadSettings order, so a later
// file's value for an existing key overwrites the earlier one and a new key
// is appended after whatever the earlier file already had (orderedSection.set).
const (
	observerConfigFileName     = "observer.ini"
	observerUserConfigFileName = "observer_user.ini"
)

// moduleEntry is one resolved row of the [Modules] section. Name is the
// section key -- also the lookup key into [Filters] and into a
// same-named section holding the module's own settings. FileName is the
// value, a plain file name resolved only against Provider.modulesDir, never
// treated as a path in its own right; "-" or an empty Name/FileName is
// skipped during loadModuleEntries the same way
// ModulesController::Init skips an explicitly disabled or malformed row.
//
// Filter is the raw semicolon-separated glob straight out of [Filters]:
// internal/filemask.Match already treats ';' the same as ',' (see its own
// doc comment), so nothing needs translating. An empty Filter means CanOpen
// never matches this entry through a file name alone -- the same reading
// upstream's own stock observer.ini gives ISO/UDF/MBOX/MIME/VDISK, which
// rely on a signature check or an explicit "open with" command instead
// (see the ticket's first comment); Provider does not implement either yet,
// so an entry left with an empty filter is reachable only once it does.
//
// Settings is the module's own section (a section named exactly Name, if
// one exists), already rendered into the NUL-joined wire format
// LoadSubModule expects -- see buildSettingsString.
type moduleEntry struct {
	Name     string
	FileName string
	Filter   string
	Settings string
}

// defaultModuleEntries is what Provider falls back to when neither
// observer.ini nor observer_user.ini exists at all under the observer
// directory: the single isoimg/"*.iso" row parts 5, 7 and 8 already shipped
// as a hardcoded default, unchanged -- a profile that has not opted into a
// config file sees no behaviour change from this part.
func defaultModuleEntries() []moduleEntry {
	return []moduleEntry{{Name: "ISO", FileName: isoimgModuleFileName, Filter: "*.iso"}}
}

// loadModuleEntries reads observerDir/observer.ini and
// observerDir/observer_user.ini into one merged config and returns
// [Modules] as an ordered slice, in file order -- order is the whole
// selection mechanism between several modules (see this file's own
// package-level comment above and Provider.CanOpen/Open in provider.go,
// which try entries in the order returned here and stop at the first one
// that recognizes the file).
//
// Neither file existing at all is different from a file existing with an
// empty or missing [Modules] section: the former is defaultModuleEntries,
// the latter faithfully returns no entries at all, the same way
// ModulesController::Init does ("if (!mModulesList) return 0;") -- an
// explicit but empty [Modules] section is a deliberate "no Observer formats
// at all" a user wrote by hand, not a case for this package to second-guess
// back into the default.
func loadModuleEntries(observerDir string) []moduleEntry {
	cfg := newOrderedINI()
	haveBase := cfg.mergeFile(filepath.Join(observerDir, observerConfigFileName))
	haveUser := cfg.mergeFile(filepath.Join(observerDir, observerUserConfigFileName))
	if !haveBase && !haveUser {
		return defaultModuleEntries()
	}

	modules := cfg.section("Modules")
	if modules == nil {
		return nil
	}
	filters := cfg.section("Filters")

	entries := make([]moduleEntry, 0, len(modules.keys))
	for _, name := range modules.keys {
		fileName := modules.vals[name]
		if name == "" || fileName == "" || fileName == "-" {
			continue
		}
		var filter string
		if filters != nil {
			filter = filters.vals[name]
		}
		entries = append(entries, moduleEntry{
			Name:     name,
			FileName: fileName,
			Filter:   filter,
			Settings: buildSettingsString(cfg.section(name)),
		})
	}
	return entries
}

// buildSettingsString renders a module's own ini section (named exactly the
// module's [Modules] key) as the NUL-separated "Key=Value" sequence
// Observer's own depends/modulecrt/OptionsParser.cpp
// (OptionsList::ParseLines) and plugin/Config.cpp (ConfigSection::GetAll)
// both use for ModuleLoadParameters.Settings -- one entry per line, in file
// order, each followed by its own NUL. wchar.go's encodeWChars appends one
// further terminating NUL unit on top of whatever this function returns, so
// together they reproduce GetAll()'s "entry\0" * N + "\0" double-NUL
// termination exactly, without this function adding a second trailing NUL
// of its own. A module with no matching section (sec is nil) or an empty
// one yields "", the same value every part before this one already
// hardcoded for LoadSubModule.
func buildSettingsString(sec *orderedSection) string {
	if sec == nil || len(sec.keys) == 0 {
		return ""
	}
	var b strings.Builder
	for _, key := range sec.keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(sec.vals[key])
		b.WriteByte(0)
	}
	return b.String()
}

// orderedSection is one INI section. keys preserves file order -- the whole
// point of this hand-rolled parser instead of internal/ini.File, whose
// map[string]string sections (internal/ini/ini.go) cannot answer "which
// [Modules] entry came first" at all, and file order is exactly what
// decides module priority here (see this file's package comment). Re-setting
// an existing key updates its value in place without moving its position,
// the same as Observer's own ConfigSection::AddItem ("Try to find and
// replace existing value" before falling back to appending a new one).
type orderedSection struct {
	keys []string
	vals map[string]string
}

func newOrderedSection() *orderedSection {
	return &orderedSection{vals: make(map[string]string)}
}

func (s *orderedSection) set(key, val string) {
	if _, ok := s.vals[key]; !ok {
		s.keys = append(s.keys, key)
	}
	s.vals[key] = val
}

// orderedINI holds every section parsed so far, keyed by section name.
// mergeFile is called once per file (observer.ini, then observer_user.ini)
// against the same instance -- see loadModuleEntries -- which is exactly
// what Observer-Far3.cpp's LoadSettings does by calling
// cfg.ParseFile(observer.ini) and cfg.ParseFile(observer_user.ini) on one
// Config in that order.
type orderedINI struct {
	sections map[string]*orderedSection
}

func newOrderedINI() *orderedINI {
	return &orderedINI{sections: make(map[string]*orderedSection)}
}

func (ini *orderedINI) section(name string) *orderedSection {
	return ini.sections[name]
}

// mergeFile parses path into ini, returning false only when the file does
// not exist (or cannot be opened) at all -- loadModuleEntries only needs to
// tell "neither file present" apart from every other case, the same
// "missing means not configured yet" reading the rest of this package
// already gives a missing .wasm module file.
//
// Parsing itself mirrors Observer's own plugin/Config.cpp: a line whose
// trimmed form is "[Name]" starts section Name; a line starting with ';'
// (after the same trim -- Config.cpp only ever skips leading spaces and
// tabs before checking for ';', which TrimSpace already covers) is a
// comment; anything else with a top-level '=' becomes one Key=Value pair in
// the current section, outside any section discarded the same way
// GetPrivateProfileSection could never return it in the first place.
func (ini *orderedINI) mergeFile(path string) bool {
	f, err := os.Open(path) //nolint:gosec // G304: path is built from this Provider's own configured observer directory (filepath.Dir of modulesDir), not from panel/user path input.
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	var cur *orderedSection
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			name := line[1 : len(line)-1]
			cur = ini.sections[name]
			if cur == nil {
				cur = newOrderedSection()
				ini.sections[name] = cur
			}
		case strings.HasPrefix(line, ";"):
			// comment, nothing to do
		case cur != nil:
			if eq := strings.IndexByte(line, '='); eq >= 0 {
				cur.set(strings.TrimSpace(line[:eq]), strings.TrimSpace(line[eq+1:]))
			}
		}
	}
	return true
}
