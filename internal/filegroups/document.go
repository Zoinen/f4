package filegroups

import (
	"bufio"
	"bytes"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
)

// Document preserves source lines, including comments and unknown sections.
// All values in v2/v3 Group/Command sections use Go quoted-string syntax. Legacy
// documents are deliberately not unquoted, so Windows backslashes stay literal.
type Document struct {
	lines    []string
	sections map[string]map[string]string
	version  int
}

func Parse(data []byte) (*Document, error) {
	d := &Document{sections: map[string]map[string]string{}}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 65536), 10*1024*1024)
	section := ""
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		d.lines = append(d.lines, line)
		s := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, ";") {
			continue
		}
		if strings.HasPrefix(s, "[") {
			if !strings.HasSuffix(s, "]") {
				return nil, fmt.Errorf("invalid INI section at line %d", len(d.lines))
			}
			section = s[1 : len(s)-1]
			if _, exists := d.sections[section]; exists {
				return nil, fmt.Errorf("duplicate section %q", section)
			}
			d.sections[section] = map[string]string{}
			continue
		}
		key, value, ok := strings.Cut(s, "=")
		if !ok || section == "" {
			return nil, fmt.Errorf("invalid INI entry at line %d", len(d.lines))
		}
		d.sections[section][strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read file groups: %w", err)
	}
	if raw := d.sections["FileGroups"]["Version"]; raw != "" {
		version, err := strconv.Atoi(raw)
		if err != nil || (version != 2 && version != 3) {
			return nil, fmt.Errorf("unsupported file groups version %q", raw)
		}
		d.version = version
	}
	if d.Unified() {
		for section, fields := range d.sections {
			if !ownedSection(section) {
				continue
			}
			for key, value := range fields {
				if strings.HasPrefix(value, `"`) {
					decoded, err := strconv.Unquote(value)
					if err != nil {
						return nil, fmt.Errorf("%s.%s: %w", section, key, err)
					}
					fields[key] = decoded
				}
			}
		}
	}
	return d, nil
}

func ownedSection(s string) bool {
	return strings.HasPrefix(s, "Group.") || strings.HasPrefix(s, "Command.") || strings.HasPrefix(s, "FileGroupTheme.") || strings.HasPrefix(s, "FileColorTheme.")
}

func (d *Document) Unified() bool { return d != nil && (d.version == 2 || d.version == 3) }

// Section returns a copy, avoiding accidental mutation of the loaded baseline.
func (d *Document) Section(name string) map[string]string { return maps.Clone(d.sections[name]) }

func (d *Document) SectionNames() []string {
	names := make([]string, 0, len(d.sections))
	for name := range d.sections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (d *Document) Groups() ([]Group, error) {
	groups := []Group{}
	if !d.Unified() {
		return groups, nil
	}
	for _, section := range d.SectionNames() {
		if !strings.HasPrefix(section, "Group.") {
			continue
		}
		id := strings.TrimPrefix(section, "Group.")
		if err := validID(id); err != nil {
			return nil, err
		}
		g := Group{ID: id, Fields: d.Section(section)}
		for _, commandSection := range d.SectionNames() {
			if !strings.HasPrefix(commandSection, "Command."+id+"/") {
				continue
			}
			fields := d.Section(commandSection)
			c := commandFromValues(strings.TrimPrefix(commandSection, "Command."+id+"/"), fields)
			c.Overrides = maps.Clone(fields)
			if err := validID(c.ID); err != nil {
				return nil, err
			}
			g.Commands = append(g.Commands, c)
		}
		g.Commands = orderedCommands(g.Commands, g.Fields["CommandOrder"])
		if _, present := g.Fields["CommandOrder"]; present && g.Commands == nil {
			g.Commands = []CommandSet{}
		}
		groups = append(groups, g)
	}
	return orderGroups(groups, d.sections["FileGroups"]["Order"]), nil
}

func orderGroups(groups []Group, order string) []Group {
	rank := map[string]int{}
	for i, id := range strings.Split(order, ",") {
		rank[strings.TrimSpace(id)] = i + 1
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := rank[groups[i].ID], rank[groups[j].ID]
		if a == 0 {
			return false
		}
		if b == 0 {
			return true
		}
		return a < b
	})
	return groups
}

func orderedCommands(commands []CommandSet, order string) []CommandSet {
	rank := map[string]int{}
	for i, id := range strings.Split(order, ",") {
		rank[strings.TrimSpace(id)] = i + 1
	}
	sort.SliceStable(commands, func(i, j int) bool {
		a, b := rank[commands[i].ID], rank[commands[j].ID]
		if a == 0 {
			return false
		}
		if b == 0 {
			return true
		}
		return a < b
	})
	return commands
}

// encodeSnapshot marks a complete working set without mutating the transaction's
// source document. Failed publication must leave the old sparse baseline intact.
func (d *Document) encodeSnapshot(groups []Group, themes []ThemeSet) ([]byte, error) {
	copy := *d
	copy.sections = maps.Clone(d.sections)
	meta := d.Section("FileGroups")
	if meta == nil {
		meta = map[string]string{}
	}
	meta["Mode"] = "snapshot"
	copy.sections["FileGroups"] = meta
	return copy.Encode(groups, themes)
}

func (d *Document) encodeSnapshotColors(groups []Group, themes []ThemeSet, colors []ColorTheme, selected string) ([]byte, error) {
	if err := ValidateColorThemes(colors); err != nil {
		return nil, err
	}
	if selected != "" {
		if err := validID(selected); err != nil {
			return nil, err
		}
	}
	copy := *d
	copy.sections = maps.Clone(d.sections)
	for section := range copy.sections {
		if strings.HasPrefix(section, "FileColorTheme.") {
			delete(copy.sections, section)
		}
	}
	ids := make([]string, 0, len(colors))
	for _, theme := range colors {
		copy.sections["FileColorTheme."+theme.ID] = maps.Clone(theme.Fields)
		ids = append(ids, theme.ID)
	}
	meta := d.Section("FileGroups")
	if meta == nil {
		meta = map[string]string{}
	}
	meta["ColorTheme"] = selected
	meta["ColorThemeOrder"] = strings.Join(ids, ",")
	copy.sections["FileGroups"] = meta
	return copy.encodeSnapshot(groups, themes)
}

// Encode replaces owned records while preserving untouched source lines and
// unknown keys. Omitted records are removed; omitted keys revert to inheritance.
func (d *Document) Encode(groups []Group, themeSets ...[]ThemeSet) ([]byte, error) {
	sections := map[string]map[string]string{}
	colors, err := d.ColorThemes()
	if err != nil {
		return nil, err
	}
	for _, theme := range colors {
		sections["FileColorTheme."+theme.ID] = maps.Clone(theme.Fields)
	}
	themes, err := d.Themes()
	if err != nil {
		return nil, err
	}
	if len(themeSets) > 0 {
		themes = themeSets[0]
	}
	if err := ValidateThemes(themes); err != nil {
		return nil, err
	}
	for _, t := range themes {
		sections["FileGroupTheme."+t.ID] = maps.Clone(t.Fields)
	}
	ids := []string{}
	for _, g := range groups {
		if err := validID(g.ID); err != nil {
			return nil, err
		}
		name := "Group." + g.ID
		if _, exists := sections[name]; exists {
			return nil, fmt.Errorf("duplicate group %q", g.ID)
		}
		fields := maps.Clone(g.Fields)
		if fields == nil {
			fields = map[string]string{}
		}
		commandIDs := []string{}
		for _, c := range g.Commands {
			if err := validID(c.ID); err != nil {
				return nil, err
			}
			key := "Command." + g.ID + "/" + c.ID
			if _, exists := sections[key]; exists {
				return nil, fmt.Errorf("duplicate command %q", c.ID)
			}
			values := commandValues(c)
			if c.Overrides != nil {
				values = maps.Clone(c.Overrides)
			}
			sections[key] = values
			commandIDs = append(commandIDs, c.ID)
		}
		if g.Commands != nil {
			fields["CommandOrder"] = strings.Join(commandIDs, ",")
		} else {
			delete(fields, "CommandOrder")
		}
		sections[name] = fields
		ids = append(ids, g.ID)
	}
	meta := d.Section("FileGroups")
	if meta == nil {
		meta = map[string]string{}
	}
	meta["Version"] = "3"
	meta["Order"] = strings.Join(ids, ",")
	sections["FileGroups"] = meta
	var out strings.Builder
	written := map[string]bool{}
	section := ""
	seenKeys := map[string]bool{}
	flush := func() {
		values, ok := sections[section]
		if !ok {
			return
		}
		keys := []string{}
		for k := range values {
			if !seenKeys[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&out, "%s = %s\n", k, encodeValue(section, k, values[k]))
		}
		written[section] = true
	}
	for _, line := range d.lines {
		s := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			flush()
			section = s[1 : len(s)-1]
			seenKeys = map[string]bool{}
			if ownedSection(section) {
				if _, ok := sections[section]; !ok {
					continue
				}
			}
			out.WriteString(line + "\n")
			continue
		}
		if ownedSection(section) {
			if _, ok := sections[section]; !ok {
				if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, ";") {
					out.WriteString(line + "\n")
				}
				continue
			}
		}
		if values, ok := sections[section]; ok && s != "" && !strings.HasPrefix(s, "#") && !strings.HasPrefix(s, ";") {
			if k, _, ok := strings.Cut(s, "="); ok {
				k = strings.TrimSpace(k)
				if value, exists := values[k]; exists {
					fmt.Fprintf(&out, "%s = %s\n", k, encodeValue(section, k, value))
					seenKeys[k] = true
				}
				continue
			}
		}
		out.WriteString(line + "\n")
	}
	flush()
	names := []string{}
	for name := range sections {
		if !written[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&out, "\n[%s]\n", name)
		keys := []string{}
		for k := range sections[name] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&out, "%s = %s\n", k, encodeValue(name, k, sections[name][k]))
		}
	}
	return []byte(out.String()), nil
}

func encodeValue(section, key, value string) string {
	if section == "FileGroups" {
		return value
	}
	return strconv.Quote(value)
}

// Merge overlays fields by identity. A missing field inherits, whereas an
// explicit empty value clears. User order applies without freezing new defaults.
func Merge(defaults, overrides []Group) []Group {
	byID := map[string]Group{}
	order := []string{}
	for _, g := range defaults {
		byID[g.ID] = g.Clone()
		order = append(order, g.ID)
	}
	for _, g := range overrides {
		base, ok := byID[g.ID]
		if !ok {
			base = Group{ID: g.ID, Fields: map[string]string{}}
			order = append(order, g.ID)
		}
		if base.Fields == nil {
			base.Fields = map[string]string{}
		}
		maps.Copy(base.Fields, g.Fields)
		base.Commands = mergeCommands(base.Commands, g.Clone().Commands)
		byID[g.ID] = base
	}
	result := []Group{}
	for _, id := range order {
		result = append(result, byID[id])
	}
	ids := []string{}
	for _, g := range overrides {
		ids = append(ids, g.ID)
	}
	return orderGroups(result, strings.Join(ids, ","))
}
