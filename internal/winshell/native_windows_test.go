//go:build windows

package winshell

import (
	"errors"
	"strings"
	"testing"

	"github.com/zzl/go-win32api/v2/win32"
)

func TestShellEnumerationEnd(t *testing.T) {
	for _, value := range []uint32{1, 0x80070012, 0x80070103, 0x800710d2} {
		if !isShellEnumerationEnd(win32.HRESULT(value)) {
			t.Fatalf("HRESULT %08x was not recognized as enumeration completion", value)
		}
	}
	for _, value := range []uint32{0, 0x80004005, 0x80070005} {
		if isShellEnumerationEnd(win32.HRESULT(value)) {
			t.Fatalf("HRESULT %08x was incorrectly recognized as enumeration completion", value)
		}
	}
}

func TestGalleryEmptyMeansIndexingRequired(t *testing.T) {
	emptyBits := uint32(0x800710d2)
	empty := win32.HRESULT(emptyBits)
	if !isGalleryIndexingRequired("shell:::"+galleryCLSID, empty) {
		t.Fatal("Gallery ERROR_EMPTY was not recognized as an indexing requirement")
	}
	if isGalleryIndexingRequired("shell:::"+networkCLSID, empty) {
		t.Fatal("Network ERROR_EMPTY was incorrectly classified as a Gallery indexing requirement")
	}
	if isGalleryIndexingRequired("shell:::"+galleryCLSID, win32.S_FALSE) {
		t.Fatal("an ordinary empty Gallery was incorrectly classified as an indexing requirement")
	}
}

func TestGalleryEnumerationStatusRoundTrip(t *testing.T) {
	encoded, err := encodeEnumerationResponse(nil, ErrGalleryIndexingRequired)
	if err != nil {
		t.Fatalf("encode Gallery indexing requirement: %v", err)
	}
	response, ok := encoded.(enumerateResponse)
	if !ok {
		t.Fatalf("encoded response has type %T", encoded)
	}
	items, err := decodeEnumerationResponse(response, nil)
	if len(items) != 0 || !errors.Is(err, ErrGalleryIndexingRequired) {
		t.Fatalf("decoded items=%#v err=%v", items, err)
	}
}

func TestCLSIDRoot(t *testing.T) {
	for _, parsingName := range []string{
		"shell:::" + networkCLSID,
		"::" + networkCLSID,
		" SHELL:::" + strings.ToLower(networkCLSID) + " ",
	} {
		if !isCLSIDRoot(parsingName, networkCLSID) {
			t.Fatalf("%q was not recognized as the Network root", parsingName)
		}
	}
	if isCLSIDRoot("::"+networkCLSID+"\\child", networkCLSID) {
		t.Fatal("a Network descendant was incorrectly recognized as the root")
	}
}

func TestCollectEnumerationSnapshotsMergesLateResults(t *testing.T) {
	snapshots := [][]Node{
		{{Name: "TV", ParsingName: "ssdp:tv"}},
		{{Name: "TV", ParsingName: "SSDP:TV"}, {Name: "HC", ParsingName: `\\HC`}},
		{{Name: "HC", ParsingName: `\\hc`}, {Name: "Printer", URI: "windows://network/printer/"}},
	}
	calls := 0
	pauses := 0
	items, err := collectEnumerationSnapshots(len(snapshots), func() { pauses++ }, func() ([]Node, error) {
		items := snapshots[calls]
		calls++
		return items, nil
	})
	if err != nil {
		t.Fatalf("collect snapshots: %v", err)
	}
	if calls != 3 || pauses != 2 {
		t.Fatalf("calls=%d pauses=%d, want 3 and 2", calls, pauses)
	}
	if len(items) != 3 || items[0].Name != "TV" || items[1].Name != "HC" || items[2].Name != "Printer" {
		t.Fatalf("merged items = %#v", items)
	}
}

func TestCollectEnumerationSnapshotsKeepsPartialResultsOnLateError(t *testing.T) {
	calls := 0
	items, err := collectEnumerationSnapshots(3, nil, func() ([]Node, error) {
		calls++
		if calls == 1 {
			return []Node{{Name: "HC", ParsingName: `\\HC`}}, nil
		}
		return nil, errors.New("provider stopped")
	})
	if err != nil || len(items) != 1 || items[0].Name != "HC" {
		t.Fatalf("partial items=%#v err=%v", items, err)
	}
}
