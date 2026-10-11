package winex11drag

import (
	"encoding/binary"
	"errors"
	"io"
)

// Family values from /usr/include/X11/Xauth.h -- the same constants every
// X11 client, including the vendored github.com/jezek/xgb's own Xauthority
// reader (auth.go), matches an entry's Address field against.
const (
	XauthFamilyLocal uint16 = 256
	XauthFamilyWild  uint16 = 65535
)

// XauthEntry is one record of a binary .Xauthority file: an address family,
// the address and display it applies to, and an authentication name/data
// pair -- usually "MIT-MAGIC-COOKIE-1" and a 16-byte cookie.
type XauthEntry struct {
	Family  uint16
	Address string
	Display string
	Name    string
	Data    []byte
}

// ParseXauthority reads every record from a binary .Xauthority file: the
// format xauth(1) writes and that every X11 client reads back, a sequence of
//
//	uint16 family (big-endian)
//	uint16 length + that many bytes: address
//	uint16 length + that many bytes: display number
//	uint16 length + that many bytes: auth name
//	uint16 length + that many bytes: auth data
//
// repeated until EOF (issue #566, step 2). This function only needs an
// io.Reader, not a path -- OpenWineX11Connection reads the host's real
// Xauthority file through libwinescape rather than package os, since Wine's
// own $HOME and $XAUTHORITY point into the Win32 world, not the host's.
func ParseXauthority(r io.Reader) ([]XauthEntry, error) {
	var entries []XauthEntry
	for {
		var family uint16
		if err := binary.Read(r, binary.BigEndian, &family); err != nil {
			if errors.Is(err, io.EOF) {
				return entries, nil
			}
			return entries, err
		}

		address, err := readXauthField(r)
		if err != nil {
			return entries, err
		}
		display, err := readXauthField(r)
		if err != nil {
			return entries, err
		}
		name, err := readXauthField(r)
		if err != nil {
			return entries, err
		}
		data, err := readXauthField(r)
		if err != nil {
			return entries, err
		}

		entries = append(entries, XauthEntry{
			Family:  family,
			Address: string(address),
			Display: string(display),
			Name:    string(name),
			Data:    data,
		})
	}
}

// readXauthField reads one length-prefixed field: a big-endian uint16
// length followed by that many bytes. A clean io.EOF here means the file
// ended partway through a record, which readXauthField reports as
// io.ErrUnexpectedEOF -- a record's four fields and its family are not
// optional once the family byte has been read.
func readXauthField(r io.Reader) ([]byte, error) {
	var n uint16
	if err := binary.Read(r, binary.BigEndian, &n); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	buf := make([]byte, n)
	if n > 0 {
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// FindMagicCookie picks the MIT-MAGIC-COOKIE-1 entry matching the given
// display number (e.g. "0" for DISPLAY=":0" -- DisplaySpec.Display formatted
// as a string, with no hostname or screen) among entries parsed by
// ParseXauthority.
//
// A real .Xauthority file holds one relevant record per display in the
// overwhelming majority of cases, so this matches on display number and
// auth name first. An entry with an empty Display field is a wildcard that
// matches any display, per the same convention xauth(1) and every X11
// client's own reader use. Among several MIT-MAGIC-COOKIE-1 candidates for
// the same display, XauthFamilyLocal or XauthFamilyWild is preferred, since
// those are the families a local Unix-domain socket connection actually
// uses; anything else is kept only as a fallback.
func FindMagicCookie(entries []XauthEntry, display string) ([]byte, bool) {
	var fallback []byte
	haveFallback := false
	for _, e := range entries {
		if e.Name != "MIT-MAGIC-COOKIE-1" {
			continue
		}
		if e.Display != "" && e.Display != display {
			continue
		}
		if e.Family == XauthFamilyLocal || e.Family == XauthFamilyWild {
			return e.Data, true
		}
		if !haveFallback {
			fallback, haveFallback = e.Data, true
		}
	}
	return fallback, haveFallback
}
