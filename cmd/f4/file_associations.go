package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// AssocKind numbers the six command slots per far2l's filetype.hpp
// (FILETYPE_EXEC, FILETYPE_ALTEXEC, …). Keeping the same order lets a
// far2l associations.ini be dropped into f4 (and vice versa) without
// remapping the State bitmask.
type AssocKind int

const (
	AssocExecute AssocKind = 0 // Enter
	AssocAltExec AssocKind = 1 // Ctrl+PgDn (reserved; not wired yet)
	AssocView    AssocKind = 2 // F3
	AssocAltView AssocKind = 3 // Alt+F3 (reserved; not wired yet)
	AssocEdit    AssocKind = 4 // F4
	AssocAltEdit AssocKind = 5 // Alt+F4 (reserved; not wired yet)
)

const assocKindCount = 6

// assocKeyName maps AssocKind to the INI key far2l uses for the slot.
var assocKeyName = [assocKindCount]string{
	"Execute",
	"AltExec",
	"View",
	"AltView",
	"Edit",
	"AltEdit",
}

// FileAssoc mirrors far2l's FileTypeStrings so associations.ini is
// interchangeable between the two applications. Enabled[N] carries the
// far2l State bit for slot N; a disabled slot stays in the file but is
// skipped during dispatch.
type FileAssoc struct {
	Mask        string
	Description string
	Commands    [assocKindCount]string
	Enabled     [assocKindCount]bool
}

// associationsRoot is the INI section prefix used by far2l. Do not
// rename — files must round-trip byte-identical with far2l.
const associationsRoot = "Associations"

// associationsFilePathFn is the resolver used by AssociationsFilePath.
// Tests overwrite it to redirect load/save to a temp file so they can
// exercise the full dispatch path without touching the user's config.
var associationsFilePathFn = defaultAssociationsFilePath

// AssociationsFilePath returns the user-config location. Follows the
// far2l naming so the same file can live under either app's settings.
func AssociationsFilePath() string { return associationsFilePathFn() }

func defaultAssociationsFilePath() string {
	// In portable mode (UseSystemProfiles=0) write under <exeDir>/Profile;
	// otherwise use the system %AppData%/f4/settings path as before. Read
	// userConfigDir live in non-portable mode so tests overriding the seam
	// keep working.
	if IsPortableProfile() {
		return filepath.Join(GetF4ConfigDir(), "settings", "associations.ini")
	}
	configDir, _ := userConfigDir()
	return filepath.Join(configDir, "f4", "settings", "associations.ini")
}

// LoadAssociations parses an associations.ini. A missing file is not
// an error — an empty slice is returned. Sections outside the
// Associations/ subtree are ignored so the loader stays safe when
// pointed at a mixed-content INI.
func LoadAssociations(path string) ([]FileAssoc, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []FileAssoc{}, nil
		}
		return nil, err
	}
	defer f.Close()

	sections := map[string]map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var cur map[string]string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			name := trimmed[1 : len(trimmed)-1]
			if name == associationsRoot || strings.HasPrefix(name, associationsRoot+"/") {
				if _, ok := sections[name]; !ok {
					sections[name] = map[string]string{}
				}
				cur = sections[name]
			} else {
				cur = nil
			}
			continue
		}
		if cur == nil {
			continue
		}
		if eq := strings.IndexByte(line, '='); eq != -1 {
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			if key != "" {
				cur[key] = val
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	list := []FileAssoc{}
	for i := 0; ; i++ {
		sec, ok := sections[fmt.Sprintf("%s/Type%d", associationsRoot, i)]
		if !ok {
			break
		}
		if sec["Mask"] == "" {
			// Empty mask terminates in far2l too.
			break
		}
		a := FileAssoc{
			Mask:        sec["Mask"],
			Description: sec["Description"],
		}
		state := parseAssocState(sec["State"])
		for k := 0; k < assocKindCount; k++ {
			a.Commands[k] = sec[assocKeyName[k]]
			a.Enabled[k] = state&(1<<uint(k)) != 0
		}
		list = append(list, a)
	}
	return list, nil
}

// SaveAssociations writes the list atomically (tmp+rename). Parent
// directories are created as needed. Keys are written in the same
// order far2l uses so file-level diffs stay small.
func SaveAssociations(path string, list []FileAssoc) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	var buf strings.Builder
	for i, a := range list {
		if i > 0 {
			buf.WriteByte('\n')
		}
		fmt.Fprintf(&buf, "[%s/Type%d]\n", associationsRoot, i)
		buf.WriteString("Mask=")
		buf.WriteString(a.Mask)
		buf.WriteByte('\n')
		buf.WriteString("Description=")
		buf.WriteString(a.Description)
		buf.WriteByte('\n')
		for k := 0; k < assocKindCount; k++ {
			buf.WriteString(assocKeyName[k])
			buf.WriteByte('=')
			buf.WriteString(a.Commands[k])
			buf.WriteByte('\n')
		}
		fmt.Fprintf(&buf, "State=%d\n", encodeAssocState(a.Enabled))
	}

	return writeFileAtomically(path, []byte(buf.String()), 0o600)
}

func parseAssocState(v string) uint32 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}

func encodeAssocState(enabled [assocKindCount]bool) uint32 {
	var m uint32
	for i, on := range enabled {
		if on {
			m |= 1 << uint(i)
		}
	}
	return m
}

// MatchMask reports whether name satisfies the given far2l-style mask.
//
// Syntax mirrors far2l's CFileMask:
//   - "," and ";" separate include globs (OR).
//   - "|" splits include vs. exclude; matches on the exclude side veto.
//   - A section wrapped in "/…/" is a regular expression.
//
// Matching honours ignoreCase for both globs and regex (via (?i)).
// A mask that fails to parse (bad regex, empty include list) never
// matches — safer than silently degrading to "matches everything".
func MatchMask(name, mask string, ignoreCase bool) bool {
	mask = strings.TrimSpace(mask)
	if mask == "" || name == "" {
		return false
	}
	includes, excludes := splitMaskIncludeExclude(mask)
	if len(includes) == 0 {
		return false
	}
	matchAny := func(list []string) bool {
		for _, m := range list {
			if m = strings.TrimSpace(m); m == "" {
				continue
			}
			if matchOneMask(name, m, ignoreCase) {
				return true
			}
		}
		return false
	}
	if !matchAny(includes) {
		return false
	}
	if matchAny(excludes) {
		return false
	}
	return true
}

// splitMaskIncludeExclude divides on the far2l "|" separator and then
// splits each side on "," / ";" into individual masks. A regex section
// (`/…/`) is preserved as a single mask so its commas stay literal.
func splitMaskIncludeExclude(mask string) (includes, excludes []string) {
	inc, exc, hasExc := splitOnPipeSkipRegex(mask)
	includes = splitMaskCommaSemi(inc)
	if hasExc {
		excludes = splitMaskCommaSemi(exc)
	}
	return includes, excludes
}

// splitOnPipeSkipRegex splits mask on the first '|' that is not inside
// a /regex/ block. If no '|' occurs, hasExc is false.
func splitOnPipeSkipRegex(mask string) (inc, exc string, hasExc bool) {
	depth := 0 // "inside /…/" tracker
	for i := 0; i < len(mask); i++ {
		c := mask[i]
		switch c {
		case '/':
			// Toggle regex mode on unescaped slashes. Rough but matches
			// far2l behaviour: a lone '/' outside a regex is treated as
			// part of the glob (unusual but permitted).
			if depth == 0 {
				depth = 1
			} else {
				depth = 0
			}
		case '|':
			if depth == 0 {
				return mask[:i], mask[i+1:], true
			}
		}
	}
	return mask, "", false
}

// splitMaskCommaSemi splits a mask side on ',' / ';' while keeping
// regex sections (/…/) intact.
func splitMaskCommaSemi(side string) []string {
	if side == "" {
		return nil
	}
	var out []string
	var cur strings.Builder
	depth := 0
	flush := func() {
		s := strings.TrimSpace(cur.String())
		cur.Reset()
		if s != "" {
			out = append(out, s)
		}
	}
	for i := 0; i < len(side); i++ {
		c := side[i]
		switch c {
		case '/':
			if depth == 0 {
				depth = 1
			} else {
				depth = 0
			}
			cur.WriteByte(c)
		case ',', ';':
			if depth == 0 {
				flush()
			} else {
				cur.WriteByte(c)
			}
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

// matchOneMask handles a single mask: '/regex/' or a glob. Glob "*.*"
// is normalised to "*" so it matches extension-less names too, matching
// far2l's long-standing quirk.
func matchOneMask(name, mask string, ignoreCase bool) bool {
	if len(mask) >= 2 && mask[0] == '/' && mask[len(mask)-1] == '/' {
		pattern := mask[1 : len(mask)-1]
		if ignoreCase {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(name)
	}
	// Star-dot-star means "match everything".
	if mask == "*.*" {
		mask = "*"
	}
	m, n := mask, name
	if ignoreCase {
		m = strings.ToLower(m)
		n = strings.ToLower(n)
	}
	ok, err := filepath.Match(m, n)
	if err != nil {
		return false
	}
	return ok
}

// MatchingAssociations returns the associations that fire for name in
// the given slot. Result order preserves list order — that is what the
// picker will show. On Windows the mask match is case-insensitive by
// default; elsewhere it honours the case of the underlying filesystem
// (best-effort: we default to case-sensitive on non-Windows too, as
// far2l does).
func MatchingAssociations(list []FileAssoc, name string, kind AssocKind) []FileAssoc {
	if name == "" || int(kind) < 0 || int(kind) >= assocKindCount {
		return nil
	}
	ignoreCase := runtime.GOOS == "windows"
	var out []FileAssoc
	for _, a := range list {
		if !a.Enabled[kind] || a.Commands[kind] == "" {
			continue
		}
		if MatchMask(name, a.Mask, ignoreCase) {
			out = append(out, a)
		}
	}
	return out
}
