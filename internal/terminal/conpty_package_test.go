package terminal

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fakeConPTYPackage builds a package shaped like the real nupkg, serves it,
// and returns a manifest that describes it.
func fakeConPTYPackage(t *testing.T) (conPTYPackage, map[string][]byte, *int32) {
	t.Helper()
	members := map[string][]byte{
		"runtimes/win-x64/native/conpty.dll":        []byte("fake conpty.dll"),
		"build/native/runtimes/x64/OpenConsole.exe": []byte("fake OpenConsole.exe"),
		"runtimes/win-x64/native/other.dll":         []byte("not taken"),
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range members {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	pkg := conPTYPackage{
		Version: "test-version",
		URL:     srv.URL + "/package.nupkg",
		SHA256:  sha256Hex(data),
		Runtime: "win-x64",
		Files: []conPTYPackageFile{
			{Name: "conpty.dll", Entry: "runtimes/win-x64/native/conpty.dll", SHA256: sha256Hex(members["runtimes/win-x64/native/conpty.dll"])},
			{Name: "OpenConsole.exe", Entry: "build/native/runtimes/x64/OpenConsole.exe", SHA256: sha256Hex(members["build/native/runtimes/x64/OpenConsole.exe"])},
		},
	}
	return pkg, members, &requests
}

func TestConPTYPackageInstallsVerifiesAndReuses(t *testing.T) {
	pkg, members, requests := fakeConPTYPackage(t)
	root := t.TempDir()

	dir, err := installConPTYPackage(context.Background(), http.DefaultClient, root, pkg)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if want := conPTYPackageDir(root, pkg); dir != want {
		t.Fatalf("installed in %q, want %q", dir, want)
	}
	for name, entry := range map[string]string{"conpty.dll": "runtimes/win-x64/native/conpty.dll", "OpenConsole.exe": "build/native/runtimes/x64/OpenConsole.exe"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, members[entry]) {
			t.Fatalf("%s = %q, %v; want %q", name, got, err, members[entry])
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "other.dll")); !os.IsNotExist(err) {
		t.Fatalf("a file the manifest does not name was installed: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Fatalf("staging directory %q was left behind", e.Name())
		}
	}

	if _, err := installConPTYPackage(context.Background(), http.DefaultClient, root, pkg); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if n := atomic.LoadInt32(requests); n != 1 {
		t.Fatalf("an installed package was downloaded again: %d requests", n)
	}

	// A damaged copy is not trusted: it is replaced, not used.
	if err := os.WriteFile(filepath.Join(dir, "OpenConsole.exe"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if verifyConPTYPackage(dir, pkg) == nil {
		t.Fatal("a tampered file passed verification")
	}
	if _, err := installConPTYPackage(context.Background(), http.DefaultClient, root, pkg); err != nil {
		t.Fatalf("reinstall over a damaged copy: %v", err)
	}
	if err := verifyConPTYPackage(dir, pkg); err != nil {
		t.Fatalf("after reinstall: %v", err)
	}
}

func TestConPTYPackageRejectsWrongBytes(t *testing.T) {
	pkg, _, _ := fakeConPTYPackage(t)
	root := t.TempDir()

	wrongPackage := pkg
	wrongPackage.SHA256 = strings.Repeat("0", 64)
	if _, err := installConPTYPackage(context.Background(), http.DefaultClient, root, wrongPackage); err == nil {
		t.Fatal("a package with the wrong SHA-256 was installed")
	}

	wrongMember := pkg
	wrongMember.Files = append([]conPTYPackageFile(nil), pkg.Files...)
	wrongMember.Files[1].SHA256 = strings.Repeat("0", 64)
	if _, err := installConPTYPackage(context.Background(), http.DefaultClient, root, wrongMember); err == nil {
		t.Fatal("a package member with the wrong SHA-256 was installed")
	}

	missing := pkg
	missing.Files = append([]conPTYPackageFile(nil), pkg.Files...)
	missing.Files[0].Entry = "runtimes/win-x64/native/absent.dll"
	if _, err := installConPTYPackage(context.Background(), http.DefaultClient, root, missing); err == nil {
		t.Fatal("a package without a named member was installed")
	}
	if _, err := os.Stat(conPTYPackageDir(root, pkg)); !os.IsNotExist(err) {
		t.Fatalf("a rejected package left a directory behind: %v", err)
	}
}

func TestConPTYPackageManifestCoversWindowsArchitectures(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64", "386"} {
		pkg, ok := conPTYPackageFor(arch)
		if !ok {
			t.Fatalf("no ConPTY package for %s", arch)
		}
		if len(pkg.Files) != 2 || pkg.SHA256 != conPTYPackageSHA256 {
			t.Fatalf("%s: unexpected manifest %+v", arch, pkg)
		}
		for _, f := range pkg.Files {
			if len(f.SHA256) != 64 || !strings.HasSuffix(f.Entry, "/"+f.Name) {
				t.Fatalf("%s: malformed file entry %+v", arch, f)
			}
		}
	}
	if _, ok := conPTYPackageFor("riscv64"); ok {
		t.Fatal("a package was claimed for an architecture it does not ship")
	}
}
