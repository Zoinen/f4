package filegroups

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Predicate is immutable. All regexes, dates and limits are compiled once.
type Predicate struct {
	include, exclude              []compiledMask
	attrs, excludedAttrs          uint32
	minimum, maximum              int64
	dateType                      string
	after, before                 time.Time
	afterDuration, beforeDuration time.Duration
	relative, deferred            bool
	lowerName                     bool
}

type compiledMask struct {
	expression *regexp.Regexp
	lower      bool
	literal    bool
	wildcard   bool
	prefix     string
	suffix     string
}

func (m compiledMask) MatchString(name string) bool {
	if m.lower {
		name = strings.ToLower(name)
	}
	if m.literal {
		if !m.wildcard {
			return name == m.prefix
		}
		if len(name) < len(m.prefix)+len(m.suffix) ||
			!strings.HasPrefix(name, m.prefix) || !strings.HasSuffix(name, m.suffix) {
			return false
		}
		return !strings.ContainsRune(name[len(m.prefix):len(name)-len(m.suffix)], filepath.Separator)
	}
	return m.expression.MatchString(name)
}

const (
	attrDir uint32 = 1 << iota
	attrHidden
	attrExec
	attrReadOnly
	attrSystem
	attrArchive
	attrSymlink
	attrJunction
)

func CompilePredicate(fields map[string]string) (*Predicate, error) {
	p := &Predicate{}
	mask := fields["Mask"]
	if mask == "" {
		mask = "*"
	}
	ignore := fields["Case"] != "sensitive"
	if fields["Case"] == "platform" {
		ignore = platformIgnoreCase()
	}
	legacy := fields["MaskSyntax"] == "legacy-glob"
	p.lowerName = legacy && ignore
	if p.lowerName {
		mask = strings.ToLower(mask)
		ignore = false
	}
	if syntax := fields["MaskSyntax"]; syntax != "" && syntax != "far" && !legacy {
		return nil, fmt.Errorf("unknown mask syntax %q", syntax)
	}
	if mode := fields["Case"]; mode != "" && mode != "insensitive" && mode != "sensitive" && mode != "platform" {
		return nil, fmt.Errorf("unknown case matching %q", mode)
	}
	include, exclude := mask, ""
	if !legacy {
		parts := splitFarMasks(mask, "|", true)
		include = parts[0]
		if len(parts) > 1 {
			exclude = parts[1]
		}
	}
	var err error
	p.include, err = compileMasks(include, ignore, legacy)
	if err != nil {
		return nil, err
	}
	if len(p.include) == 0 {
		return nil, fmt.Errorf("file name mask must not be empty")
	}
	if exclude != "" {
		p.exclude, err = compileMasks(exclude, ignore, false)
		if err != nil {
			return nil, err
		}
	}
	p.attrs, err = parseAttributes(fields["IncludeAttributes"])
	if err != nil {
		return nil, err
	}
	p.excludedAttrs, err = parseAttributes(fields["ExcludeAttributes"])
	if err != nil {
		return nil, err
	}
	if p.attrs&p.excludedAttrs != 0 {
		return nil, fmt.Errorf("an attribute cannot be both included and excluded")
	}
	if (p.attrs|p.excludedAttrs)&attrJunction != 0 {
		vtui.DebugLog("[FIX:file-groups] compiling Junction predicate from platform-independent VFS metadata")
	}
	for key, target := range map[string]*int64{"SizeAbove": &p.minimum, "SizeBelow": &p.maximum} {
		if raw := fields[key]; raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 0 {
				return nil, fmt.Errorf("%s must be a nonnegative byte count", key)
			}
			*target = value
		}
	}
	if p.maximum > 0 && p.minimum > p.maximum {
		return nil, fmt.Errorf("minimum size exceeds maximum size")
	}
	p.dateType = strings.ToLower(fields["DateType"])
	switch p.dateType {
	case "", "modified", "mod", "m", "created", "create", "c", "accessed", "access", "a":
	default:
		return nil, fmt.Errorf("unknown timestamp type %q", p.dateType)
	}
	p.relative = fields["DateRelative"] == "1" || fields["DateRelative"] == "true"
	for key, target := range map[string]*time.Time{"DateAfter": &p.after, "DateBefore": &p.before} {
		raw := fields[key]
		if raw == "" {
			continue
		}
		if p.relative {
			duration, err := parseDuration(raw)
			if err != nil || duration < 0 {
				return nil, fmt.Errorf("invalid %s duration %q", key, raw)
			}
			if key == "DateAfter" {
				p.afterDuration = duration
			} else {
				p.beforeDuration = duration
			}
		} else {
			value, err := time.Parse("2006-01-02 15:04:05", raw)
			if err != nil {
				return nil, fmt.Errorf("invalid %s: %w", key, err)
			}
			*target = value
		}
	}
	p.deferred = p.minimum > 0 || p.maximum > 0 || !p.after.IsZero() || !p.before.IsZero() || p.afterDuration > 0 || p.beforeDuration > 0 || (p.attrs|p.excludedAttrs)&(attrExec|attrReadOnly|attrSystem|attrArchive|attrJunction) != 0
	return p, nil
}

func parseDuration(raw string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(raw, "d"); ok {
		n, err := strconv.ParseInt(days, 10, 64)
		if err != nil || n < 0 || n > int64((1<<63-1)/(24*time.Hour)) {
			return 0, fmt.Errorf("invalid day duration")
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(raw)
}

type fileAttribute struct {
	name    string
	mask    uint32
	aliases []string
}

var fileAttributes = [...]fileAttribute{
	{name: "Directory", mask: attrDir, aliases: []string{"dir", "d"}},
	{name: "Hidden", mask: attrHidden, aliases: []string{"h"}},
	{name: "Executable", mask: attrExec, aliases: []string{"exec", "e"}},
	{name: "ReadOnly", mask: attrReadOnly, aliases: []string{"ro"}},
	{name: "System", mask: attrSystem, aliases: []string{"sys"}},
	{name: "Archive", mask: attrArchive, aliases: []string{"arc"}},
	{name: "Symlink", mask: attrSymlink, aliases: []string{"link", "sym", "l"}},
	{name: "Junction", mask: attrJunction, aliases: []string{"junc", "j"}},
}

// AttributeNames returns the canonical names accepted in file group conditions.
func AttributeNames() []string {
	names := make([]string, 0, len(fileAttributes))
	for _, attribute := range fileAttributes {
		names = append(names, attribute.name)
	}
	return names
}

// CanonicalAttributeName resolves an INI name or alias, or returns an empty
// string if the name is unknown.
func CanonicalAttributeName(name string) string {
	return lookupFileAttribute(name).name
}

func lookupFileAttribute(name string) fileAttribute {
	name = strings.ToLower(name)
	for _, attribute := range fileAttributes {
		if strings.EqualFold(name, attribute.name) || slices.Contains(attribute.aliases, name) {
			return attribute
		}
	}
	return fileAttribute{}
}

func parseAttributes(raw string) (uint32, error) {
	var result uint32
	for _, token := range strings.Split(strings.ToLower(raw), ",") {
		name := strings.TrimSpace(token)
		if name == "" {
			continue
		}
		attribute := lookupFileAttribute(name)
		if attribute.mask == 0 {
			return 0, fmt.Errorf("unknown file attribute %q", token)
		}
		result |= attribute.mask
	}
	return result, nil
}

func compileMasks(raw string, ignore, legacy bool) ([]compiledMask, error) {
	patterns := []string{}
	if legacy {
		patterns = strings.Split(raw, ",")
	} else {
		patterns = splitFarMasks(raw, ",;", false)
	}
	result := []compiledMask{}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if !legacy && strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") && len(pattern) > 1 {
			expression := pattern[1 : len(pattern)-1]
			if ignore {
				expression = "(?i)" + expression
			}
			re, err := regexp.Compile(expression)
			if err != nil {
				return nil, err
			}
			result = append(result, compiledMask{expression: re})
			continue
		}
		if pattern == "*.*" {
			pattern = "*"
		}
		if _, err := filepath.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("invalid mask %q: %w", pattern, err)
		}
		if ignore {
			pattern = strings.ToLower(pattern)
		}
		re, err := regexp.Compile(globRegexp(pattern, false))
		if err != nil {
			return nil, fmt.Errorf("invalid mask %q: %w", pattern, err)
		}
		mask := compiledMask{expression: re, lower: ignore}
		// Most icon masks are exact names or a single literal prefix/suffix.
		// Keep classes, escapes and multiple wildcards on the regexp path.
		if !strings.ContainsAny(pattern, "?[]\\") && strings.Count(pattern, "*") <= 1 {
			mask.literal = true
			mask.prefix, mask.suffix, mask.wildcard = strings.Cut(pattern, "*")
		}
		result = append(result, mask)
	}
	return result, nil
}

// MaskPreviewParts separates comma-delimited chips without splitting Far regular expressions.
func MaskPreviewParts(raw string, legacy bool) []string {
	var parts []string
	if legacy {
		parts = strings.Split(raw, ",")
	} else {
		parts = splitFarMasks(raw, ",", false)
	}
	chips := parts[:0]
	for _, part := range parts {
		if chip := strings.TrimSpace(part); chip != "" {
			chips = append(chips, chip)
		}
	}
	return chips
}

func splitFarMasks(raw, separators string, firstOnly bool) []string {
	result := []string{}
	inRegex, start := false, 0
	for i, r := range raw {
		if r == '/' {
			inRegex = !inRegex
		}
		if !inRegex && strings.ContainsRune(separators, r) {
			result = append(result, raw[start:i])
			start = i + 1
			if firstOnly {
				break
			}
		}
	}
	return append(result, raw[start:])
}

func globRegexp(pattern string, ignore bool) string {
	var out strings.Builder
	if ignore {
		out.WriteString("(?i)")
	}
	out.WriteString("^")
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch r := runes[i]; r {
		case '*':
			if filepath.Separator == '\\' {
				out.WriteString(`[^\\]*`)
			} else {
				out.WriteString(`[^/]*`)
			}
		case '?':
			if filepath.Separator == '\\' {
				out.WriteString(`[^\\]`)
			} else {
				out.WriteString(`[^/]`)
			}
		case '[':
			out.WriteRune(r)
			for i++; i < len(runes); i++ {
				out.WriteRune(runes[i])
				if runes[i] == ']' {
					break
				}
			}
		case '\\':
			if filepath.Separator == '\\' {
				out.WriteString(`\\`)
				continue
			}
			if i+1 < len(runes) {
				i++
				out.WriteString(regexp.QuoteMeta(string(runes[i])))
			}
		default:
			out.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	out.WriteString("$")
	return out.String()
}

func (p *Predicate) Match(item *vfs.VFSItem, metadataKnown bool, now time.Time) bool {
	if item == nil || (!metadataKnown && p.deferred) {
		return false
	}
	attrs := platformAttributes(item)
	if item.IsDir {
		attrs |= attrDir
	}
	if item.IsHidden {
		attrs |= attrHidden
	}
	if item.IsExecutable && !item.IsDir {
		attrs |= attrExec
	}
	if item.IsSymlink {
		attrs |= attrSymlink
	}
	// Reparse tags describe the VFS entry, not the OS rendering it. A plugin
	// can supply Windows link metadata even when f4 runs on another platform.
	if vfs.LinkKindOf(item) == vfs.LinkJunction {
		attrs |= attrJunction
	}
	if attrs&p.attrs != p.attrs || attrs&p.excludedAttrs != 0 {
		return false
	}
	if p.minimum > 0 && item.Size < p.minimum || p.maximum > 0 && item.Size > p.maximum {
		return false
	}
	stamp := item.MTime
	switch p.dateType {
	case "created", "create", "c":
		stamp = item.CTime
	case "accessed", "access", "a":
		stamp = item.ATime
	}
	if p.relative {
		if p.afterDuration > 0 && stamp.Before(now.Add(-p.afterDuration)) || p.beforeDuration > 0 && stamp.After(now.Add(-p.beforeDuration)) {
			return false
		}
	} else if !p.after.IsZero() && stamp.Before(p.after) || !p.before.IsZero() && stamp.After(p.before) {
		return false
	}
	name := item.Name
	if p.lowerName {
		name = strings.ToLower(name)
	}
	for _, re := range p.exclude {
		if re.MatchString(name) {
			return false
		}
	}
	for _, re := range p.include {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

type compiledGroup struct {
	group      Group
	predicate  *Predicate
	highlight  *Predicate
	highlights map[string]*Predicate
	icons      map[string]IconReference
}

// Snapshot cannot be mutated through the public API. Groups and matches are
// returned as copies; predicates and the revision remain shared and immutable.
type Snapshot struct {
	themes       map[string]ThemeSet
	colorThemes  []ColorTheme
	colorTheme   string
	iconTheme    string
	groups       []compiledGroup
	revision     string
	sortRevision string
	source       *Snapshot
	viewsMu      sync.Mutex
	views        map[string]*Snapshot
}

func Compile(groups []Group, themeSets ...[]ThemeSet) (*Snapshot, error) {
	s := &Snapshot{themes: map[string]ThemeSet{}}
	if len(themeSets) > 0 {
		if err := ValidateThemes(themeSets[0]); err != nil {
			return nil, err
		}
		for _, set := range themeSets[0] {
			s.themes[set.ID] = ThemeSet{ID: set.ID, Fields: maps.Clone(set.Fields)}
		}
	}
	seen := map[string]bool{}
	for _, g := range groups {
		if err := validID(g.ID); err != nil {
			return nil, err
		}
		if seen[g.ID] {
			return nil, fmt.Errorf("duplicate group %q", g.ID)
		}
		seen[g.ID] = true
		if scope := g.IconTheme(); scope != "" {
			if err := validID(scope); err != nil {
				return nil, fmt.Errorf("group %s: invalid icon theme: %w", g.ID, err)
			}
		}
		p, err := CompilePredicate(g.Fields)
		if err != nil {
			return nil, fmt.Errorf("group %s: %w", g.ID, err)
		}
		if raw := g.Fields["Sort.Number"]; raw != "" {
			if _, err := strconv.Atoi(raw); err != nil {
				return nil, fmt.Errorf("group %s: invalid sort group", g.ID)
			}
		}
		icons, err := groupIconReferences(g, s.themes)
		if err != nil {
			return nil, err
		}
		commands := map[string]bool{}
		for _, command := range g.Commands {
			if err := validID(command.ID); err != nil {
				return nil, err
			}
			if commands[command.ID] {
				return nil, fmt.Errorf("group %s: duplicate command identity %s", g.ID, command.ID)
			}
			commands[command.ID] = true
		}
		for _, key := range []string{"Enabled", "DateRelative", "ContinueProcessing", "UseDefaults", "OverrideNativeIcon", "Highlight.Enabled", "Icons.Enabled", "Sort.Enabled", "Associations.Enabled"} {
			switch g.Fields[key] {
			case "", "true", "false", "0", "1":
			default:
				return nil, fmt.Errorf("group %s: invalid boolean %s", g.ID, key)
			}
		}
		highlight, err := compileHighlight(g, "")
		if err != nil {
			return nil, fmt.Errorf("group %s highlighting: %w", g.ID, err)
		}
		styles, err := paletteStyles(g)
		if err != nil {
			return nil, fmt.Errorf("group %s: %w", g.ID, err)
		}
		highlights := map[string]*Predicate{}
		for _, style := range styles {
			predicate, err := compileHighlight(g, style)
			if err != nil {
				return nil, fmt.Errorf("group %s palette %q: %w", g.ID, style, err)
			}
			highlights[style] = predicate
		}
		s.groups = append(s.groups, compiledGroup{
			group: g.Clone(), predicate: p, highlight: highlight, highlights: highlights, icons: icons,
		})
	}
	return finishSnapshot(s, groups)
}

func finishSnapshot(s *Snapshot, groups []Group) (*Snapshot, error) {
	encoded, err := json.Marshal(struct {
		Groups     []Group
		Themes     map[string]ThemeSet
		Colors     []ColorTheme
		ColorTheme string
	}{Groups: groups, Themes: s.themes, Colors: s.colorThemes, ColorTheme: s.colorTheme})
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(encoded)
	s.revision = hex.EncodeToString(hash[:])
	sortFields := []map[string]string{}
	for _, g := range groups {
		if !g.Enabled("Sort") {
			continue
		}
		fields := map[string]string{}
		for _, key := range []string{"Mask", "MaskSyntax", "Case", "IncludeAttributes", "ExcludeAttributes", "SizeAbove", "SizeBelow", "DateType", "DateAfter", "DateBefore", "DateRelative", "Sort.Number"} {
			fields[key] = g.Fields[key]
		}
		sortFields = append(sortFields, fields)
	}
	encoded, _ = json.Marshal(sortFields)
	hash = sha256.Sum256(encoded)
	s.sortRevision = hex.EncodeToString(hash[:])
	return s, nil
}

// ForIconTheme selects universal groups first, followed by this theme's set.
// It shares immutable compiled predicates and icon references. Repeated theme
// switches reuse the same view; they never compile masks or read configuration.
func (s *Snapshot) ForIconTheme(theme string) *Snapshot {
	if s == nil {
		return nil
	}
	if s.source != nil {
		s = s.source
	}
	if theme == "" {
		theme = "generic"
	}
	s.viewsMu.Lock()
	defer s.viewsMu.Unlock()
	if view := s.views[theme]; view != nil {
		return view
	}
	view := &Snapshot{source: s, themes: s.themes, iconTheme: theme, colorThemes: s.colorThemes, colorTheme: s.colorTheme}
	groups := []Group{}
	for _, scope := range []string{"", theme} {
		for _, compiled := range s.groups {
			if compiled.group.IconTheme() == scope {
				view.groups = append(view.groups, compiled)
				groups = append(groups, compiled.group)
			}
		}
	}
	view, _ = finishSnapshot(view, groups)
	// Even identical rule sets can resolve per-theme icon references differently.
	view.revision += ":" + theme
	if s.views == nil {
		s.views = map[string]*Snapshot{}
	}
	s.views[theme] = view
	return view
}

// IconTint reports an explicit theme policy; absent metadata keeps old clients'
// existing artwork defaults. The immutable view owns the returned value.
func (s *Snapshot) IconTint() (bool, bool) {
	if s == nil {
		return false, false
	}
	theme, ok := s.themes[s.iconTheme]
	if !ok {
		return false, false
	}
	return theme.TintIcons(), true
}

// Predicate returns the already compiled immutable condition for a projection.
func (s *Snapshot) Predicate(id string) *Predicate {
	if s != nil {
		for _, compiled := range s.groups {
			if compiled.group.ID == id {
				return compiled.predicate
			}
		}
	}
	return nil
}

func (s *Snapshot) SortRevision() string {
	if s == nil {
		return ""
	}
	return s.sortRevision
}

func (s *Snapshot) Revision() string {
	if s == nil {
		return ""
	}
	return s.revision
}
func (s *Snapshot) Groups() []Group {
	groups := []Group{}
	if s != nil {
		for _, g := range s.groups {
			groups = append(groups, g.group.Clone())
		}
	}
	return groups
}

// Matching returns only enabled facets, so unrelated matches cannot block them.
func (s *Snapshot) Matching(item *vfs.VFSItem, known bool, facet string, now time.Time) []Group {
	result := []Group{}
	if s == nil {
		return result
	}
	for _, g := range s.groups {
		enabled := g.group.Enabled(facet)
		if facet == "Icons" {
			enabled = g.iconsEnabled()
		}
		if enabled && g.predicate.Match(item, known, now) {
			result = append(result, g.group.Clone())
		}
	}
	return result
}
