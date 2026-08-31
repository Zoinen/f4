package winshell

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	destinationPrefix = Scheme + "://create/"
	homeURI           = Scheme + "://home/"

	homeCLSID        = "{F874310E-B6B7-47DC-BC84-B9E6B38F5903}"
	galleryCLSID     = "{E88865EA-0E1C-4E20-9AA6-EDCD0212C87C}"
	quickAccessCLSID = "{679F85CB-0220-4080-B29B-5540CC05AAB6}"
	thisPCCLSID      = "{20D04FE0-3AEA-1069-A2D8-08002B30309D}"
	networkCLSID     = "{F02C1A0D-BE21-4350-88B0-7367FC96EF3C}"
	linuxCLSID       = "{B2B4A4D1-2754-4140-A2EB-9A76D9D7CDC6}"
	recycleBinCLSID  = "{645FF040-5081-101B-9F08-00AA002F954E}"
)

type shellLocationAlias struct {
	alias string
	clsid string
}

var shellLocationAliases = []shellLocationAlias{
	{alias: "home", clsid: homeCLSID},
	{alias: "gallery", clsid: galleryCLSID},
	{alias: "quick-access", clsid: quickAccessCLSID},
	{alias: "this-pc", clsid: thisPCCLSID},
	{alias: "network", clsid: networkCLSID},
	{alias: "linux", clsid: linuxCLSID},
	{alias: "recycle-bin", clsid: recycleBinCLSID},
}

type namedShellAlias struct {
	alias       string
	parsingName string
}

var namedShellAliases = []namedShellAlias{
	{alias: "desktop", parsingName: "shell:Desktop"},
	{alias: "downloads", parsingName: "shell:Downloads"},
	{alias: "documents", parsingName: "shell:Personal"},
	{alias: "pictures", parsingName: "shell:My Pictures"},
	{alias: "music", parsingName: "shell:My Music"},
	{alias: "videos", parsingName: "shell:My Video"},
}

// URIFromParsingName returns a persistent, user-facing URI for a Windows
// Shell parsing identity. Known namespace roots, local paths, UNC paths, and
// WSL distributions all receive readable hierarchical forms. The generic
// fallback is percent-escaped text.
func URIFromParsingName(parsingName string) string {
	if clsid, suffix, ok := splitCLSIDParsingName(parsingName); ok {
		if alias, known := aliasForCLSID(clsid); known && suffix == "" {
			return friendlyURI(alias)
		}
		root := strings.Trim(clsid, "{}")
		if alias, known := aliasForCLSID(clsid); known {
			root = alias
		}
		segments := []string{root}
		segments = append(segments, splitBackslashPath(suffix)...)
		return friendlyURI("namespace", segments...)
	}

	if alias, ok := aliasForNamedShellPath(parsingName); ok {
		return friendlyURI(alias)
	}

	if drive, segments, ok := splitDriveParsingName(parsingName); ok {
		return friendlyURI("local", append([]string{drive}, segments...)...)
	}

	if server, segments, ok := splitUNCParsingName(parsingName); ok {
		if len(segments) > 0 && (strings.EqualFold(server, "wsl.localhost") || strings.EqualFold(server, "wsl$")) {
			return friendlyURI("linux", segments...)
		}
		return friendlyURI("network", append([]string{server}, segments...)...)
	}

	if len(parsingName) >= len("shell:") && strings.EqualFold(parsingName[:len("shell:")], "shell:") {
		return friendlyURI("shell", splitBackslashPath(parsingName[len("shell:"):])...)
	}

	return friendlyURI("parsing", parsingName)
}

// ParsingNameFromURI resolves a readable Windows namespace URI.
func ParsingNameFromURI(raw string) (string, error) {
	authority, segments, err := splitFriendlyURI(raw)
	if err != nil {
		return "", err
	}

	if clsid, ok := clsidForAlias(authority); ok {
		if len(segments) == 0 {
			return "::" + clsid, nil
		}
		if authority != "linux" && authority != "network" {
			return "", fmt.Errorf("invalid Windows Shell location URI")
		}
	}
	if parsingName, ok := namedShellPathForAlias(authority); ok {
		if len(segments) != 0 {
			return "", fmt.Errorf("invalid Windows Shell named location URI")
		}
		return parsingName, nil
	}

	switch authority {
	case "local":
		if len(segments) == 0 || !isDriveSegment(segments[0]) {
			return "", fmt.Errorf("invalid local Windows Shell URI")
		}
		drive := strings.ToUpper(segments[0][:1]) + ":"
		if len(segments) == 1 {
			return drive + `\`, nil
		}
		return drive + `\` + strings.Join(segments[1:], `\`), nil
	case "linux":
		if len(segments) == 0 {
			return "::" + linuxCLSID, nil
		}
		return `\\wsl.localhost\` + strings.Join(segments, `\`), nil
	case "network":
		if len(segments) == 0 {
			return "::" + networkCLSID, nil
		}
		return `\\` + strings.Join(segments, `\`), nil
	case "shell":
		if len(segments) == 0 {
			return "", fmt.Errorf("invalid named Windows Shell URI")
		}
		return "shell:" + strings.Join(segments, `\`), nil
	case "namespace":
		if len(segments) == 0 {
			return "", fmt.Errorf("invalid Windows Shell namespace URI")
		}
		clsid, ok := clsidForAlias(segments[0])
		if !ok {
			clsid, ok = normalizeBareCLSID(segments[0])
		}
		if !ok {
			return "", fmt.Errorf("invalid Windows Shell namespace root")
		}
		parsingName := "::" + clsid
		if len(segments) > 1 {
			parsingName += `\` + strings.Join(segments[1:], `\`)
		}
		return parsingName, nil
	case "parsing":
		if len(segments) != 1 || segments[0] == "" {
			return "", fmt.Errorf("invalid Windows Shell parsing URI")
		}
		return segments[0], nil
	default:
		return "", fmt.Errorf("invalid Windows Shell URI")
	}
}

func IsURI(raw string) bool {
	if _, err := ParsingNameFromURI(raw); err == nil {
		return true
	}
	_, _, err := DestinationFromURI(raw)
	return err == nil
}

// DestinationURI represents a not-yet-existing child. Its parent is embedded
// using the same readable URI hierarchy.
func DestinationURI(parentParsingName, name string) string {
	parent := strings.TrimSuffix(strings.TrimPrefix(URIFromParsingName(parentParsingName), Scheme+"://"), "/")
	return destinationPrefix + parent + "/" + escapeFriendlySegment(name)
}

func DestinationFromURI(raw string) (parentParsingName, name string, err error) {
	if !strings.HasPrefix(strings.ToLower(raw), destinationPrefix) {
		return "", "", fmt.Errorf("invalid Windows Shell destination URI")
	}
	payload := raw[len(destinationPrefix):]
	separator := strings.LastIndexByte(payload, '/')
	if separator <= 0 || separator == len(payload)-1 {
		return "", "", fmt.Errorf("invalid Windows Shell destination URI payload")
	}
	parentURI := Scheme + "://" + payload[:separator] + "/"
	parentParsingName, err = ParsingNameFromURI(parentURI)
	if err != nil {
		return "", "", err
	}
	name, err = unescapeFriendlySegment(payload[separator+1:])
	if err != nil || strings.TrimSpace(name) == "" || strings.ContainsAny(name, `\/`) {
		return "", "", fmt.Errorf("invalid Windows Shell destination name")
	}
	return parentParsingName, name, nil
}

func friendlyURI(authority string, segments ...string) string {
	var result strings.Builder
	result.WriteString(Scheme)
	result.WriteString("://")
	result.WriteString(strings.ToLower(authority))
	result.WriteByte('/')
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		result.WriteString(escapeFriendlySegment(segment))
		result.WriteByte('/')
	}
	return result.String()
}

func splitFriendlyURI(raw string) (string, []string, error) {
	prefix := Scheme + "://"
	if !strings.HasPrefix(strings.ToLower(raw), prefix) {
		return "", nil, fmt.Errorf("invalid Windows Shell URI")
	}
	rest := raw[len(prefix):]
	separator := strings.IndexByte(rest, '/')
	if separator < 0 {
		if rest == "" {
			return "", nil, fmt.Errorf("invalid Windows Shell URI")
		}
		return strings.ToLower(rest), nil, nil
	}
	authority := strings.ToLower(rest[:separator])
	if authority == "" {
		return "", nil, fmt.Errorf("invalid Windows Shell URI authority")
	}
	payload := strings.TrimSuffix(rest[separator+1:], "/")
	if payload == "" {
		return authority, nil, nil
	}
	parts := strings.Split(payload, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return "", nil, fmt.Errorf("invalid Windows Shell URI path")
		}
		segment, err := unescapeFriendlySegment(part)
		if err != nil || segment == "" || strings.IndexByte(segment, 0) >= 0 {
			return "", nil, fmt.Errorf("invalid Windows Shell URI path")
		}
		segments = append(segments, segment)
	}
	return authority, segments, nil
}

func escapeFriendlySegment(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "/", "%2F")
	return strings.ReplaceAll(value, `\`, "%5C")
}

func unescapeFriendlySegment(value string) (string, error) {
	decoded, err := url.PathUnescape(value)
	if err != nil || strings.IndexByte(decoded, 0) >= 0 {
		return "", fmt.Errorf("invalid Windows Shell URI path")
	}
	return decoded, nil
}

func aliasForCLSID(clsid string) (string, bool) {
	for _, location := range shellLocationAliases {
		if strings.EqualFold(clsid, location.clsid) {
			return location.alias, true
		}
	}
	return "", false
}

func clsidForAlias(alias string) (string, bool) {
	for _, location := range shellLocationAliases {
		if strings.EqualFold(alias, location.alias) {
			return location.clsid, true
		}
	}
	return "", false
}

func aliasForNamedShellPath(parsingName string) (string, bool) {
	for _, location := range namedShellAliases {
		if strings.EqualFold(parsingName, location.parsingName) {
			return location.alias, true
		}
	}
	return "", false
}

func namedShellPathForAlias(alias string) (string, bool) {
	for _, location := range namedShellAliases {
		if strings.EqualFold(alias, location.alias) {
			return location.parsingName, true
		}
	}
	return "", false
}

func splitCLSIDParsingName(parsingName string) (clsid, suffix string, ok bool) {
	value := strings.TrimSpace(parsingName)
	if len(value) >= len("shell:") && strings.EqualFold(value[:len("shell:")], "shell:") {
		value = value[len("shell:"):]
	}
	if len(value) < 40 || !strings.HasPrefix(value, "::{") {
		return "", "", false
	}
	clsid = value[2:40]
	if normalized, valid := normalizeCLSID(clsid); valid {
		clsid = normalized
	} else {
		return "", "", false
	}
	if len(value) == 40 {
		return clsid, "", true
	}
	if value[40] != '\\' {
		return "", "", false
	}
	return clsid, value[41:], true
}

func normalizeBareCLSID(value string) (string, bool) {
	return normalizeCLSID("{" + strings.Trim(value, "{}") + "}")
}

func normalizeCLSID(value string) (string, bool) {
	if len(value) != 38 || value[0] != '{' || value[37] != '}' {
		return "", false
	}
	for index := 1; index < 37; index++ {
		if index == 9 || index == 14 || index == 19 || index == 24 {
			if value[index] != '-' {
				return "", false
			}
			continue
		}
		if !isHexDigit(value[index]) {
			return "", false
		}
	}
	return strings.ToUpper(value), true
}

func isHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func splitDriveParsingName(parsingName string) (string, []string, bool) {
	value := parsingName
	if len(value) >= 4 && strings.EqualFold(value[:4], `\\?\`) {
		value = value[4:]
	}
	value = strings.ReplaceAll(value, "/", `\`)
	if len(value) < 3 || !isASCIIAlpha(value[0]) || value[1] != ':' || value[2] != '\\' {
		return "", nil, false
	}
	drive := strings.ToUpper(value[:1]) + ":"
	return drive, splitBackslashPath(value[3:]), true
}

func splitUNCParsingName(parsingName string) (string, []string, bool) {
	value := strings.ReplaceAll(parsingName, "/", `\`)
	if len(value) >= len(`\\?\UNC\`) && strings.EqualFold(value[:len(`\\?\UNC\`)], `\\?\UNC\`) {
		value = `\\` + value[len(`\\?\UNC\`):]
	}
	if !strings.HasPrefix(value, `\\`) {
		return "", nil, false
	}
	parts := splitBackslashPath(value[2:])
	if len(parts) == 0 {
		return "", nil, false
	}
	return parts[0], parts[1:], true
}

func splitBackslashPath(value string) []string {
	if value == "" {
		return nil
	}
	raw := strings.Split(value, `\`)
	result := make([]string, 0, len(raw))
	for _, part := range raw {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func isDriveSegment(value string) bool {
	return len(value) == 2 && isASCIIAlpha(value[0]) && value[1] == ':'
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
