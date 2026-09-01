package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type AttrFlags uint32

const (
	AttrDirectory AttrFlags = 1 << iota
	AttrHidden
	AttrExecutable
	AttrReadOnly
	AttrSystem
	AttrArchive
)

type DateType int

const (
	DateModified DateType = iota
	DateCreated
	DateAccessed
)

type HighlightRule struct {
	RuleID            string
	Name              string
	Masks             []string
	AttrSet           AttrFlags
	AttrClear         AttrFlags
	IgnoreCase        bool
	NormalStr         string
	SelectedStr       string
	CursorStr         string
	SelectedCursorStr string
	Mark              string
	IconURL           string

	// Фильтрация по размеру (0 означает, что лимит не задан)
	SizeAbove int64
	SizeBelow int64

	// Фильтрация по датам
	DateType      DateType
	DateAfter     time.Time
	DateBefore    time.Time
	DateAfterDur  time.Duration
	DateBeforeDur time.Duration
	DateRelative  bool

	// Каскадная обработка (Continue Processing)
	ContinueProcessing bool

	// maskIndex is prepared when the complete rule list is combined. It is an
	// immutable acceleration structure for the common exact/suffix masks; the
	// original filepath.Match fallback remains available for arbitrary globs.
	maskIndex *highlightMaskIndex
}

type highlightMaskIndex struct {
	all      bool
	exact    map[string]struct{}
	suffix   map[string]struct{}
	fallback []string
	masks    []string
}

func compileHighlightMaskIndex(rule HighlightRule) *highlightMaskIndex {
	index := &highlightMaskIndex{}
	if len(rule.Masks) == 0 {
		index.all = true
		return index
	}
	for _, rawMask := range rule.Masks {
		mask := rawMask
		if rule.IgnoreCase {
			mask = strings.ToLower(mask)
		}
		index.masks = append(index.masks, mask)
		switch {
		case mask == "*":
			index.all = true
		case !strings.ContainsAny(mask, "*?[]\\/"):
			if index.exact == nil {
				index.exact = make(map[string]struct{})
			}
			index.exact[mask] = struct{}{}
		case strings.HasPrefix(mask, "*.") &&
			!strings.ContainsAny(mask[2:], "*?[]\\/"):
			if index.suffix == nil {
				index.suffix = make(map[string]struct{})
			}
			index.suffix[mask[1:]] = struct{}{}
		default:
			index.fallback = append(index.fallback, mask)
		}
	}
	return index
}

func (index *highlightMaskIndex) matches(name string, ignoreCase bool) bool {
	if index == nil {
		return false
	}
	lookup := name
	if ignoreCase {
		lookup = strings.ToLower(lookup)
	}
	if len(index.masks) == 0 {
		return true
	}
	if strings.ContainsAny(lookup, "\\/") {
		for _, mask := range index.masks {
			matched, err := filepath.Match(mask, lookup)
			if err == nil && matched {
				return true
			}
		}
		return false
	}
	if index.all {
		// filepath.Match treats path separators as structural even for "*".
		// VFS entry names normally cannot contain them, but retain that behavior
		// for direct HighlightRule callers as well.
		if !strings.ContainsAny(lookup, "\\/") {
			return true
		}
	}
	if _, ok := index.exact[lookup]; ok {
		return true
	}
	for dot := strings.IndexByte(lookup, '.'); dot >= 0; {
		if _, ok := index.suffix[lookup[dot:]]; ok {
			return true
		}
		next := strings.IndexByte(lookup[dot+1:], '.')
		if next < 0 {
			break
		}
		dot += next + 1
	}
	for _, mask := range index.fallback {
		matched, err := filepath.Match(mask, lookup)
		if err == nil && matched {
			return true
		}
	}
	return false
}

type FileHighlighter struct {
	UserRules  []HighlightRule
	ThemeRules []HighlightRule
	Rules      []HighlightRule
	Revision   int64

	matchCacheMu       sync.RWMutex
	matchCacheRevision int64
	matchCache         map[highlightMatchCacheKey][]int

	semanticStyleCacheMu       sync.RWMutex
	semanticStyleCacheRevision int64
	semanticStyleCache         map[string]semanticStyleCacheValue
}

const maxHighlightMatchCacheEntries = 8192

const maxSemanticStyleCacheEntries = 8192

type semanticStyleCacheValue struct {
	id    string
	style extui.HighlightStyleModel
}

type highlightMatchCacheKey struct {
	Name          string
	Size          int64
	MTime         int64
	ATime         int64
	CTime         int64
	UnixMode      uint32
	WinAttrs      uint32
	IsDir         bool
	IsHidden      bool
	IsExecutable  bool
	MetadataKnown bool
}

var GlobalFileHighlighter *FileHighlighter

func init() {
	GlobalFileHighlighter = &FileHighlighter{}
}

func (fh *FileHighlighter) LoadFromIni(ini *IniFile) {
	fh.LoadUserRules(ini)
}

func (fh *FileHighlighter) LoadUserRules(ini *IniFile) {
	fh.LoadFromIniAt(ini, "")
}

// LoadFromIniAt loads user highlight rules and resolves relative file: icon
// URLs against baseDir. Theme and user rules continue to share the upstream
// priority/composition path.
func (fh *FileHighlighter) LoadFromIniAt(ini *IniFile, baseDir string) {
	fh.UserRules = parseHighlightRulesAt(ini, baseDir)
	fh.CombineRules()
}

func (fh *FileHighlighter) LoadThemeRules(ini *IniFile) {
	fh.ThemeRules = parseHighlightRules(ini)
	fh.CombineRules()
}

func (fh *FileHighlighter) CombineRules() {
	fh.Rules = nil
	if AppConfig.HighlightPriority == 1 { // Theme wins
		fh.Rules = append(fh.Rules, fh.ThemeRules...)
		fh.Rules = append(fh.Rules, fh.UserRules...)
	} else { // User wins
		fh.Rules = append(fh.Rules, fh.UserRules...)
		fh.Rules = append(fh.Rules, fh.ThemeRules...)
	}
	fh.Revision = highlightRulesRevision(fh.Rules)
	for i := range fh.Rules {
		fh.Rules[i].maskIndex = compileHighlightMaskIndex(fh.Rules[i])
	}
	fh.clearMatchCache()
	vtui.DebugLog("HIGHLIGHT: Loaded %d file highlighting rules", len(fh.Rules))
}

func parseHighlightRules(ini *IniFile) []HighlightRule {
	return parseHighlightRulesAt(ini, "")
}

func parseHighlightRulesAt(ini *IniFile, baseDir string) []HighlightRule {
	var rules []HighlightRule
	var sections []string
	for secName := range ini.data {
		if strings.HasPrefix(strings.ToLower(secName), "highlight_") {
			sections = append(sections, secName)
		}
	}
	sort.Slice(sections, func(i, j int) bool {
		idxI, _ := strconv.Atoi(strings.TrimPrefix(strings.ToLower(sections[i]), "highlight_"))
		idxJ, _ := strconv.Atoi(strings.TrimPrefix(strings.ToLower(sections[j]), "highlight_"))
		return idxI < idxJ
	})

	for _, secName := range sections {
		rule := HighlightRule{
			RuleID:     secName,
			Name:       ini.GetString(secName, "Name", ""),
			IgnoreCase: true,
		}
		maskStr := ini.GetString(secName, "Mask", "")
		if maskStr != "" {
			rawMasks := strings.Split(maskStr, ",")
			for _, m := range rawMasks {
				m = strings.TrimSpace(m)
				if m == "*.*" {
					m = "*"
				}
				if m != "" {
					rule.Masks = append(rule.Masks, m)
				}
			}
		} else {
			rule.Masks = []string{"*"}
		}

		attrInclude := strings.ToLower(ini.GetString(secName, "IncludeAttributes", ""))
		attrExclude := strings.ToLower(ini.GetString(secName, "ExcludeAttributes", ""))
		rule.AttrSet = parseAttrFlags(attrInclude)
		rule.AttrClear = parseAttrFlags(attrExclude)

		// Чтение размеров
		sizeAboveStr := ini.GetString(secName, "SizeAbove", "")
		if sizeAboveStr != "" {
			fmt.Sscanf(sizeAboveStr, "%d", &rule.SizeAbove)
		}
		sizeBelowStr := ini.GetString(secName, "SizeBelow", "")
		if sizeBelowStr != "" {
			fmt.Sscanf(sizeBelowStr, "%d", &rule.SizeBelow)
		}

		// Чтение дат
		dateTypeStr := strings.ToLower(ini.GetString(secName, "DateType", ""))
		switch dateTypeStr {
		case "create", "created", "c":
			rule.DateType = DateCreated
		case "access", "accessed", "a":
			rule.DateType = DateAccessed
		default:
			rule.DateType = DateModified
		}

		rule.DateRelative = ini.GetString(secName, "DateRelative", "0") == "1"

		parseDuration := func(s string) (time.Duration, error) {
			s = strings.TrimSpace(s)
			if strings.HasSuffix(s, "d") {
				daysStr := strings.TrimSuffix(s, "d")
				days, err := strconv.Atoi(daysStr)
				if err != nil {
					return 0, err
				}
				return time.Duration(days) * 24 * time.Hour, nil
			}
			return time.ParseDuration(s)
		}

		dateAfterStr := ini.GetString(secName, "DateAfter", "")
		if dateAfterStr != "" {
			if rule.DateRelative {
				if dur, err := parseDuration(dateAfterStr); err == nil {
					rule.DateAfterDur = dur
				}
			} else {
				if t, err := time.Parse("2006-01-02 15:04:05", dateAfterStr); err == nil {
					rule.DateAfter = t
				}
			}
		}

		dateBeforeStr := ini.GetString(secName, "DateBefore", "")
		if dateBeforeStr != "" {
			if rule.DateRelative {
				if dur, err := parseDuration(dateBeforeStr); err == nil {
					rule.DateBeforeDur = dur
				}
			} else {
				if t, err := time.Parse("2006-01-02 15:04:05", dateBeforeStr); err == nil {
					rule.DateBefore = t
				}
			}
		}

		rule.ContinueProcessing = ini.GetString(secName, "ContinueProcessing", "0") == "1"

		rule.Mark = ini.GetString(secName, "Mark", "")
		if rule.Mark == "" {
			rule.Mark = ini.GetString(secName, "MarkChar", "")
		}
		rule.IconURL = normalizeHighlightIconURL(
			ini.GetString(secName, "Icon", ""), baseDir)

		rule.NormalStr = ini.GetString(secName, "NormalColor", "")
		rule.SelectedStr = ini.GetString(secName, "SelectedColor", "")
		rule.CursorStr = ini.GetString(secName, "CursorColor", "")
		rule.SelectedCursorStr = ini.GetString(secName, "SelectedCursorColor", "")
		rules = append(rules, rule)
	}
	return rules
}

func (fh *FileHighlighter) clearMatchCache() {
	fh.matchCacheMu.Lock()
	fh.matchCache = nil
	fh.matchCacheRevision = fh.Revision
	fh.matchCacheMu.Unlock()
	fh.semanticStyleCacheMu.Lock()
	fh.semanticStyleCache = nil
	fh.semanticStyleCacheRevision = fh.Revision
	fh.semanticStyleCacheMu.Unlock()
}

func highlightMatchKey(item *vfs.VFSItem, metadataKnown bool) highlightMatchCacheKey {
	return highlightMatchCacheKey{
		Name:          item.Name,
		Size:          item.Size,
		MTime:         semanticMTimeNanos(item.MTime),
		ATime:         semanticMTimeNanos(item.ATime),
		CTime:         semanticMTimeNanos(item.CTime),
		UnixMode:      item.UnixMode,
		WinAttrs:      item.WinAttrs,
		IsDir:         item.IsDir,
		IsHidden:      item.IsHidden,
		IsExecutable:  item.IsExecutable,
		MetadataKnown: metadataKnown,
	}
}

func (fh *FileHighlighter) hasRelativeDateRules() bool {
	for i := range fh.Rules {
		if fh.Rules[i].DateRelative {
			return true
		}
	}
	return false
}

// matchedRuleIndices performs the rule/mask work once per immutable directory
// entry. Text rows, markers, and the semantic GUI representation all consume
// the same result, so recomputing filepath.Match in each presentation path is
// both redundant and especially expensive during key repeat.
func (fh *FileHighlighter) matchedRuleIndices(item *vfs.VFSItem, metadataKnown bool) []int {
	if fh == nil || item == nil {
		return nil
	}
	if fh.hasRelativeDateRules() {
		return fh.computeMatchedRuleIndices(item, metadataKnown)
	}

	key := highlightMatchKey(item, metadataKnown)
	fh.matchCacheMu.RLock()
	if fh.matchCacheRevision == fh.Revision {
		if cached, ok := fh.matchCache[key]; ok {
			fh.matchCacheMu.RUnlock()
			return cached
		}
	}
	fh.matchCacheMu.RUnlock()

	matched := fh.computeMatchedRuleIndices(item, metadataKnown)
	fh.matchCacheMu.Lock()
	if fh.matchCacheRevision != fh.Revision || len(fh.matchCache) >= maxHighlightMatchCacheEntries {
		fh.matchCache = make(map[highlightMatchCacheKey][]int)
		fh.matchCacheRevision = fh.Revision
	} else if fh.matchCache == nil {
		fh.matchCache = make(map[highlightMatchCacheKey][]int)
	}
	fh.matchCache[key] = matched
	fh.matchCacheMu.Unlock()
	return matched
}

func (fh *FileHighlighter) computeMatchedRuleIndices(item *vfs.VFSItem, metadataKnown bool) []int {
	var matched []int
	for i := range fh.Rules {
		if !fh.Rules[i].Match(item, metadataKnown) {
			continue
		}
		matched = append(matched, i)
		if !fh.Rules[i].ContinueProcessing {
			break
		}
	}
	return matched
}

func (fh *FileHighlighter) semanticMatchedRuleIndices(item *vfs.VFSItem, metadataKnown bool) []int {
	if fh == nil || item == nil {
		return nil
	}
	if !item.IsHidden {
		return fh.matchedRuleIndices(item, metadataKnown)
	}
	var matched []int
	for i := range fh.Rules {
		// Hidden-only presentation rules do not participate in semantic GUI
		// cascading and therefore must not terminate it either.
		if item.IsHidden && fh.Rules[i].AttrSet&AttrHidden != 0 {
			continue
		}
		if !fh.Rules[i].Match(item, metadataKnown) {
			continue
		}
		matched = append(matched, i)
		if !fh.Rules[i].ContinueProcessing {
			break
		}
	}
	return matched
}

func normalizeHighlightIconURL(raw, baseDir string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "qrc":
		if parsed.Host != "" || parsed.Path == "" || !strings.HasPrefix(raw, "qrc:/") {
			return ""
		}
		return "qrc:" + filepath.ToSlash(filepath.Clean(parsed.Path))
	case "file":
		if parsed.Host != "" && parsed.Host != "localhost" {
			return ""
		}
		path := parsed.Path
		if parsed.Opaque != "" {
			path = parsed.Opaque
		}
		if path == "" {
			return ""
		}
		if !filepath.IsAbs(path) {
			if baseDir == "" {
				return ""
			}
			path = filepath.Join(baseDir, filepath.FromSlash(path))
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return ""
		}
		if info, err := os.Stat(absolute); err != nil || info.IsDir() {
			return ""
		}
		return (&url.URL{Scheme: "file", Path: absolute}).String()
	default:
		return ""
	}
}

func highlightRulesRevision(rules []HighlightRule) int64 {
	data, _ := json.Marshal(rules)
	digest := sha256.Sum256(data)
	revision := int64(binary.BigEndian.Uint64(digest[:8]) & 0x7fffffffffffffff)
	if revision == 0 && len(rules) > 0 {
		return 1
	}
	return revision
}

func parseAttrFlags(s string) AttrFlags {
	var flags AttrFlags
	parts := strings.Split(s, ",")
	for _, p := range parts {
		switch strings.TrimSpace(p) {
		case "directory", "dir", "d":
			flags |= AttrDirectory
		case "hidden", "h":
			flags |= AttrHidden
		case "executable", "exec", "e":
			flags |= AttrExecutable
		case "readonly", "ro":
			flags |= AttrReadOnly
		case "system", "sys":
			flags |= AttrSystem
		case "archive", "arc":
			flags |= AttrArchive
		}
	}
	return flags
}

// hasDeferredPredicate reports whether this rule filters on data that a
// panel's fast base pass never has (size, dates, and the readonly/system/
// archive/executable attributes, all only populated by the metadata pass).
// Matching against those fields' zero values before metadata arrives can
// produce a wrong color rather than merely no color yet (a zero time.Time,
// for instance, satisfies almost any "before date X" rule).
func (r *HighlightRule) hasDeferredPredicate() bool {
	const deferredAttrs = AttrExecutable | AttrReadOnly | AttrSystem | AttrArchive
	if r.AttrSet&deferredAttrs != 0 || r.AttrClear&deferredAttrs != 0 {
		return true
	}
	if r.SizeAbove > 0 || r.SizeBelow > 0 {
		return true
	}
	if !r.DateAfter.IsZero() || r.DateAfterDur > 0 ||
		!r.DateBefore.IsZero() || r.DateBeforeDur > 0 {
		return true
	}
	return false
}

// metadataKnown is false for a panel's fast base pass, where only
// Name/IsDir/IsHidden/IsSymlink are populated. A rule that depends on other
// fields is skipped entirely in that pass (see hasDeferredPredicate) rather
// than evaluated against those fields' zero values, so the base pass shows
// either the correct color or none — never a wrong one. The metadata pass
// (metadataKnown true) re-evaluates every rule against the complete item and
// its result overwrites the provisional one.
func (r *HighlightRule) Match(item *vfs.VFSItem, metadataKnown bool) bool {
	if !metadataKnown && r.hasDeferredPredicate() {
		return false
	}
	// Определение платформозависимых флагов "на лету"
	isReadOnly := false
	isSystem := false
	isArchive := false
	if runtime.GOOS == "windows" {
		isReadOnly = item.WinAttrs&1 != 0 // FILE_ATTRIBUTE_READONLY
		isSystem = item.WinAttrs&4 != 0   // FILE_ATTRIBUTE_SYSTEM
		isArchive = item.WinAttrs&32 != 0 // FILE_ATTRIBUTE_ARCHIVE
	} else {
		isReadOnly = item.UnixMode&0222 == 0 // Нет прав на запись
	}

	matchAttr := func(flag AttrFlags, set bool) bool {
		switch flag {
		case AttrDirectory:
			return item.IsDir == set
		case AttrHidden:
			return item.IsHidden == set
		case AttrExecutable:
			return item.IsExecutable == set
		case AttrReadOnly:
			return isReadOnly == set
		case AttrSystem:
			return isSystem == set
		case AttrArchive:
			return isArchive == set
		}
		return true
	}

	// Проверка AttrSet (должны присутствовать)
	for _, f := range []AttrFlags{AttrDirectory, AttrHidden, AttrExecutable, AttrReadOnly, AttrSystem, AttrArchive} {
		if r.AttrSet&f != 0 && !matchAttr(f, true) {
			return false
		}
	}

	// Проверка AttrClear (должны отсутствовать)
	for _, f := range []AttrFlags{AttrDirectory, AttrHidden, AttrExecutable, AttrReadOnly, AttrSystem, AttrArchive} {
		if r.AttrClear&f != 0 && !matchAttr(f, false) {
			return false
		}
	}

	// Фильтрация по размеру
	if r.SizeAbove > 0 && item.Size < r.SizeAbove {
		return false
	}
	if r.SizeBelow > 0 && item.Size > r.SizeBelow {
		return false
	}

	// Фильтрация по датам
	if !r.DateAfter.IsZero() || r.DateAfterDur > 0 || !r.DateBefore.IsZero() || r.DateBeforeDur > 0 {
		var t time.Time
		switch r.DateType {
		case DateCreated:
			t = item.CTime
		case DateAccessed:
			t = item.ATime
		default:
			t = item.MTime
		}

		if r.DateRelative {
			if r.DateAfterDur > 0 && t.Before(time.Now().Add(-r.DateAfterDur)) {
				return false
			}
			if r.DateBeforeDur > 0 && t.After(time.Now().Add(-r.DateBeforeDur)) {
				return false
			}
		} else {
			if !r.DateAfter.IsZero() && t.Before(r.DateAfter) {
				return false
			}
			if !r.DateBefore.IsZero() && t.After(r.DateBefore) {
				return false
			}
		}
	}

	// Проверка по маске имени файла
	if r.maskIndex != nil {
		return r.maskIndex.matches(item.Name, r.IgnoreCase)
	}
	if len(r.Masks) == 0 {
		return true
	}
	name := item.Name
	if r.IgnoreCase {
		name = strings.ToLower(name)
	}
	for _, mask := range r.Masks {
		m := mask
		if r.IgnoreCase {
			m = strings.ToLower(m)
		}
		matched, err := filepath.Match(m, name)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func (fh *FileHighlighter) GetColor(item *vfs.VFSItem, defaultAttr uint64, isSelected, isCursor bool) uint64 {
	if item.Name == ".." {
		return defaultAttr
	}
	attr := defaultAttr
	matchedAny := false

	// Every GetColor caller renders a fully-loaded panel entry (text-mode
	// rendering and path hints never see a deferred/partial VFSItem).
	for _, ruleIndex := range fh.matchedRuleIndices(item, true) {
		rule := fh.Rules[ruleIndex]
		colorExpr := ""
		if isCursor {
			if isSelected {
				if rule.SelectedCursorStr != "" {
					colorExpr = rule.SelectedCursorStr
				} else if rule.SelectedStr != "" {
					colorExpr = rule.SelectedStr
				}
			} else {
				if rule.CursorStr != "" {
					colorExpr = rule.CursorStr
				}
			}
		} else if isSelected {
			if rule.SelectedStr != "" {
				colorExpr = rule.SelectedStr
			}
		} else {
			if rule.NormalStr != "" {
				colorExpr = rule.NormalStr
			}
		}

		if colorExpr != "" {
			attr = ParseFarColor(colorExpr, attr)
			matchedAny = true
		}

		// Если каскадная обработка выключена, сразу возвращаем результат
		if !rule.ContinueProcessing {
			if matchedAny {
				if AppConfig.EnforceColorCorrection {
					fg, bg := GetColorRGBBoth(attr)
					nfg := CorrectContrast(fg, bg)
					if nfg != fg {
						attr = vtui.SetRGBFore(attr, nfg)
					}
				}
				return attr
			}
			return defaultAttr
		}
	}

	if matchedAny {
		if AppConfig.EnforceColorCorrection {
			fg, bg := GetColorRGBBoth(attr)
			nfg := CorrectContrast(fg, bg)
			if nfg != fg {
				attr = vtui.SetRGBFore(attr, nfg)
			}
		}
		return attr
	}
	return defaultAttr
}

// GetMarker возвращает символ пометки для файла от первого совпавшего правила.
func (fh *FileHighlighter) GetMarker(item *vfs.VFSItem) string {
	if item.Name == ".." {
		return ""
	}
	for _, ruleIndex := range fh.matchedRuleIndices(item, true) {
		rule := fh.Rules[ruleIndex]
		if rule.Mark != "" {
			return rule.Mark
		}
		if !rule.ContinueProcessing {
			break
		}
	}
	return ""
}

func highlightRuleColor(rule HighlightRule, selected, cursor bool) string {
	if cursor && selected && rule.SelectedCursorStr != "" {
		return rule.SelectedCursorStr
	}
	if cursor && rule.CursorStr != "" {
		return rule.CursorStr
	}
	if selected && rule.SelectedStr != "" {
		return rule.SelectedStr
	}
	return rule.NormalStr
}

func semanticColorPatch(expr string) extui.HighlightColorPatchModel {
	var patch extui.HighlightColorPatchModel
	for _, rawPart := range strings.Split(expr, "|") {
		part := strings.TrimSpace(rawPart)
		if strings.HasPrefix(part, "foreground:#") && len(part) >= 18 {
			if value, err := strconv.ParseUint(part[12:18], 16, 32); err == nil {
				patch.Foreground = fmt.Sprintf("#%06X", value)
			}
			continue
		}
		if strings.HasPrefix(part, "background:#") && len(part) >= 18 {
			if value, err := strconv.ParseUint(part[12:18], 16, 32); err == nil {
				patch.Background = fmt.Sprintf("#%06X", value)
			}
			continue
		}
		if colorIndex, ok := namedColors[part]; ok {
			if strings.HasPrefix(part, "F_") {
				patch.Foreground = fmt.Sprintf("#%06X", far2lPalette[colorIndex])
			} else if strings.HasPrefix(part, "B_") {
				patch.Background = fmt.Sprintf("#%06X", far2lPalette[colorIndex>>4])
			}
		}
	}
	return patch
}

func mergeHighlightPatch(target *extui.HighlightColorPatchModel, patch extui.HighlightColorPatchModel) {
	if patch.Foreground != "" {
		target.Foreground = patch.Foreground
	}
	if patch.Background != "" {
		target.Background = patch.Background
	}
}

func highlightStyleEmpty(style extui.HighlightStyleModel) bool {
	return len(style.Groups) == 0 && style.Marker == "" && style.IconKey == "" && style.Icon == "" &&
		style.Normal.Foreground == "" && style.Normal.Background == "" &&
		style.Selected.Foreground == "" && style.Selected.Background == "" &&
		style.Cursor.Foreground == "" && style.Cursor.Background == "" &&
		style.SelectedCursor.Foreground == "" && style.SelectedCursor.Background == ""
}

// semanticHighlightIconKey keeps host-owned Lucide resources out of the
// reusable gallery contract. Custom qrc/file URLs intentionally keep only
// Icon: those are user appearance overrides, not semantic icon names.
func semanticHighlightIconKey(source string) string {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || strings.ToLower(parsed.Scheme) != "qrc" {
		return ""
	}
	path := filepath.ToSlash(parsed.Path)
	if !strings.Contains(path, "/lucide/") && !strings.Contains(path, "/lucide-gallery/") {
		return ""
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if name == "" {
		return ""
	}
	for _, character := range name {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '-' && character != '_' {
			return ""
		}
	}
	return name
}

func semanticStyleCacheKey(matched []int, parentEntry bool) string {
	key := make([]byte, 1+len(matched)*4)
	if parentEntry {
		key[0] = 1
	}
	for index, ruleIndex := range matched {
		binary.LittleEndian.PutUint32(key[1+index*4:], uint32(ruleIndex))
	}
	return string(key)
}

// SemanticStyle returns a presentation-neutral style for QML. It preserves
// the existing Far cascade while keeping vtui attributes out of the scene.
// metadataKnown must be false when item only carries a panel's fast base-pass
// fields (Name/IsDir/IsHidden/IsSymlink); rules that need anything else are
// then skipped rather than matched against zero values (see
// HighlightRule.hasDeferredPredicate). The later metadata pass calls this
// again with metadataKnown true and its result supersedes the provisional
// one.
func (fh *FileHighlighter) SemanticStyle(item *vfs.VFSItem, metadataKnown bool) (string, extui.HighlightStyleModel) {
	var style extui.HighlightStyleModel
	if fh == nil || item == nil {
		return "", style
	}
	parentEntry := item.Name == ".."
	matched := fh.semanticMatchedRuleIndices(item, metadataKnown)
	cacheKey := semanticStyleCacheKey(matched, parentEntry)
	fh.semanticStyleCacheMu.RLock()
	if fh.semanticStyleCacheRevision == fh.Revision {
		if cached, ok := fh.semanticStyleCache[cacheKey]; ok {
			fh.semanticStyleCacheMu.RUnlock()
			return cached.id, cached.style
		}
	}
	fh.semanticStyleCacheMu.RUnlock()
	for _, ruleIndex := range matched {
		rule := fh.Rules[ruleIndex]
		style.Groups = append(style.Groups, extui.HighlightGroupModel{
			ID:   rule.RuleID,
			Name: rule.Name,
		})
		if style.Icon == "" && rule.IconURL != "" {
			style.Icon = rule.IconURL
			style.IconKey = semanticHighlightIconKey(rule.IconURL)
		}
		if !parentEntry {
			if style.Marker == "" && rule.Mark != "" {
				style.Marker = rule.Mark
			}
			mergeHighlightPatch(&style.Normal,
				semanticColorPatch(highlightRuleColor(rule, false, false)))
			mergeHighlightPatch(&style.Selected,
				semanticColorPatch(highlightRuleColor(rule, true, false)))
			mergeHighlightPatch(&style.Cursor,
				semanticColorPatch(highlightRuleColor(rule, false, true)))
			mergeHighlightPatch(&style.SelectedCursor,
				semanticColorPatch(highlightRuleColor(rule, true, true)))
		}
		if !rule.ContinueProcessing {
			break
		}
	}
	if highlightStyleEmpty(style) {
		fh.rememberSemanticStyle(cacheKey, "", style)
		return "", style
	}
	encoded, _ := json.Marshal(style)
	digest := sha256.Sum256(encoded)
	id := hex.EncodeToString(digest[:])
	fh.rememberSemanticStyle(cacheKey, id, style)
	return id, style
}

func (fh *FileHighlighter) rememberSemanticStyle(key, id string, style extui.HighlightStyleModel) {
	fh.semanticStyleCacheMu.Lock()
	defer fh.semanticStyleCacheMu.Unlock()
	if fh.semanticStyleCacheRevision != fh.Revision || len(fh.semanticStyleCache) >= maxSemanticStyleCacheEntries {
		fh.semanticStyleCache = make(map[string]semanticStyleCacheValue)
		fh.semanticStyleCacheRevision = fh.Revision
	} else if fh.semanticStyleCache == nil {
		fh.semanticStyleCache = make(map[string]semanticStyleCacheValue)
	}
	fh.semanticStyleCache[key] = semanticStyleCacheValue{id: id, style: style}
}
