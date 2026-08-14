package cloudfox

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

// URI is a parsed, secret-free CloudFox location.
type URI struct {
	Provider     ProviderType
	ConnectionID string
	Location     string
}

func (u URI) String() string {
	if u.Provider == "" {
		return ManagerRoot
	}
	base := ManagerRoot + string(u.Provider) + "/" + strings.ToLower(u.ConnectionID)
	encoded := EncodeLocation(u.Location)
	if encoded == "" {
		return base
	}
	return base + "/" + encoded
}

// ParseURI accepts only the canonical cloud://provider/uuid/location form.
// User info, ports, query strings and fragments are rejected so credentials
// can never be smuggled into session or bookmark persistence.
func ParseURI(raw string) (URI, error) {
	if raw == ManagerRoot {
		return URI{}, nil
	}
	// File-operation dialogs append a slash to an absolute destination to
	// mark it as a directory. CloudFox locations are opaque tokens rather
	// than hierarchical URL paths, so that marker is not part of the remote
	// location and must be removed before decoding.
	raw = strings.TrimRight(raw, "/")
	u, err := url.Parse(raw)
	if err != nil {
		return URI{}, fmt.Errorf("cloudfox: parse URI: %w", err)
	}
	if !strings.EqualFold(u.Scheme, Scheme) || u.Host == "" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
		return URI{}, fmt.Errorf("cloudfox: invalid URI %q", raw)
	}
	provider := ProviderType(strings.ToLower(u.Hostname()))
	if !provider.Valid() {
		return URI{}, fmt.Errorf("cloudfox: unsupported URI provider %q", u.Host)
	}
	parts := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(parts) == 0 || !validUUID(parts[0]) {
		return URI{}, fmt.Errorf("cloudfox: invalid connection id in %q", raw)
	}
	connectionID := strings.ToLower(parts[0])
	location, err := DecodeLocation(strings.Join(parts[1:], "/"))
	if err != nil {
		return URI{}, err
	}
	return URI{Provider: provider, ConnectionID: connectionID, Location: location}, nil
}

// EncodeLocation percent-escapes each opaque backend location segment. Literal
// dot segments are forced to escapes because URL parsers otherwise normalize
// them. A leading slash is a backend detail and is intentionally preserved.
func EncodeLocation(location string) string {
	if location == "" {
		return ""
	}
	leading := strings.HasPrefix(location, "/")
	segments := strings.Split(strings.TrimPrefix(location, "/"), "/")
	for i, segment := range segments {
		if segment == "." {
			segments[i] = "%2E"
		} else if segment == ".." {
			segments[i] = "%2E%2E"
		} else {
			segments[i] = url.PathEscape(segment)
		}
	}
	encoded := strings.Join(segments, "/")
	if leading {
		return "%2F" + encoded
	}
	return encoded
}

func DecodeLocation(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	leading := false
	if strings.HasPrefix(strings.ToUpper(encoded), "%2F") {
		leading = true
		encoded = encoded[3:]
	}
	segments := strings.Split(encoded, "/")
	for i, segment := range segments {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return "", fmt.Errorf("cloudfox: invalid escaped location: %w", err)
		}
		if strings.Contains(decoded, "/") || strings.ContainsRune(decoded, '\x00') {
			return "", errors.New("cloudfox: escaped location segment contains a separator or NUL")
		}
		segments[i] = decoded
	}
	location := strings.Join(segments, "/")
	if leading {
		location = "/" + location
	}
	return location, nil
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("cloudfox: create connection id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]), hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]), hex.EncodeToString(b[10:16])), nil
}

func validUUID(s string) bool {
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	for i := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func cleanDisplayPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	cleaned := path.Clean("/" + strings.TrimPrefix(p, "/"))
	if cleaned == "/." {
		return "/"
	}
	return cleaned
}
