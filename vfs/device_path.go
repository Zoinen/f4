package vfs

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// DevicePath maps a device's POSIX paths to human-readable, qualified VFS paths.
// Device is a discovery-resolved name, never the transport's session identity.
// Domain optionally identifies a mounted export within that device.
type DevicePath struct {
	Scheme string
	Device string
	Domain string
}

func devicePathSegment(value string) string {
	// These are f4 addresses, not network URLs: Unicode, spaces and apostrophes
	// remain readable, while delimiters and literal percent signs round-trip.
	const hex = "0123456789ABCDEF"
	var encoded strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c < 32 || c == 127 || strings.ContainsRune("%/\\?#@:", rune(c)) {
			encoded.WriteByte('%')
			encoded.WriteByte(hex[c>>4])
			encoded.WriteByte(hex[c&15])
		} else {
			encoded.WriteByte(c)
		}
	}
	return encoded.String()
}

func encodeDevicePath(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for i, part := range parts {
		parts[i] = devicePathSegment(part)
	}
	return strings.Join(parts, "/")
}

// Root returns the qualified root of the mounted export, with a trailing slash.
func (d DevicePath) Root() string {
	root := strings.ToLower(d.Scheme) + "://" + devicePathSegment(d.Device) + "/"
	if domain := strings.Trim(d.Domain, "/"); domain != "" {
		root += encodeDevicePath(domain) + "/"
	}
	return root
}

// Public qualifies a native device-side absolute path.
func (d DevicePath) Public(remote string) string {
	return d.Root() + encodeDevicePath(path.Clean("/"+strings.TrimPrefix(remote, "/")))
}

// ParseDevicePath decodes an f4 device address without interpreting the device
// name as a DNS host, userinfo, or port. The returned remote path is absolute.
func ParseDevicePath(raw string) (scheme, device, remote string, err error) {
	scheme, ok := URIScheme(raw)
	if !ok {
		return "", "", "", fmt.Errorf("device path: missing scheme")
	}
	rest := raw[strings.Index(raw, "://")+3:]
	authority, tail, _ := strings.Cut(rest, "/")
	device, err = url.PathUnescape(authority)
	if err != nil || device == "" || strings.ContainsAny(device, "\x00\r\n") {
		return "", "", "", fmt.Errorf("device path: invalid device name")
	}
	parts := strings.Split(tail, "/")
	for i, part := range parts {
		decoded, decodeErr := url.PathUnescape(part)
		if decodeErr != nil || strings.ContainsAny(decoded, "/\x00") {
			return "", "", "", fmt.Errorf("device path: invalid path segment")
		}
		parts[i] = decoded
	}
	return scheme, device, "/" + strings.Join(parts, "/"), nil
}

// Remote accepts native absolute/relative paths and this export's public paths.
// A qualified path to another device or export never reaches the transport.
func (d DevicePath) Remote(currentRemote, input string) (string, error) {
	if strings.ContainsRune(input, 0) {
		return "", fmt.Errorf("device path: NUL is not allowed")
	}
	if IsURIPath(input) {
		scheme, device, remote, err := ParseDevicePath(input)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(scheme, d.Scheme) || device != d.Device {
			return "", fmt.Errorf("device path: address belongs to another device")
		}
		root := "/" + strings.Trim(d.Domain, "/")
		if root != "/" {
			if remote != root && !strings.HasPrefix(remote, root+"/") {
				return "", fmt.Errorf("device path: address belongs to another export")
			}
			remote = strings.TrimPrefix(remote, root)
		}
		// Resolve within the export, never through a selector or device boundary.
		input = "/" + strings.TrimPrefix(remote, "/")
	}
	if path.IsAbs(input) {
		return path.Clean(input), nil
	}
	return path.Join("/", currentRemote, input), nil
}

func (d DevicePath) IsAbs(p string) bool { return path.IsAbs(p) || IsURIPath(p) }

// Join retains the URI authority; subsequent elements are literal filenames.
func (d DevicePath) Join(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	if IsURIPath(elem[0]) {
		scheme, device, remote, err := ParseDevicePath(elem[0])
		if err != nil {
			return elem[0]
		}
		parts := append([]string{remote}, elem[1:]...)
		return (DevicePath{Scheme: scheme, Device: device}).Public(path.Join(parts...))
	}
	return path.Join(elem...)
}

func (d DevicePath) Base(p string) string {
	if IsURIPath(p) {
		_, _, remote, err := ParseDevicePath(p)
		if err == nil {
			return path.Base(remote)
		}
	}
	return path.Base(p)
}

func (d DevicePath) Dir(p string) string {
	if IsURIPath(p) {
		remote, err := d.Remote("/", p)
		if err == nil {
			return d.Public(path.Dir(remote))
		}
		return p
	}
	return path.Dir(p)
}
