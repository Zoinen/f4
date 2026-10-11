//go:build !lite

package netfox

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh/knownhosts"
)

// These tests target the parts of ssh_known_hosts.go that
// TestSSHHostKeyCallback* in ssh_known_hosts_test.go does not reach: the
// discovery errors in newSSHKnownHosts, the address-splitting and per-file
// fallback paths in verifyFor, the known_hosts line filter and its marker
// handling, and the small pure helpers (recordedKeysOfType,
// remapKnownHostsError, lacksFinalNewline).

func TestNewSSHKnownHostsRejectsEmptyHome(t *testing.T) {
	if _, err := newSSHKnownHosts(""); err == nil {
		t.Fatal("an empty home directory must be rejected")
	} else if !strings.Contains(err.Error(), "home directory is empty") {
		t.Fatalf("err = %v, want a complaint about the empty home directory", err)
	}
}

func TestNewSSHKnownHostsRejectsDirectoryNamedKnownHosts(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(sshDir, "known_hosts"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := newSSHKnownHosts(home)
	if err == nil {
		t.Fatal("known_hosts being a directory must be rejected")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("err = %v, want a complaint that known_hosts is a directory", err)
	}
}

func TestNewSSHKnownHostsReportsUnreadableSSHDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// No execute bit: os.Stat on a file inside can no longer traverse the
	// directory, so the lookup fails with something other than "not exist".
	if err := os.Chmod(sshDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sshDir, 0o700) })

	_, err := newSSHKnownHosts(home)
	if err == nil {
		t.Fatal("an unreadable ~/.ssh must not be silently treated as empty")
	}
	if !strings.Contains(err.Error(), "inspect") {
		t.Fatalf("err = %v, want the inspection failure to be reported", err)
	}
}

// check() must return an unrelated I/O error (a missing known_hosts file
// discovered too late, after the fast path already believed it existed)
// wrapped with its own message, rather than silently asking the user.
func TestSSHKnownHostsCheckReportsVerifyForFailure(t *testing.T) {
	kh := &sshKnownHosts{files: []string{filepath.Join(t.TempDir(), "does-not-exist")}}
	prompt := stubHostKeyPrompt(t, true)

	err := kh.check("example.test:22", testSSHRemoteAddr{}, testSSHHostKey(t))
	if err == nil {
		t.Fatal("a missing known_hosts file must not verify silently")
	}
	if !strings.Contains(err.Error(), "read known_hosts") {
		t.Fatalf("err = %v, want the read failure to be named", err)
	}
	if len(prompt.questions()) != 0 {
		t.Fatal("a lookup failure is not a trust question")
	}
}

// A key that a known_hosts file has revoked is refused outright: it is not a
// *knownhosts.KeyError, so it must skip the trust dialog entirely, the same
// way a "changed key" does.
func TestSSHHostKeyCallbackRefusesRevokedKeyWithoutAsking(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	key := testSSHHostKey(t)
	line := "@revoked " + knownhosts.Line([]string{"*"}, key) + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt := stubHostKeyPrompt(t, true)

	callback, err := sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	err = callback("example.test:2222", testSSHRemoteAddr{}, key)
	if err == nil {
		t.Fatal("a revoked host key was accepted")
	}
	if asked := prompt.questions(); len(asked) != 0 {
		t.Fatalf("a revoked key raised %d trust questions, want none", len(asked))
	}
}

func TestVerifyForNeedsAHostnameOrARemoteAddress(t *testing.T) {
	kh := &sshKnownHosts{}
	if _, err := kh.verifyFor("", nil); err == nil {
		t.Fatal("verifyFor with neither a hostname nor a remote address must fail")
	}
	if _, err := kh.verifyFor("", testSSHRemoteAddr{}); err != nil {
		t.Fatalf("verifyFor must fall back to the remote address: %v", err)
	}
}

func TestVerifyForPropagatesUnreadableFile(t *testing.T) {
	kh := &sshKnownHosts{files: []string{filepath.Join(t.TempDir(), "gone")}}
	if _, err := kh.verifyFor("example.test", testSSHRemoteAddr{}); err == nil {
		t.Fatal("a missing source file must not be silently skipped")
	}
}

func TestVerifyForPropagatesTempFileCreationFailure(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "known_hosts")
	if err := os.WriteFile(source, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(home, "no-such-tmp-dir")
	t.Setenv("TMPDIR", bad)
	t.Setenv("TMP", bad)
	t.Setenv("TEMP", bad)

	kh := &sshKnownHosts{files: []string{source}}
	if _, err := kh.verifyFor("example.test", testSSHRemoteAddr{}); err == nil {
		t.Fatal("a broken temp directory must surface as an error")
	}
}

func TestRecordedKeysOfType(t *testing.T) {
	ed := testSSHHostKey(t)
	rsa := testSSHRSAHostKey(t)
	recorded := []knownhosts.KnownKey{
		{Filename: "known_hosts", Line: 1, Key: ed},
		{Filename: "known_hosts", Line: 2, Key: rsa},
		{Filename: "known_hosts", Line: 3, Key: nil}, // must not panic
	}

	got := recordedKeysOfType(recorded, ed.Type())
	if len(got) != 1 || got[0].Line != 1 {
		t.Fatalf("recordedKeysOfType(%s) = %+v, want just the matching entry", ed.Type(), got)
	}

	if got := recordedKeysOfType(nil, ed.Type()); len(got) != 0 {
		t.Fatalf("recordedKeysOfType(nil, ...) = %+v, want none", got)
	}

	if got := recordedKeysOfType(recorded, "ssh-nonexistent-type"); len(got) != 0 {
		t.Fatalf("recordedKeysOfType with no matching type = %+v, want none", got)
	}
}

func TestRemapKnownHostsErrorNilIsNil(t *testing.T) {
	if err := remapKnownHostsError(nil, nil); err != nil {
		t.Fatalf("remapKnownHostsError(nil, ...) = %v, want nil", err)
	}
}

func TestRemapKnownHostsErrorRewritesKeyErrorFilenames(t *testing.T) {
	sources := map[string]string{"/tmp/f4-known-hosts-123": "/home/user/.ssh/known_hosts"}
	in := &knownhosts.KeyError{Want: []knownhosts.KnownKey{{Filename: "/tmp/f4-known-hosts-123", Line: 7}}}

	out := remapKnownHostsError(in, sources)
	var keyErr *knownhosts.KeyError
	if !errors.As(out, &keyErr) {
		t.Fatalf("remapped error is not a *KeyError: %v", out)
	}
	if len(keyErr.Want) != 1 || keyErr.Want[0].Filename != "/home/user/.ssh/known_hosts" || keyErr.Want[0].Line != 7 {
		t.Fatalf("got %+v, want the source path and the line preserved", keyErr.Want)
	}
	// The original must not be mutated: filterKnownHosts's callers reuse it.
	if in.Want[0].Filename != "/tmp/f4-known-hosts-123" {
		t.Fatalf("remapKnownHostsError mutated its input: %+v", in.Want)
	}
}

func TestRemapKnownHostsErrorRewritesRevokedErrorFilename(t *testing.T) {
	sources := map[string]string{"/tmp/f4-known-hosts-9": "/home/user/.ssh/known_hosts"}
	in := &knownhosts.RevokedError{Revoked: knownhosts.KnownKey{Filename: "/tmp/f4-known-hosts-9", Line: 3}}

	out := remapKnownHostsError(in, sources)
	var revokedErr *knownhosts.RevokedError
	if !errors.As(out, &revokedErr) {
		t.Fatalf("remapped error is not a *RevokedError: %v", out)
	}
	if revokedErr.Revoked.Filename != "/home/user/.ssh/known_hosts" || revokedErr.Revoked.Line != 3 {
		t.Fatalf("got %+v, want the source path and the line preserved", revokedErr.Revoked)
	}
}

func TestRemapKnownHostsErrorRewritesPlainMessages(t *testing.T) {
	sources := map[string]string{"/tmp/f4-known-hosts-5": "/home/user/.ssh/known_hosts"}
	in := errors.New("open /tmp/f4-known-hosts-5: permission denied")

	out := remapKnownHostsError(in, sources)
	if strings.Contains(out.Error(), "/tmp/f4-known-hosts-5") {
		t.Fatalf("temp path leaked into the error: %v", out)
	}
	if !strings.Contains(out.Error(), "/home/user/.ssh/known_hosts") {
		t.Fatalf("err = %v, want the real path substituted in", out)
	}
}

func TestFilterKnownHostsKeepsCommentsAndAnUnterminatedFinalLine(t *testing.T) {
	key := testSSHHostKey(t)
	kept := knownhosts.Line([]string{"kept.test:2222"}, key)
	// The last line of the input has no trailing newline, as a hand-edited
	// or truncated file would.
	data := "# a comment\n" + kept + "\n" + "unrelated.test:2222 ssh-ed25519 not-actually-used"

	var out bytes.Buffer
	if err := filterKnownHosts(&out, []byte(data), "kept.test:2222"); err != nil {
		t.Fatalf("filterKnownHosts: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "# a comment\n") {
		t.Errorf("the comment line was dropped: %q", got)
	}
	if !strings.Contains(got, kept) {
		t.Errorf("the matching line was dropped: %q", got)
	}
	if !strings.Contains(got, "# ignored by f4\n") {
		t.Errorf("the unrelated, unterminated line was not replaced: %q", got)
	}
}

// failAfterWriter fails starting from its (1-based) nth call to Write, so a
// test can choose exactly which of filterKnownHosts's two write sites is
// exercised.
type failAfterWriter struct {
	failFrom int
	calls    int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls >= w.failFrom {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

func TestFilterKnownHostsPropagatesWriteFailureOnAKeptLine(t *testing.T) {
	key := testSSHHostKey(t)
	data := knownhosts.Line([]string{"kept.test:2222"}, key) + "\n"
	w := &failAfterWriter{failFrom: 1}
	if err := filterKnownHosts(w, []byte(data), "kept.test:2222"); err == nil {
		t.Fatal("a write failure on a kept line must be reported")
	}
}

func TestFilterKnownHostsPropagatesWriteFailureOnAnIgnoredLine(t *testing.T) {
	data := "unrelated.test:2222 ssh-ed25519 not-actually-used\n"
	w := &failAfterWriter{failFrom: 1}
	if err := filterKnownHosts(w, []byte(data), "kept.test:2222"); err == nil {
		t.Fatal("a write failure while replacing an ignored line must be reported")
	}
}

func TestKnownHostLineAppliesEdgeCases(t *testing.T) {
	key := testSSHHostKey(t)
	// A non-default port keeps knownhosts.Normalize from collapsing the host
	// pattern down to a bare hostname, so the pattern below unambiguously
	// names one host and one host only.
	pattern := knownhosts.Line([]string{"match.test:2222"}, key)
	revokedLine := fmt.Sprintf("@revoked match.test:2222 %s %s",
		key.Type(), base64.StdEncoding.EncodeToString(key.Marshal()))

	cases := []struct {
		name    string
		line    string
		address string
		want    bool
	}{
		{"blank line", "", "match.test:2222", false},
		{"cert-authority marker without a pattern", "@cert-authority", "match.test:2222", true},
		{"revoked marker always applies", revokedLine, "unrelated.test:2222", true},
		{"unknown marker always applies", "@unknown match.test:2222", "unrelated.test:2222", true},
		{"matching host pattern applies", pattern, "match.test:2222", true},
		{"non-matching host pattern does not apply", pattern, "unrelated.test:2222", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := knownHostLineApplies(tc.line, tc.address); got != tc.want {
				t.Fatalf("knownHostLineApplies(%q, %q) = %v, want %v", tc.line, tc.address, got, tc.want)
			}
		})
	}
}

func TestKnownHostLineAppliesDefaultsToTrueWhenTheProbeCannotBeCreated(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-tmp-dir")
	t.Setenv("TMPDIR", bad)
	t.Setenv("TMP", bad)
	t.Setenv("TEMP", bad)

	key := testSSHHostKey(t)
	pattern := knownhosts.Line([]string{"match.test:2222"}, key)
	// Without a place to write the probe file, the line has to be kept
	// rather than silently dropped from the filtered known_hosts.
	if !knownHostLineApplies(pattern, "unrelated.test:2222") {
		t.Fatal("a line whose applicability could not be probed must be kept")
	}
}

func TestLacksFinalNewline(t *testing.T) {
	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "known_hosts")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		got, err := lacksFinalNewline(f)
		if err != nil || got {
			t.Fatalf("lacksFinalNewline(empty) = %v, %v, want false, nil", got, err)
		}
	})

	t.Run("terminated file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "known_hosts")
		if err := os.WriteFile(path, []byte("line\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		got, err := lacksFinalNewline(f)
		if err != nil || got {
			t.Fatalf("lacksFinalNewline(terminated) = %v, %v, want false, nil", got, err)
		}
	})

	t.Run("unterminated file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "known_hosts")
		if err := os.WriteFile(path, []byte("line"), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		got, err := lacksFinalNewline(f)
		if err != nil || !got {
			t.Fatalf("lacksFinalNewline(unterminated) = %v, %v, want true, nil", got, err)
		}
	})
}
