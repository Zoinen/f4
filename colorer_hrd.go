package main

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/unxed/vtui"
)

// ColorerScheme describes a color style ("hrd" file) declared in catalog.xml.
type ColorerScheme struct {
	Name        string
	Description string
	Path        string
}

type colorerRegionStyle struct {
	fore     uint32
	back     uint32
	style    uint32
	hasFore  bool
	hasBack  bool
	hasStyle bool
}

// The style bits an hrd assign carries, as StyledRegion defines them. Colorer
// reads the attribute as a hex number and treats it as a mask.
const (
	colorerStyleBold      = 1
	colorerStyleItalic    = 2
	colorerStyleUnderline = 4
	colorerStyleStrikeout = 8
)

// maxColorerRegionDepth bounds the walk up the region parents, so that a
// schema declaring a cycle cannot spin the resolver forever.
const maxColorerRegionDepth = 32

// colorerCharsetReader keeps the decoder going for the schema files that
// declare a legacy encoding: a fair number of them are windows-1251, and Go
// refuses to decode those without a reader, which used to leave the style
// empty without a word. Everything read out of them is ASCII.
func colorerCharsetReader(_ string, input io.Reader) (io.Reader, error) {
	return input, nil
}

var (
	schemeMu         sync.Mutex
	schemeName       string
	schemeStyles     map[string]colorerRegionStyle
	schemeKeys       []string
	schemeMemo       map[string]colorerRegionStyle
	schemeGeneration uint64
)

// ColorerConfigsDir returns the directory the Colorer schemas are read from.
// A folder configured by the user wins over the downloaded copy, so that the
// schemas of an existing far2l installation can be used instead.
func ColorerConfigsDir() string {
	if custom := strings.TrimSpace(AppConfig.EditorColorerCatalog); custom != "" {
		return custom
	}
	return filepath.Join(GetF4ConfigDir(), "colorer", "configs")
}

// ListColorerSchemes returns the rgb color styles shipped with the schemas.
// Console styles are skipped: they carry palette indices, not colors.
func ListColorerSchemes() []ColorerScheme {
	return parseColorerCatalog(filepath.Join(ColorerConfigsDir(), "base", "catalog.xml"))
}

// maxColorerCatalogFiles bounds the entity graph, so a catalog referring to
// itself cannot spin the scanner forever.
const maxColorerCatalogFiles = 32

// catalogEntityRe matches an external entity declaration inside the DOCTYPE
// internal subset, e.g.
// <!ENTITY hrd-sets SYSTEM "env:$FAR_HOME/hrd/catalog-console.xml">.
var catalogEntityRe = regexp.MustCompile(`(?is)<!ENTITY\s+(?:%\s+)?[^\s>]+\s+(?:SYSTEM|PUBLIC)\s+((?:"[^"]*"|'[^']*')(?:\s+(?:"[^"]*"|'[^']*'))?)`)

// catalogQuotedRe splits an entity declaration into its quoted parts. For the
// PUBLIC form the system id is the last one.
var catalogQuotedRe = regexp.MustCompile(`"[^"]*"|'[^']*'`)

// entityDeclRe matches both internal and external entity declarations inside the DOCTYPE,
// capturing the entity name and its string literal value (which may be double or single-quoted).
var entityDeclRe = regexp.MustCompile(`(?is)<!ENTITY\s+(?:%\s+)?([^\s>]+)\s+(?:SYSTEM\s+|PUBLIC\s+(?:"[^"]*"|'[^']*')\s+)?(?:"([^"]*)"|'([^']*)')`)

// parseDirectiveEntities parses any entity declarations from the given XML directive string
// and adds them to the provided map.
func parseDirectiveEntities(directive string, entitiesMap map[string]string) {
	for _, match := range entityDeclRe.FindAllStringSubmatch(directive, -1) {
		name := match[1]
		val := match[2]
		if val == "" && match[3] != "" {
			val = match[3]
		}
		entitiesMap[name] = val
	}
}

// catalogEnvRe matches the environment references colorer expands in entity
// paths: $NAME and ${NAME}, plus the Windows %NAME% form.
var catalogEnvRe = regexp.MustCompile(`\$\{([A-Za-z]\w*)\}|\$([A-Za-z]\w*)|%([A-Za-z]\w*)%`)

// parseColorerCatalog collects the rgb color styles reachable from catalog.xml.
// FarColorer catalogs keep the <hrd-sets> block in a separate file pulled in
// with an external XML entity, and encoding/xml never expands those, so the
// entity declarations are followed by hand.
func parseColorerCatalog(catalogPath string) []ColorerScheme {
	var schemes []ColorerScheme
	seenFiles := make(map[string]bool)
	seenNames := make(map[string]bool)

	entitiesMap := make(map[string]string)

	queue := []string{catalogPath}
	for len(queue) > 0 && len(seenFiles) < maxColorerCatalogFiles {
		path := queue[0]
		queue = queue[1:]

		key := filepath.Clean(path)
		if abs, err := filepath.Abs(path); err == nil {
			key = filepath.Clean(abs)
		}
		if seenFiles[key] {
			continue
		}
		seenFiles[key] = true

		found, entities := scanColorerCatalogFile(path, catalogPath, entitiesMap)
		for _, scheme := range found {
			lower := strings.ToLower(scheme.Name)
			if seenNames[lower] {
				continue
			}
			seenNames[lower] = true
			schemes = append(schemes, scheme)
		}
		for _, entity := range entities {
			if resolved := resolveColorerCatalogEntity(entity, path, catalogPath); resolved != "" {
				queue = append(queue, resolved)
			}
		}
	}

	// The declaration order of the catalog is kept, the way FarColorer lists
	// the styles: the catalog groups them deliberately, and sorting by the
	// machine name scattered that grouping.
	return schemes
}

// scanColorerCatalogFile reads a single catalog file and returns the rgb
// styles it declares together with the system ids of the external entities it
// references. Style locations are resolved against the main catalog, the way
// the colorer engine itself does it.
func scanColorerCatalogFile(path, catalogPath string, entitiesMap map[string]string) ([]ColorerScheme, []string) {
	f, err := os.Open(path)
	if err != nil {
		vtui.DebugLog("COLORER: Cannot open catalog file %q: %v", path, err)
		return nil, nil
	}
	defer f.Close()

	var schemes []ColorerScheme
	var entities []string
	var current *ColorerScheme

	dec := xml.NewDecoder(f)
	dec.Strict = false
	dec.Entity = entitiesMap
	dec.CharsetReader = colorerCharsetReader
	for {
		tok, tErr := dec.Token()
		if tErr != nil {
			break
		}
		switch el := tok.(type) {
		case xml.Directive:
			parseDirectiveEntities(string(el), entitiesMap)
			entities = append(entities, colorerCatalogEntities(string(el))...)
		case xml.StartElement:
			switch strings.ToLower(el.Name.Local) {
			case "hrd":
				current = nil
				if !strings.EqualFold(xmlAttr(el, "class"), "rgb") {
					continue
				}
				current = &ColorerScheme{
					Name:        xmlAttr(el, "name"),
					Description: xmlAttr(el, "description"),
				}
			case "location":
				if current != nil && current.Path == "" {
					link := xmlAttr(el, "link")
					if link != "" {
						resolved, found := resolveColorerLocation(link, path, catalogPath)
						if !found {
							vtui.DebugLog("COLORER: Style %q location %q not found, assuming %q", current.Name, link, resolved)
						}
						current.Path = resolved
					}
				}
			}
		case xml.EndElement:
			if strings.EqualFold(el.Name.Local, "hrd") {
				if current != nil && current.Name != "" && current.Path != "" {
					schemes = append(schemes, *current)
				}
				current = nil
			}
		}
	}

	return schemes, entities
}

// colorerCatalogEntities pulls the system ids out of the entity declarations
// of a DOCTYPE directive. Internal entities carry no path and are skipped.
func colorerCatalogEntities(directive string) []string {
	var paths []string
	for _, decl := range catalogEntityRe.FindAllStringSubmatch(directive, -1) {
		quoted := catalogQuotedRe.FindAllString(decl[1], -1)
		if len(quoted) == 0 {
			continue
		}
		value := quoted[len(quoted)-1]
		paths = append(paths, value[1:len(value)-1])
	}
	return paths
}

// resolveColorerCatalogEntity turns an entity system id into a readable path.
// FarColorer writes them as "env:$FAR_HOME/hrd/catalog-console.xml": the
// variable is expanded when the environment defines it, and what is left is
// looked up next to the file that declared the entity otherwise.
// resolveColorerLocation turns a catalog path into a readable file path.
// Entity system ids and style locations share the same syntax: an optional
// "env:" prefix, environment references, and a path that may be absolute,
// relative to the main catalog, or relative to the file that declared it.
// When nothing exists the catalog-relative form is returned with false, since
// that is the one the colorer engine itself would have used.
func resolveColorerLocation(location, fromFile, catalogPath string) (string, bool) {
	text := strings.TrimSpace(location)
	text = strings.TrimPrefix(text, "env:")
	text = strings.TrimPrefix(text, "file://")
	if text == "" {
		return "", false
	}
	text = filepath.FromSlash(expandColorerCatalogEnv(text))
	relative := strings.TrimLeft(text, `/\`)

	candidates := []string{text}
	fallback := text
	if catalogPath != "" {
		fallback = filepath.Join(filepath.Dir(catalogPath), relative)
		candidates = append(candidates, fallback)
	}
	if fromFile != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(fromFile), relative))
	}

	for _, candidate := range candidates {
		if isColorerCatalogFile(candidate) {
			return candidate, true
		}
	}
	return fallback, false
}

// resolveColorerCatalogEntity resolves the system id of an external entity,
// or returns an empty string when no such file can be found.
func resolveColorerCatalogEntity(systemID, fromFile, catalogPath string) string {
	resolved, found := resolveColorerLocation(systemID, fromFile, catalogPath)
	if !found {
		vtui.DebugLog("COLORER: Catalog entity %q could not be resolved, tried %q", systemID, resolved)
		return ""
	}
	return resolved
}

// expandColorerCatalogEnv replaces the environment references of an entity
// path. An undefined variable expands to nothing, which leaves a path that is
// then looked up relative to the declaring file.
func expandColorerCatalogEnv(text string) string {
	return catalogEnvRe.ReplaceAllStringFunc(text, func(match string) string {
		groups := catalogEnvRe.FindStringSubmatch(match)
		for _, name := range groups[1:] {
			if name != "" {
				return os.Getenv(name)
			}
		}
		return match
	})
}

func isColorerCatalogFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func loadColorerScheme(path string) map[string]colorerRegionStyle {
	f, err := os.Open(path)
	if err != nil {
		vtui.DebugLog("COLORER: Cannot open color style %q: %v", path, err)
		return nil
	}
	defer f.Close()

	entitiesMap := make(map[string]string)
	styles := make(map[string]colorerRegionStyle)
	dec := xml.NewDecoder(f)
	dec.Strict = false
	dec.Entity = entitiesMap
	dec.CharsetReader = colorerCharsetReader
	for {
		tok, tErr := dec.Token()
		if tErr != nil {
			break
		}
		switch el := tok.(type) {
		case xml.Directive:
			parseDirectiveEntities(string(el), entitiesMap)
		case xml.StartElement:
			// Only the region assignments carry colors. The mapper writes them
			// back as <define>, so both spellings are accepted.
			switch strings.ToLower(el.Name.Local) {
			case "assign", "define":
			default:
				continue
			}
			name := xmlAttr(el, "name")
			if name == "" {
				continue
			}
			var style colorerRegionStyle
			style.fore, style.hasFore = parseColorerColor(xmlAttr(el, "fore"))
			style.back, style.hasBack = parseColorerColor(xmlAttr(el, "back"))
			style.style, style.hasStyle = parseColorerStyleBits(xmlAttr(el, "style"))
			if style.hasFore || style.hasBack || style.hasStyle {
				styles[strings.ToLower(name)] = style
			}
		}
	}

	vtui.DebugLog("COLORER: Color style %q defines %d regions", path, len(styles))
	return styles
}

// parseColorerColor understands the "#RRGGBB", "0xRRGGBB" and "RRGGBB" forms
// used by rgb color styles. Console palette indices are rejected.
func parseColorerColor(value string) (uint32, bool) {
	text := strings.TrimSpace(value)
	text = strings.TrimPrefix(text, "#")
	if len(text) > 2 && (strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X")) {
		text = text[2:]
	}
	if len(text) == 8 {
		// An alpha channel may be prepended; the color itself is the low part.
		text = text[2:]
	}
	if len(text) != 6 {
		return 0, false
	}
	parsed, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(parsed), true
}

// parseColorerStyleBits reads the "style" attribute of an assign. Colorer
// parses it as a hex number, so "10" means sixteen and not ten.
func parseColorerStyleBits(value string) (uint32, bool) {
	text := strings.TrimSpace(value)
	text = strings.TrimPrefix(text, "#")
	if text == "" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(parsed), true
}

// applyColorerStyle paints an attribute with a resolved region style. vtui has
// no italic flag, so that bit is dropped.
func applyColorerStyle(base uint64, style colorerRegionStyle) uint64 {
	attr := base
	if style.hasFore {
		attr = vtui.SetRGBFore(attr, style.fore)
	}
	if style.hasBack {
		attr = vtui.SetRGBBack(attr, style.back)
	}
	if !style.hasStyle {
		return attr
	}
	if style.style&colorerStyleBold != 0 {
		attr |= vtui.ForegroundIntensity
	}
	if style.style&colorerStyleUnderline != 0 {
		attr |= vtui.CommonLvbUnderscore
	}
	if style.style&colorerStyleStrikeout != 0 {
		attr |= vtui.CommonLvbStrikeout
	}
	return attr
}

// colorerSchemeLabel is what the user sees in the style lists. FarColorer
// shows the human description of an hrd and keeps the machine name for its
// own configuration, so f4 does the same.
func colorerSchemeLabel(scheme ColorerScheme) string {
	if label := strings.TrimSpace(scheme.Description); label != "" {
		return label
	}
	return scheme.Name
}

// SetColorerScheme activates a color style by name. An empty or unknown name
// switches back to the built-in color map.
func SetColorerScheme(name string) {
	schemeMu.Lock()
	unchanged := name == schemeName
	schemeMu.Unlock()
	if unchanged {
		return
	}

	var styles map[string]colorerRegionStyle
	stylePath := ""
	if name != "" {
		for _, scheme := range ListColorerSchemes() {
			if strings.EqualFold(scheme.Name, name) {
				stylePath = scheme.Path
				styles = loadColorerScheme(scheme.Path)
				break
			}
		}
		if stylePath == "" {
			vtui.DebugLog("COLORER: Color style %q is not listed in the catalog", name)
		}
	}

	keys := make([]string, 0, len(styles))
	for key := range styles {
		keys = append(keys, key)
	}
	sortColorerKeys(keys)

	schemeMu.Lock()
	schemeName = name
	schemeStyles = styles
	schemeKeys = keys
	schemeMemo = make(map[string]colorerRegionStyle)
	schemeGeneration++
	schemeMu.Unlock()

	vtui.DebugLog("COLORER: Color style %q activated from %q, %d regions defined", name, stylePath, len(styles))
}

// ResetColorerScheme drops the styles of the active color style so that the
// next SetColorerScheme reads them from disk again. Replacing the schemas or
// pointing the catalog somewhere else leaves the style name untouched, and
// SetColorerScheme skips a switch to the name that is already active.
func ResetColorerScheme() {
	schemeMu.Lock()
	schemeName = ""
	schemeStyles = nil
	schemeKeys = nil
	schemeMemo = make(map[string]colorerRegionStyle)
	schemeGeneration++
	schemeMu.Unlock()
}

// ColorerSchemeGeneration changes every time a color style is applied.
// Highlighters cache the attributes they computed per line, so they watch it
// to notice that the colors underneath them have been replaced.
func ColorerSchemeGeneration() uint64 {
	schemeMu.Lock()
	defer schemeMu.Unlock()
	return schemeGeneration
}

// colorerSchemeActive reports whether a color style from the catalog is in
// charge of the colors.
func colorerSchemeActive() bool {
	schemeMu.Lock()
	defer schemeMu.Unlock()
	return schemeStyles != nil
}

// InvalidateColorerRegionCache drops the memoized region colors and makes the
// highlighters recompute their lines. It is called once the region graph has
// been scanned, since everything resolved before that was an approximation.
func InvalidateColorerRegionCache() {
	schemeMu.Lock()
	schemeMemo = make(map[string]colorerRegionStyle)
	schemeGeneration++
	schemeMu.Unlock()
}

// colorerBackgroundRegion is the region FarColorer copies into the editor
// color when its "Change editor background" option is on.
const colorerBackgroundRegion = "def:Text"

// colorerSchemeExactStyle looks a region up in the active color style without
// the prefix and substring fallbacks. The canvas color must not be guessed
// from an unrelated region, so a style without def:Text finds nothing here.
func colorerSchemeExactStyle(name string) (colorerRegionStyle, bool) {
	schemeMu.Lock()
	defer schemeMu.Unlock()
	if schemeStyles == nil {
		return colorerRegionStyle{}, false
	}
	style, found := schemeStyles[strings.ToLower(name)]
	return style, found && (style.hasFore || style.hasBack || style.hasStyle)
}

// ColorerEditorBaseAttr paints the editor with the def:Text region of the
// active color style, the way FarColorer's "Change editor background" option
// does. Without Colorer, without an active style, or with the option off the
// f4 palette is returned untouched.
func ColorerEditorBaseAttr(base uint64) uint64 {
	if !AppConfig.EditorColorerBackground {
		return base
	}
	if !strings.EqualFold(AppConfig.EditorHighlighter, "Colorer") {
		return base
	}
	style, ok := colorerSchemeExactStyle(colorerBackgroundRegion)
	if !ok {
		return base
	}

	attr := base
	if style.hasFore {
		attr = vtui.SetRGBFore(attr, style.fore)
	}
	if style.hasBack {
		attr = vtui.SetRGBBack(attr, style.back)
	}
	return attr
}

// minColorerLocalMatch keeps the local name comparison from latching onto a
// two letter fragment of an unrelated region.
const minColorerLocalMatch = 3

// maxUnresolvedColorerLogs bounds the report of the regions no rule could
// resolve, so that a file full of them cannot flood the log.
const maxUnresolvedColorerLogs = 64

var (
	unresolvedMu    sync.Mutex
	unresolvedSeen  = make(map[string]bool)
	unresolvedCount int
)

// logUnresolvedColorerRegion reports a region that reached no color at all.
// That is what tells a hole in the recovered hierarchy from a style that
// genuinely leaves a region alone, and there is no other way to see it from
// the outside.
func logUnresolvedColorerRegion(name string) {
	unresolvedMu.Lock()
	defer unresolvedMu.Unlock()
	if unresolvedCount >= maxUnresolvedColorerLogs || unresolvedSeen[name] {
		return
	}
	unresolvedSeen[name] = true
	unresolvedCount++
	vtui.DebugLog("COLORER: No color for region %q in the active style", name)
}

// colorerSchemeStyle resolves a region name through the active color style.
// Colorer looks the full name up and then walks the region parents declared in
// the hrc schemas. The recovered graph is only as complete as the files that
// could be read, and colorer pulls a good part of its region declarations in
// through external entities that Go's decoder never expands, so the name based
// rules stay in place behind the graph rather than instead of it. Everything
// they reach is another region of the very same style, so a monochrome style
// stays monochrome; only the built-in color map is out of bounds here.
func colorerSchemeStyle(name string) (colorerRegionStyle, bool) {
	nameLower := strings.ToLower(name)
	chain := append([]string{nameLower}, ColorerRegionChain(nameLower)...)

	schemeMu.Lock()
	defer schemeMu.Unlock()
	if schemeStyles == nil {
		return colorerRegionStyle{}, false
	}
	if schemeMemo == nil {
		schemeMemo = make(map[string]colorerRegionStyle)
	}
	if cached, hit := schemeMemo[nameLower]; hit {
		return cached, cached.hasFore || cached.hasBack || cached.hasStyle
	}

	var finalStyle colorerRegionStyle
	var anyFound bool

	vtui.DebugLog("    Trace chain for %q: %v", name, chain)
	for _, parent := range chain {
		if style, found := schemeStyles[parent]; found {
			vtui.DebugLog("      Matched parent %q in HRD -> fore=#%06X (has=%v) back=#%06X (has=%v) style=%d", parent, style.fore, style.hasFore, style.back, style.hasBack, style.style)
			if !finalStyle.hasFore && style.hasFore {
				finalStyle.fore = style.fore
				finalStyle.hasFore = true
			}
			if !finalStyle.hasBack && style.hasBack {
				finalStyle.back = style.back
				finalStyle.hasBack = true
			}
			if style.hasStyle {
				finalStyle.style |= style.style
				finalStyle.hasStyle = true
			}
			anyFound = true
		}
	}

	if !anyFound {
		var fallbackStyle colorerRegionStyle
		var fallbackFound bool
		for _, key := range schemeKeys {
			if strings.HasPrefix(nameLower, key) {
				fallbackStyle, fallbackFound = schemeStyles[key], true
				break
			}
		}
		if !fallbackFound {
			for _, key := range schemeKeys {
				if strings.Contains(nameLower, key) {
					fallbackStyle, fallbackFound = schemeStyles[key], true
					break
				}
			}
		}
		if !fallbackFound {
			if local := colorerRegionLocalName(nameLower); len(local) >= minColorerLocalMatch {
				for _, key := range schemeKeys {
					keyLocal := colorerRegionLocalName(key)
					if len(keyLocal) >= minColorerLocalMatch && strings.Contains(local, keyLocal) {
						fallbackStyle, fallbackFound = schemeStyles[key], true
						break
					}
				}
			}
		}
		if fallbackFound {
			finalStyle = fallbackStyle
			anyFound = true
		}
	}

	if !anyFound {
		finalStyle = colorerRegionStyle{}
		logUnresolvedColorerRegion(nameLower)
	}

	schemeMemo[nameLower] = finalStyle
	return finalStyle, anyFound
}

// sortColorerKeys orders keys from the longest to the shortest one, so that
// the most specific key always wins and the result never depends on Go's
// random map iteration order.
func sortColorerKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
}

func xmlAttr(el xml.StartElement, name string) string {
	for _, attr := range el.Attr {
		if strings.EqualFold(attr.Name.Local, name) {
			return attr.Value
		}
	}
	return ""
}
