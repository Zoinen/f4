package netfox

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/unxed/vtui"
)

// sshProfile is the part of an OpenSSH Host entry that NetFox can apply to a
// native Go SSH connection. The parser deliberately keeps this type separate
// from NetFoxConfig: it is a read-only view of ~/.ssh/config, not another
// persisted connection format.
type sshProfile struct {
	Name          string
	Host          string
	Port          string
	User          string
	KeyPath       string
	Timeout       string
	IdentityFiles []string
	home          string
}

type sshConfigOption struct {
	name   string
	values []string
}

type sshConfigBlock struct {
	patterns []string
	options  []sshConfigOption
}

type sshConfigParser struct {
	home    string
	blocks  []sshConfigBlock
	aliases map[string]struct{}
	visited map[string]struct{}
}

// loadSSHProfiles reads the user's OpenSSH config and returns only explicitly
// named Host aliases. Wildcard-only blocks still contribute defaults to named
// aliases, but are not themselves displayed as NetFox connections.
func loadSSHProfiles(home string) (map[string]sshProfile, error) {
	profiles := make(map[string]sshProfile)
	if home == "" {
		return profiles, nil
	}

	configPath := filepath.Join(home, ".ssh", "config")
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return profiles, nil
		}
		return nil, fmt.Errorf("inspect SSH config %s: %w", configPath, err)
	}

	parser := &sshConfigParser{
		home:    home,
		aliases: make(map[string]struct{}),
		visited: make(map[string]struct{}),
	}
	if err := parser.parseFile(configPath); err != nil {
		return nil, err
	}

	aliases := make([]string, 0, len(parser.aliases))
	for alias := range parser.aliases {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		profile := parser.profile(alias)
		if profile.Host == "" {
			continue
		}
		profiles[alias] = profile
	}
	return profiles, nil
}

func (p *sshConfigParser) parseFile(name string) error {
	name = filepath.Clean(name)
	if _, ok := p.visited[name]; ok {
		return nil
	}
	p.visited[name] = struct{}{}

	file, err := os.Open(name)
	if err != nil {
		return fmt.Errorf("read SSH config %s: %w", name, err)
	}
	defer func() { _ = file.Close() }()

	current := &sshConfigBlock{patterns: []string{"*"}}
	inMatch := false
	flush := func() {
		if current == nil {
			return
		}
		if len(current.options) > 0 && len(current.patterns) > 0 {
			p.blocks = append(p.blocks, *current)
		}
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1024*1024)
	var continued string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasSuffix(line, "\\") {
			continued += strings.TrimSuffix(line, "\\") + " "
			continue
		}
		line = continued + line
		continued = ""
		fields := splitSSHFields(stripSSHComment(line))
		if len(fields) == 0 {
			continue
		}

		key := strings.ToLower(fields[0])
		switch key {
		case "host":
			flush()
			current = &sshConfigBlock{patterns: append([]string(nil), fields[1:]...)}
			for _, pattern := range current.patterns {
				if isSSHAlias(pattern) {
					p.aliases[pattern] = struct{}{}
				}
			}
			inMatch = false
			continue
		case "match":
			// Match criteria depend on the live connection context. NetFox
			// cannot reproduce all of OpenSSH's Match predicates, so it does
			// not apply any options in a Match block rather than guessing.
			flush()
			current = nil
			inMatch = true
			continue
		case "include":
			if inMatch {
				continue
			}
			var patterns []string
			if current != nil {
				patterns = append(patterns, current.patterns...)
				flush()
				current = &sshConfigBlock{patterns: patterns}
			}
			for _, pattern := range fields[1:] {
				for _, included := range expandSSHInclude(pattern, filepath.Dir(name), p.home) {
					if err := p.parseFile(included); err != nil {
						return err
					}
				}
			}
			continue
		}

		if inMatch || current == nil || len(fields) < 2 {
			continue
		}
		current.options = append(current.options, sshConfigOption{
			name:   key,
			values: append([]string(nil), fields[1:]...),
		})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read SSH config %s: %w", name, err)
	}
	if continued != "" {
		fields := splitSSHFields(stripSSHComment(continued))
		if len(fields) >= 2 && !inMatch && current != nil {
			current.options = append(current.options, sshConfigOption{
				name:   strings.ToLower(fields[0]),
				values: append([]string(nil), fields[1:]...),
			})
		}
	}
	flush()
	return nil
}

func (p *sshConfigParser) profile(alias string) sshProfile {
	profile := sshProfile{Name: alias, Port: "22", User: localSSHUser(), home: p.home}
	var identityFiles []string
	var hostSet, portSet, userSet, timeoutSet bool
	for _, block := range p.blocks {
		if !sshHostPatternsMatch(block.patterns, alias) {
			continue
		}
		for _, option := range block.options {
			value := option.values[0]
			switch option.name {
			case "hostname":
				if !hostSet {
					profile.Host = expandSSHValue(value, alias, profile)
					hostSet = true
				}
			case "port":
				if !portSet {
					profile.Port = expandSSHValue(value, alias, profile)
					portSet = true
				}
			case "user":
				if !userSet {
					profile.User = expandSSHValue(value, alias, profile)
					userSet = true
				}
			case "identityfile":
				for _, item := range option.values {
					item = expandSSHValue(item, alias, profile)
					if item != "none" {
						identityFiles = append(identityFiles, item)
					}
				}
			case "connecttimeout":
				if !timeoutSet {
					if seconds, err := strconv.Atoi(expandSSHValue(value, alias, profile)); err == nil && seconds > 0 {
						profile.Timeout = strconv.Itoa(seconds)
						timeoutSet = true
					}
				}
			}
		}
	}
	if profile.Host == "" {
		profile.Host = alias
	}
	profile.IdentityFiles = uniqueSSHPaths(identityFiles)
	for _, item := range profile.IdentityFiles {
		if _, err := os.Stat(item); err == nil {
			profile.KeyPath = item
			break
		}
	}
	if profile.KeyPath == "" && len(profile.IdentityFiles) > 0 {
		profile.KeyPath = profile.IdentityFiles[0]
	}
	return profile
}

func sshProfileConfig(profile sshProfile) NetFoxConfig {
	return NetFoxConfig{
		Type:           "sftp",
		Host:           profile.Host,
		Port:           profile.Port,
		User:           profile.User,
		KeyPath:        profile.KeyPath,
		Timeout:        profile.Timeout,
		Codepage:       "65001",
		autoSSHProfile: true,
	}
}

func mergeSSHProfile(name string, cfg NetFoxConfig, profile sshProfile) NetFoxConfig {
	if cfg.Type == "" {
		cfg.Type = "sftp"
	}
	if cfg.Host == "" || cfg.Host == name {
		cfg.Host = profile.Host
	}
	if cfg.Port == "" {
		cfg.Port = profile.Port
	}
	if cfg.User == "" {
		cfg.User = profile.User
	}
	if cfg.KeyPath == "" {
		cfg.KeyPath = profile.KeyPath
	}
	if cfg.Timeout == "" {
		cfg.Timeout = profile.Timeout
	}
	if cfg.Codepage == "" {
		cfg.Codepage = "65001"
	}
	return cfg
}

func localSSHUser() string {
	if current, err := user.Current(); err == nil && current != nil && current.Username != "" {
		return current.Username
	}
	return os.Getenv("USER")
}

func expandSSHInclude(pattern, baseDir, home string) []string {
	pattern = expandSSHHome(pattern, home)
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(baseDir, pattern)
	}
	matches, err := filepath.Glob(filepath.Clean(pattern))
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	return matches
}

func expandSSHValue(value, alias string, profile sshProfile) string {
	value = expandSSHHome(value, profile.home)
	userName := profile.User
	if userName == "" {
		userName = localSSHUser()
	}
	port := profile.Port
	if port == "" {
		port = "22"
	}
	replacer := strings.NewReplacer(
		"%%", "%",
		"%d", profile.home,
		"%h", alias,
		"%p", port,
		"%r", userName,
		"%u", localSSHUser(),
	)
	return expandSSHHome(replacer.Replace(value), profile.home)
}

func homeForSSHValue() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func expandSSHHome(value, home string) string {
	if home == "" {
		home = homeForSSHValue()
	}
	if value == "~" {
		return home
	}
	if strings.HasPrefix(value, "~/") || strings.HasPrefix(value, "~\\") {
		return filepath.Join(home, value[2:])
	}
	return value
}

func uniqueSSHPaths(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = filepath.Clean(value)
		if value == "." || value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isSSHAlias(pattern string) bool {
	return pattern != "" && !strings.HasPrefix(pattern, "!") &&
		!strings.ContainsAny(pattern, "*?[")
}

func sshHostPatternsMatch(patterns []string, alias string) bool {
	matched := false
	positive := false
	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		if negated {
			pattern = strings.TrimPrefix(pattern, "!")
		} else {
			positive = true
		}
		if !sshPatternMatch(pattern, alias) {
			continue
		}
		if negated {
			return false
		}
		matched = true
	}
	return positive && matched
}

func sshPatternMatch(pattern, value string) bool {
	if strings.EqualFold(pattern, value) {
		return true
	}
	matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(value))
	return err == nil && matched
}

func stripSSHComment(line string) string {
	var quote rune
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '#' && (i == 0 || unicode.IsSpace(rune(line[i-1]))) {
			return line[:i]
		}
	}
	return line
}

func splitSSHFields(line string) []string {
	var fields []string
	var field strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if field.Len() > 0 {
			fields = append(fields, field.String())
			field.Reset()
		}
	}
	for _, r := range line {
		if escaped {
			field.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				field.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		field.WriteRune(r)
	}
	if escaped {
		field.WriteRune('\\')
	}
	flush()
	return fields
}

func logSSHProfileLoad(profileCount int, path string) {
	vtui.DebugLog("[FIX:netfox-ssh-config] loaded %d SSH profiles from %s", profileCount, path)
}
