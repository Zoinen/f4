package cloudfox

import (
	"strings"
	"testing"
)

const testConnectionID = "123e4567-e89b-42d3-a456-426614174000"

func TestURIRoundTripOpaqueLocations(t *testing.T) {
	t.Parallel()
	locations := []string{
		"/my/root-id",
		"shared/drive id/файлы/100%",
		"prefix/./../literal",
		"object/contains%2Fescape",
	}
	for _, location := range locations {
		location := location
		t.Run(location, func(t *testing.T) {
			u := URI{Provider: ProviderGoogleDrive, ConnectionID: strings.ToUpper(testConnectionID), Location: location}
			raw := u.String()
			parsed, err := ParseURI(raw)
			if err != nil {
				t.Fatalf("ParseURI(%q): %v", raw, err)
			}
			if parsed.Provider != u.Provider || parsed.ConnectionID != testConnectionID || parsed.Location != location {
				t.Fatalf("round trip = %#v, want %#v", parsed, URI{Provider: u.Provider, ConnectionID: testConnectionID, Location: location})
			}
		})
	}
}

func TestURIRejectsCredentialCarriers(t *testing.T) {
	t.Parallel()
	bad := []string{
		"cloud://user@gdrive/" + testConnectionID,
		"cloud://gdrive:443/" + testConnectionID,
		"cloud://gdrive/" + testConnectionID + "?token=secret",
		"cloud://gdrive/" + testConnectionID + "#secret",
		"cloud://unknown/" + testConnectionID,
		"cloud://gdrive/not-a-uuid",
	}
	for _, raw := range bad {
		if _, err := ParseURI(raw); err == nil {
			t.Errorf("ParseURI(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestManagerRootURI(t *testing.T) {
	t.Parallel()
	u, err := ParseURI(ManagerRoot)
	if err != nil || u != (URI{}) || u.String() != ManagerRoot {
		t.Fatalf("manager URI = %#v, %v", u, err)
	}
}

func TestParseURIAcceptsFileOperationDirectoryMarker(t *testing.T) {
	want := URI{Provider: ProviderGoogleDrive, ConnectionID: testConnectionID, Location: googleMyLocation}
	raw := want.String() + "/"
	parsed, err := ParseURI(raw)
	if err != nil {
		t.Fatalf("ParseURI(%q): %v", raw, err)
	}
	if parsed != want {
		t.Fatalf("directory-marker URI = %#v, want %#v", parsed, want)
	}
}
