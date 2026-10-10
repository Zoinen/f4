package multiarc

import (
	"reflect"
	"testing"
)

// format.go has no dedicated test file: detectFormat is only ever exercised
// indirectly, through provider_test.go's CanOpen/Open checks on a handful
// of extensions. This file drives it directly across every suffix group it
// claims to recognize (f4#1178), plus the "recognized but the tool is
// missing" and "not recognized at all" outcomes, and the two small helpers
// it is built from.

func TestHasAnySuffix(t *testing.T) {
	if !hasAnySuffix("backup.tar.gz", ".tar.gz", ".zip") {
		t.Error("expected a match on the first suffix")
	}
	if !hasAnySuffix("archive.zip", ".tar.gz", ".zip") {
		t.Error("expected a match on the second suffix")
	}
	if hasAnySuffix("readme.txt", ".tar.gz", ".zip") {
		t.Error("expected no match")
	}
	if hasAnySuffix("x") {
		t.Error("no suffixes at all can never match")
	}
}

func TestDetectFormatTarVariants(t *testing.T) {
	withFakeTools(t, func(name string) (string, error) {
		if name == "tar" {
			return "/usr/bin/tar", nil
		}
		return "", errNotFoundStub
	}, nil)

	for _, name := range []string{
		"a.tar", "a.tar.gz", "a.tgz", "a.taz",
		"a.tar.bz2", "a.tbz2", "a.tbz", "a.tb2",
		"a.tar.xz", "a.txz",
		"a.tar.zst", "a.tzst",
		"a.tar.lz", "a.tlz",
		// detectFormat lower-cases before matching.
		"A.TAR.GZ",
	} {
		b, id, ok := detectFormat(name)
		if !ok || id != "tar" {
			t.Errorf("detectFormat(%q) = (%v, %q, %v), want a tar backend", name, b, id, ok)
			continue
		}
		if _, isTar := b.(tarBackend); !isTar {
			t.Errorf("detectFormat(%q) backend = %T, want tarBackend", name, b)
		}
	}
}

func TestDetectFormatTarGzBeforeBareGz(t *testing.T) {
	// The switch must claim "backup.tar.gz" for tar, never falling through
	// to the single-file gzip backend that ".gz" alone maps to.
	withFakeTools(t, func(name string) (string, error) {
		switch name {
		case "tar", "gzip":
			return "/usr/bin/" + name, nil
		}
		return "", errNotFoundStub
	}, nil)

	_, id, ok := detectFormat("backup.tar.gz")
	if !ok || id != "tar" {
		t.Fatalf("detectFormat(backup.tar.gz) = (id=%q, ok=%v), want tar", id, ok)
	}
}

func TestDetectFormatZipAndJar(t *testing.T) {
	withFakeTools(t, func(name string) (string, error) {
		if name == "unzip" {
			return "/usr/bin/unzip", nil
		}
		return "", errNotFoundStub
	}, nil)

	for _, name := range []string{"a.zip", "a.jar", "A.ZIP"} {
		b, id, ok := detectFormat(name)
		if !ok || id != "zip" {
			t.Errorf("detectFormat(%q) = (%v, %q, %v), want a zip backend", name, b, id, ok)
			continue
		}
		if _, isZip := b.(zipBackend); !isZip {
			t.Errorf("detectFormat(%q) backend = %T, want zipBackend", name, b)
		}
	}
}

func TestDetectFormatSevenZip(t *testing.T) {
	withFakeTools(t, func(name string) (string, error) {
		if name == "7za" {
			return "/usr/bin/7za", nil
		}
		return "", errNotFoundStub
	}, nil)

	b, id, ok := detectFormat("a.7z")
	if !ok || id != "7z" {
		t.Fatalf("detectFormat(a.7z) = (%v, %q, %v)", b, id, ok)
	}
	sz, isSeven := b.(sevenZipBackend)
	if !isSeven || sz.bin != "7za" {
		t.Errorf("detectFormat(a.7z) backend = %#v, want sevenZipBackend{bin: 7za}", b)
	}
}

func TestDetectFormatBareGzip(t *testing.T) {
	withFakeTools(t, func(name string) (string, error) {
		if name == "gzip" {
			return "/bin/gzip", nil
		}
		return "", errNotFoundStub
	}, nil)

	b, id, ok := detectFormat("access.log.gz")
	if !ok || id != "gzip" {
		t.Fatalf("detectFormat(access.log.gz) = (%v, %q, %v)", b, id, ok)
	}
	gz, isGzip := b.(gzipBackend)
	if !isGzip || gz.bin != "gzip" {
		t.Errorf("detectFormat(access.log.gz) backend = %#v, want gzipBackend{bin: gzip}", b)
	}
}

func TestDetectFormatUnrecognizedExtension(t *testing.T) {
	withFakeTools(t, func(string) (string, error) { return "/usr/bin/tar", nil }, nil)
	if _, _, ok := detectFormat("readme.txt"); ok {
		t.Error("a .txt file should not match any archive format")
	}
	if _, _, ok := detectFormat("noext"); ok {
		t.Error("a name with no extension should not match")
	}
}

// TestDetectFormatRecognizedButToolMissing covers the "known extension, no
// tool on PATH" outcome for every backend: detectFormat must report ok=false
// rather than handing back a backend that can never actually run (f4#1178:
// this is what lets ".7z files are not supported" degrade gracefully
// instead of failing the whole plugin).
func TestDetectFormatRecognizedButToolMissing(t *testing.T) {
	withFakeTools(t, func(string) (string, error) { return "", errNotFoundStub }, nil)

	for _, name := range []string{"a.tar.gz", "a.zip", "a.7z", "a.gz"} {
		if _, _, ok := detectFormat(name); ok {
			t.Errorf("detectFormat(%q) should fail when no tool for it is on PATH", name)
		}
	}
}

func TestParseBareNameListing(t *testing.T) {
	raw := "./readme.txt\r\ndir/\r\ndir/file.txt\r\n\r\nback\\slash.txt\r\n.\r\n./\r\n"
	got := parseBareNameListing([]byte(raw))
	want := []entry{
		{Path: "readme.txt", Raw: "./readme.txt"},
		{Path: "dir", Raw: "dir/", IsDir: true},
		{Path: "dir/file.txt", Raw: "dir/file.txt"},
		{Path: "back/slash.txt", Raw: "back\\slash.txt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseBareNameListing = %#v, want %#v", got, want)
	}
}

func TestParseBareNameListingEmpty(t *testing.T) {
	if got := parseBareNameListing([]byte("")); len(got) != 0 {
		t.Fatalf("empty listing should produce no entries, got %#v", got)
	}
}
