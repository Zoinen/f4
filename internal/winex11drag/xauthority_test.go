package winex11drag

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// encodeXauthEntry builds one binary .Xauthority record, byte for byte in
// the same layout ParseXauthority reads: family, then four length-prefixed
// fields. Tests compose several of these to build a synthetic file without
// needing a real one on disk.
func encodeXauthEntry(t *testing.T, family uint16, address, display, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writeField := func(s []byte) {
		if len(s) > 65535 {
			t.Fatalf("field is %d bytes, too long for a uint16-prefixed .Xauthority field", len(s))
		}
		length := uint16(len(s)) // #nosec G115 -- bounds-checked immediately above.
		if err := binary.Write(&buf, binary.BigEndian, length); err != nil {
			t.Fatalf("writing field length: %v", err)
		}
		buf.Write(s)
	}
	if err := binary.Write(&buf, binary.BigEndian, family); err != nil {
		t.Fatalf("writing family: %v", err)
	}
	writeField([]byte(address))
	writeField([]byte(display))
	writeField([]byte(name))
	writeField(data)
	return buf.Bytes()
}

func TestParseXauthority(t *testing.T) {
	cookie0 := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	cookie1 := []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	var raw []byte
	raw = append(raw, encodeXauthEntry(t, XauthFamilyLocal, "myhost", "0", "MIT-MAGIC-COOKIE-1", cookie0)...)
	raw = append(raw, encodeXauthEntry(t, XauthFamilyLocal, "myhost", "1", "MIT-MAGIC-COOKIE-1", cookie1)...)

	entries, err := ParseXauthority(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseXauthority: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	if entries[0].Family != XauthFamilyLocal || entries[0].Address != "myhost" ||
		entries[0].Display != "0" || entries[0].Name != "MIT-MAGIC-COOKIE-1" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
	if !bytes.Equal(entries[0].Data, cookie0) {
		t.Errorf("entries[0].Data = %x, want %x", entries[0].Data, cookie0)
	}
	if entries[1].Display != "1" || !bytes.Equal(entries[1].Data, cookie1) {
		t.Errorf("entries[1] = %+v", entries[1])
	}
}

func TestParseXauthorityEmptyFileIsNotAnError(t *testing.T) {
	entries, err := ParseXauthority(bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("ParseXauthority(empty) = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries from an empty file, want 0", len(entries))
	}
}

func TestParseXauthorityTruncatedRecordIsAnError(t *testing.T) {
	full := encodeXauthEntry(t, XauthFamilyLocal, "myhost", "0", "MIT-MAGIC-COOKIE-1", []byte{1, 2, 3, 4})
	truncated := full[:len(full)-2] // cuts off partway through the auth data
	if _, err := ParseXauthority(bytes.NewReader(truncated)); err == nil {
		t.Fatalf("ParseXauthority(truncated) succeeded, want an error")
	}
}

func TestFindMagicCookie(t *testing.T) {
	entries := []XauthEntry{
		{Family: XauthFamilyLocal, Address: "myhost", Display: "0", Name: "MIT-MAGIC-COOKIE-1", Data: []byte("cookie-for-0")},
		{Family: XauthFamilyLocal, Address: "myhost", Display: "1", Name: "MIT-MAGIC-COOKIE-1", Data: []byte("cookie-for-1")},
		{Family: XauthFamilyWild, Address: "", Display: "2", Name: "OTHER-AUTH", Data: []byte("not a cookie")},
	}

	for _, tt := range []struct {
		name    string
		display string
		want    string
		wantOK  bool
	}{
		{name: "display 0", display: "0", want: "cookie-for-0", wantOK: true},
		{name: "display 1", display: "1", want: "cookie-for-1", wantOK: true},
		{name: "display has an entry but not a magic cookie one", display: "2", wantOK: false},
		{name: "display absent entirely", display: "9", wantOK: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FindMagicCookie(entries, tt.display)
			if ok != tt.wantOK {
				t.Fatalf("FindMagicCookie(%q) ok = %v, want %v", tt.display, ok, tt.wantOK)
			}
			if ok && string(got) != tt.want {
				t.Errorf("FindMagicCookie(%q) = %q, want %q", tt.display, got, tt.want)
			}
		})
	}
}

func TestFindMagicCookiePrefersLocalFamilyOverAnUnrelatedEntry(t *testing.T) {
	entries := []XauthEntry{
		{Family: 999, Address: "elsewhere", Display: "5", Name: "MIT-MAGIC-COOKIE-1", Data: []byte("weird-family-cookie")},
		{Family: XauthFamilyLocal, Address: "myhost", Display: "5", Name: "MIT-MAGIC-COOKIE-1", Data: []byte("local-cookie")},
	}
	got, ok := FindMagicCookie(entries, "5")
	if !ok || string(got) != "local-cookie" {
		t.Fatalf("FindMagicCookie = (%q, %v), want (%q, true)", got, ok, "local-cookie")
	}
}

func TestFindMagicCookieMatchesWildcardDisplayEntry(t *testing.T) {
	entries := []XauthEntry{
		{Family: XauthFamilyLocal, Address: "myhost", Display: "", Name: "MIT-MAGIC-COOKIE-1", Data: []byte("wildcard-cookie")},
	}
	got, ok := FindMagicCookie(entries, "5")
	if !ok || string(got) != "wildcard-cookie" {
		t.Fatalf("FindMagicCookie with wildcard display entry = (%q, %v), want (%q, true)", got, ok, "wildcard-cookie")
	}
}
