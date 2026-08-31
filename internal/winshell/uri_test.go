package winshell

import (
	"strings"
	"testing"
)

func TestWindowsReadableURIRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		parsingName string
		wantURI     string
		wantParsing string
	}{
		{
			name:        "This PC",
			parsingName: "::" + thisPCCLSID,
			wantURI:     "windows://this-pc/",
			wantParsing: "::" + thisPCCLSID,
		},
		{
			name:        "local Unicode path",
			parsingName: `C:\Users\Иван\Idea Creative`,
			wantURI:     `windows://local/C:/Users/Иван/Idea Creative/`,
			wantParsing: `C:\Users\Иван\Idea Creative`,
		},
		{
			name:        "WSL distribution",
			parsingName: `\\wsl.localhost\Ubuntu`,
			wantURI:     `windows://linux/Ubuntu/`,
			wantParsing: `\\wsl.localhost\Ubuntu`,
		},
		{
			name:        "UNC share",
			parsingName: `\\server\share\Фото`,
			wantURI:     `windows://network/server/share/Фото/`,
			wantParsing: `\\server\share\Фото`,
		},
		{
			name:        "named Shell path",
			parsingName: `shell:AppsFolder\Microsoft.Application`,
			wantURI:     `windows://shell/AppsFolder/Microsoft.Application/`,
			wantParsing: `shell:AppsFolder\Microsoft.Application`,
		},
		{
			name:        "known namespace child",
			parsingName: "::" + networkCLSID + `\Provider\Microsoft.Networking.SSDP//uuid:1234`,
			wantURI:     `windows://namespace/network/Provider/Microsoft.Networking.SSDP%2F%2Fuuid:1234/`,
			wantParsing: "::" + networkCLSID + `\Provider\Microsoft.Networking.SSDP//uuid:1234`,
		},
		{
			name:        "generic parsing identity",
			parsingName: `root/a`,
			wantURI:     `windows://parsing/root%2Fa/`,
			wantParsing: `root/a`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := URIFromParsingName(test.parsingName)
			if raw != test.wantURI {
				t.Fatalf("URIFromParsingName(%q) = %q, want %q", test.parsingName, raw, test.wantURI)
			}
			if !strings.HasPrefix(strings.ToLower(raw), "windows://") || strings.Contains(strings.ToLower(raw), "windows://item/") {
				t.Fatalf("generated URI is not a readable Windows URI: %q", raw)
			}
			got, err := ParsingNameFromURI(raw)
			if err != nil || got != test.wantParsing {
				t.Fatalf("ParsingNameFromURI(%q) = %q, %v; want %q", raw, got, err, test.wantParsing)
			}
			if !IsURI(raw) {
				t.Fatalf("IsURI(%q) = false", raw)
			}
		})
	}
}

func TestWellKnownWindowsLocationsUseReadableAliases(t *testing.T) {
	for _, location := range shellLocationAliases {
		for _, parsingName := range []string{"::" + location.clsid, "shell:::" + location.clsid} {
			want := "windows://" + location.alias + "/"
			if got := URIFromParsingName(parsingName); got != want {
				t.Fatalf("URIFromParsingName(%q) = %q, want %q", parsingName, got, want)
			}
		}
	}

	for _, raw := range []string{"windows://home/", "windows://home", "WINDOWS://HOME/"} {
		got, err := ParsingNameFromURI(raw)
		if err != nil || got != "::"+homeCLSID {
			t.Fatalf("ParsingNameFromURI(%q) = %q, %v; want %q", raw, got, err, "::"+homeCLSID)
		}
	}
}

func TestWindowsURIRejectsMalformedAndFormerSchemes(t *testing.T) {
	for _, raw := range []string{
		"",
		"windows:item",
		"windows://item/opaque",
		"windows://new/opaque/name",
		"windows://home/child",
		"windows://local/not-a-drive/",
		"windows://parsing/too/many/segments/",
		"windows://namespace/not-a-clsid/",
		"shell://home/",
		"shell://item/Ojp7Rjg3NDMxMEUtQjZCNy00N0RDLUJDODQtQjlFNkIzOEY1OTAzfQ",
		"other://item/QQ",
	} {
		if _, err := ParsingNameFromURI(raw); err == nil {
			t.Fatalf("accepted malformed or unsupported URI %q", raw)
		}
		if IsURI(raw) {
			t.Fatalf("IsURI(%q) = true", raw)
		}
	}
}

func TestDestinationURIUsesReadableParent(t *testing.T) {
	parent := "::" + thisPCCLSID
	name := "Новая папка.txt"
	raw := DestinationURI(parent, name)
	if want := "windows://create/this-pc/" + name; raw != want {
		t.Fatalf("DestinationURI = %q, want %q", raw, want)
	}
	gotParent, gotName, err := DestinationFromURI(raw)
	if err != nil || gotParent != parent || gotName != name {
		t.Fatalf("destination round trip = (%q, %q, %v), want (%q, %q, nil)", gotParent, gotName, err, parent, name)
	}
	if !IsURI(raw) {
		t.Fatalf("IsURI(%q) = false", raw)
	}
}
