package vtvibe

import (
	"strings"
	"testing"
)

func TestPackEmptyAndDeterministic(t *testing.T) {
	if got := NewSession().Pack(); got != "" {
		t.Fatalf("empty session pack = %q; want empty", got)
	}

	makeSession := func() *Session {
		s := NewSession()
		writeFile := func(name string, data []byte) {
			if err := s.tree.writeFile(name, data); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		writeFile("/ctx/readme.txt", []byte("hello"))
		writeFile("/ctx/sub/with-newline", []byte("world\n"))
		writeFile("/ctx/blob.bin", []byte{'x', 0, 'y'})
		writeFile("/ctx/.env", []byte("API_KEY=secret"))
		writeFile("/ctx/project/.ssh/id", []byte("private"))
		writeFile("/ctx/certificate.pem", []byte("private"))
		return s
	}

	first := makeSession().Pack()
	second := makeSession().Pack()
	if first != second {
		t.Fatal("pack output is not deterministic")
	}
	for _, want := range []string{
		"files: 3\n",
		"bytes: 14\n",
		"=== BEGIN blob.bin ===\n<binary, 3 bytes, sha256:",
		"=== BEGIN readme.txt ===\nhello\n=== END readme.txt ===",
		"=== BEGIN sub/with-newline ===\nworld\n=== END sub/with-newline ===",
		"skipped (looks like a secret): .env, certificate.pem, project/.ssh/id",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("pack output missing %q:\n%s", want, first)
		}
	}
	if strings.Contains(first, "API_KEY=secret") || strings.Contains(first, "private") {
		t.Errorf("secret contents leaked into pack:\n%s", first)
	}
}

func TestPackSecretAndBinaryRules(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: ".env", want: true},
		{name: ".env.local", want: true},
		{name: "id_rsa", want: true},
		{name: "certificate.PEM", want: true},
		{name: "project/.ssh/config", want: true},
		{name: "notes.pem.bak", want: false},
		{name: "notes.txt", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := looksSecret(test.name); got != test.want {
				t.Errorf("looksSecret(%q) = %v; want %v", test.name, got, test.want)
			}
		})
	}

	if isBinary([]byte("plain text")) {
		t.Error("plain text detected as binary")
	}
	if !isBinary([]byte{'a', 0, 'b'}) {
		t.Error("NUL-containing data not detected as binary")
	}
	if isBinary(append([]byte(strings.Repeat("x", 8193)), 0)) {
		t.Error("NUL after the first 8 KiB detected as binary")
	}
}
