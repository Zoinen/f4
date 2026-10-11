package multiarc

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestZipBackendList(t *testing.T) {
	var gotArgs []string
	withFakeTools(t, nil, func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		gotArgs = args
		return []byte("readme.txt\nsub/\nsub/data.bin\n"), nil, nil
	})
	entries, err := zipBackend{}.list(context.Background(), "/a.zip")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !reflect.DeepEqual(gotArgs, []string{"-Z1", "/a.zip"}) {
		t.Fatalf("args = %v", gotArgs)
	}
	want := []entry{
		{Path: "readme.txt", Raw: "readme.txt"},
		{Path: "sub", Raw: "sub/", IsDir: true},
		{Path: "sub/data.bin", Raw: "sub/data.bin"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries = %#v, want %#v", entries, want)
	}
}

func TestZipBackendExtractOne(t *testing.T) {
	var gotArgs []string
	withFakeTools(t, nil, func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		gotArgs = args
		return nil, nil, nil
	})
	if err := (zipBackend{}).extractOne(context.Background(), "/a.zip", "/dest", "sub/data.bin"); err != nil {
		t.Fatalf("extractOne: %v", err)
	}
	want := []string{"-o", "-q", "/a.zip", "sub/data.bin", "-d", "/dest"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

func TestUnzipLiteral(t *testing.T) {
	cases := map[string]string{
		"plain/name.txt": "plain/name.txt",
		"dir/[ab].txt":   "dir/[[]ab].txt",
		"w*ld?.txt":      "w[*]ld[?].txt",
		"-dash.txt":      "[-]dash.txt",
		"a-b/-c":         "a-b/-c",
		`back\slash`:     `back\\slash`,
	}
	for in, want := range cases {
		if got := unzipLiteral(in); got != want {
			t.Errorf("unzipLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

// unzip matches member arguments as wildcard patterns, so a member called
// "dir/[ab].txt" or "-dash.txt" is asked for through unzipLiteral.
func TestZipBackendExtractOneLiteralName(t *testing.T) {
	var gotArgs []string
	withFakeTools(t, nil, func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		gotArgs = args
		return nil, nil, nil
	})
	if err := (zipBackend{}).extractOne(context.Background(), "/a.zip", "/dest", "-dir/[ab].txt"); err != nil {
		t.Fatalf("extractOne: %v", err)
	}
	want := []string{"-o", "-q", "/a.zip", "[-]dir/[[]ab].txt", "-d", "/dest"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

func TestZipBackendExtractAll(t *testing.T) {
	var gotArgs []string
	withFakeTools(t, nil, func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		gotArgs = args
		return nil, nil, nil
	})
	if err := (zipBackend{}).extractAll(context.Background(), "/a.zip", "/dest"); err != nil {
		t.Fatalf("extractAll: %v", err)
	}
	want := []string{"-o", "-q", "/a.zip", "-d", "/dest"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}

// A zip with no members at all -- what "zip -d" leaves behind once the last
// member is gone -- makes unzip -Z1 exit 1 with "Empty zipfile." on stdout.
// That is an empty listing, not a broken archive.
func TestZipBackendListEmptyZipfile(t *testing.T) {
	withFakeTools(t, nil, func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		return []byte("Empty zipfile.\n"), nil, errors.New("exit status 1")
	})
	entries, err := zipBackend{}.list(context.Background(), "/empty.zip")
	if err != nil {
		t.Fatalf("list of an empty zip: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %#v, want none", entries)
	}
}

func TestZipBackendListDamagedZipStillFails(t *testing.T) {
	withFakeTools(t, nil, func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		return []byte("Empty zipfile.\n"), []byte("unzip: cannot find zipfile directory"), errors.New("exit status 9")
	})
	if _, err := (zipBackend{}).list(context.Background(), "/bad.zip"); err == nil {
		t.Fatal("expected an error when unzip explains a failure on stderr")
	}
}
