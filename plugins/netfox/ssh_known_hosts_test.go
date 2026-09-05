package netfox

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// hostKeyPromptStub records what the trust question was asked about and
// answers it the same way every time.
type hostKeyPromptStub struct {
	mu       sync.Mutex
	answer   bool
	askedFor []hostKeyQuestion
}

// stubHostKeyPrompt replaces the dialog for the duration of the test. The
// default prompt refuses without a UI, so a test that forgets this would
// still pass for the wrong reason; the stub also proves whether the question
// was asked at all.
func stubHostKeyPrompt(t *testing.T, answer bool) *hostKeyPromptStub {
	t.Helper()
	stub := &hostKeyPromptStub{answer: answer}
	previous := askHostKeyTrust
	askHostKeyTrust = func(question hostKeyQuestion) bool {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		stub.askedFor = append(stub.askedFor, question)
		return stub.answer
	}
	t.Cleanup(func() { askHostKeyTrust = previous })
	return stub
}

func (s *hostKeyPromptStub) questions() []hostKeyQuestion {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]hostKeyQuestion(nil), s.askedFor...)
}

func TestSSHHostKeyCallbackRecordsTrustedHostWithoutKnownHostsFile(t *testing.T) {
	home := t.TempDir()
	prompt := stubHostKeyPrompt(t, true)
	key := testSSHHostKey(t)

	callback, err := sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatalf("a home without known_hosts must not fail: %v", err)
	}
	if err := callback("example.test:2222", testSSHRemoteAddr{}, key); err != nil {
		t.Fatalf("trusted host key rejected: %v", err)
	}

	asked := prompt.questions()
	if len(asked) != 1 {
		t.Fatalf("trust questions asked = %d, want 1", len(asked))
	}
	if asked[0].address != "[example.test]:2222" {
		t.Errorf("question address = %q, want the known_hosts spelling", asked[0].address)
	}
	if asked[0].fingerprint != ssh.FingerprintSHA256(key) {
		t.Errorf("question fingerprint = %q, want %q", asked[0].fingerprint, ssh.FingerprintSHA256(key))
	}
	if asked[0].keyType != key.Type() {
		t.Errorf("question key type = %q, want %q", asked[0].keyType, key.Type())
	}

	// The point of the exercise: the answer outlives the connection, so a
	// second one no longer asks.
	path := filepath.Join(home, ".ssh", "known_hosts")
	if asked[0].path != path {
		t.Errorf("question path = %q, want %q", asked[0].path, path)
	}
	assertKnownHostsPermissions(t, home)

	callback, err = sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("example.test:2222", testSSHRemoteAddr{}, key); err != nil {
		t.Fatalf("recorded host key rejected on the next connection: %v", err)
	}
	if asked := prompt.questions(); len(asked) != 1 {
		t.Fatalf("trust questions asked = %d, want the recorded key to be silent", len(asked))
	}
}

func TestSSHHostKeyCallbackRefusedHostIsNotRecorded(t *testing.T) {
	home := t.TempDir()
	prompt := stubHostKeyPrompt(t, false)

	callback, err := sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	err = callback("example.test:2222", testSSHRemoteAddr{}, testSSHHostKey(t))
	if err == nil {
		t.Fatal("a refused host key was accepted")
	}
	if !strings.Contains(err.Error(), "[example.test]:2222") {
		t.Errorf("refusal error = %v, want the host in it", err)
	}
	if len(prompt.questions()) != 1 {
		t.Fatal("the user was not asked")
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "known_hosts")); !os.IsNotExist(err) {
		t.Errorf("known_hosts after a refusal: %v, want it not to exist", err)
	}
}

func TestSSHHostKeyCallbackRefusesChangedKeyWithoutAsking(t *testing.T) {
	home := t.TempDir()
	recorded := testSSHHostKey(t)
	writeKnownHosts(t, home, "[example.test]:2222", recorded)
	prompt := stubHostKeyPrompt(t, true)

	callback, err := sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	err = callback("example.test:2222", testSSHRemoteAddr{}, testSSHHostKey(t))
	if err == nil {
		t.Fatal("a changed host key was accepted")
	}
	if asked := prompt.questions(); len(asked) != 0 {
		t.Fatalf("a changed key raised %d trust questions, want none", len(asked))
	}
	// The message has to say which line to fix; "key mismatch" alone leaves
	// the user hunting through the file.
	if !strings.Contains(err.Error(), "known_hosts:1") {
		t.Errorf("changed-key error = %v, want the offending file and line", err)
	}
}

func TestSSHHostKeyCallbackAsksForAKnownHostWithANewKeyType(t *testing.T) {
	home := t.TempDir()
	writeKnownHosts(t, home, "[example.test]:2222", testSSHHostKey(t))
	prompt := stubHostKeyPrompt(t, true)

	callback, err := sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	// A host recorded under one algorithm answering with another is a new
	// key, not a replaced one: OpenSSH asks rather than crying tampering.
	rsaKey := testSSHRSAHostKey(t)
	if err := callback("example.test:2222", testSSHRemoteAddr{}, rsaKey); err != nil {
		t.Fatalf("a host key of an unrecorded type was rejected: %v", err)
	}
	if asked := prompt.questions(); len(asked) != 1 {
		t.Fatalf("trust questions asked = %d, want 1", len(asked))
	}

	callback, err = sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("example.test:2222", testSSHRemoteAddr{}, rsaKey); err != nil {
		t.Fatalf("the newly recorded key was rejected: %v", err)
	}
}

func TestAppendKnownHostKeepsAnUnterminatedFileParsable(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sshDir, "known_hosts")
	first := testSSHHostKey(t)
	// No trailing newline, as a hand-edited or truncated file has.
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{"[first.test]:2222"}, first)), 0o600); err != nil {
		t.Fatal(err)
	}

	second := testSSHHostKey(t)
	if err := appendKnownHost(path, "[second.test]:2222", second); err != nil {
		t.Fatalf("append to an unterminated known_hosts: %v", err)
	}

	callback, err := knownhosts.New(path)
	if err != nil {
		t.Fatalf("known_hosts is no longer parsable: %v", err)
	}
	if err := callback("first.test:2222", testSSHRemoteAddr{}, first); err != nil {
		t.Errorf("the entry that had no newline was lost: %v", err)
	}
	if err := callback("second.test:2222", testSSHRemoteAddr{}, second); err != nil {
		t.Errorf("the appended entry is unusable: %v", err)
	}
}

func TestPromptHostKeyTrustRefusesWithoutAUIToAsk(t *testing.T) {
	// Two ways to have no interactive UI, and both have to end in a refusal
	// rather than in a connection that waits forever. `f4 --mount` never
	// builds a screen at all; this test binary does (dialog tests need one)
	// but runs no event loop, so a posted task never executes -- the same
	// state a UI being torn down is in.
	previousGrace := hostKeyPromptGrace
	hostKeyPromptGrace = 50 * time.Millisecond
	t.Cleanup(func() { hostKeyPromptGrace = previousGrace })

	question := hostKeyQuestion{address: "example.test", path: "known_hosts"}
	if promptHostKeyTrust(question) {
		t.Fatal("the prompt trusted a host with no UI to ask")
	}
}

func assertKnownHostsPermissions(t *testing.T, home string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return // Windows carries ACLs, not mode bits.
	}
	sshDir := filepath.Join(home, ".ssh")
	dirInfo, err := os.Stat(sshDir)
	if err != nil {
		t.Fatalf("~/.ssh was not created: %v", err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0o700 {
		t.Errorf("~/.ssh mode = %04o, want 0700", mode)
	}
	fileInfo, err := os.Stat(filepath.Join(sshDir, "known_hosts"))
	if err != nil {
		t.Fatalf("known_hosts was not created: %v", err)
	}
	if mode := fileInfo.Mode().Perm(); mode != 0o600 {
		t.Errorf("known_hosts mode = %04o, want 0600", mode)
	}
}

func testSSHRSAHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(&private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
