package netfox

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unxed/vtui"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// hostKeyQuestion is everything the "do you trust this host?" dialog shows.
// It carries the key already reduced to the two things a user can act on --
// its type and its fingerprint -- because that is what the other side prints
// too, and comparing the two strings is the whole point of the question.
type hostKeyQuestion struct {
	address     string // As known_hosts spells it: host, or [host]:port.
	keyType     string
	fingerprint string
	path        string // Where a yes is written.
}

// askHostKeyTrust is replaceable in tests so trust-on-first-use can be
// exercised without an interactive terminal, the same way the archive plugin
// swaps its password prompt.
var askHostKeyTrust = promptHostKeyTrust

// hostKeyPromptPoll is how often a waiting connection re-checks that the UI
// it asked is still there. It only bounds teardown, never the user.
const hostKeyPromptPoll = 250 * time.Millisecond

// hostKeyPromptGrace is how long the question waits to reach the screen
// before the connection concludes there is nobody to ask. A frame manager
// can exist without an event loop draining its task queue -- a test binary
// that only builds dialogs is in that state, and so is a UI being torn
// down -- and a posted task then never runs. It is a variable so tests do
// not have to sit through it.
var hostKeyPromptGrace = 3 * time.Second

// sshKnownHosts is the OpenSSH known_hosts database plus the policy for what
// to do when it has nothing to say about a host. knownhosts.New alone can
// only answer "known", "unknown" or "mismatch"; a first connection to a host
// nobody has ever recorded is the normal case, not a failure, so the callback
// below asks the user and records the answer.
type sshKnownHosts struct {
	verify ssh.HostKeyCallback
	path   string // The file a newly trusted key is appended to.
}

// newSSHKnownHosts reads the known_hosts files under home. Missing files are
// not an error: an empty database means every host is unknown, which is
// exactly the state a fresh account is in, and the first accepted key creates
// the file.
func newSSHKnownHosts(home string) (*sshKnownHosts, error) {
	if home == "" {
		return nil, fmt.Errorf("SSH host-key verification: home directory is empty")
	}

	sshDir := filepath.Join(home, ".ssh")
	files := make([]string, 0, 2)
	for _, name := range []string{"known_hosts", "known_hosts2"} {
		path := filepath.Join(sshDir, name)
		info, err := os.Stat(path)
		switch {
		case err == nil && !info.IsDir():
			files = append(files, path)
		case err == nil:
			return nil, fmt.Errorf("SSH host-key verification: %s is a directory", path)
		case os.IsNotExist(err):
			continue
		default:
			return nil, fmt.Errorf("SSH host-key verification: inspect %s: %w", path, err)
		}
	}

	verify, err := knownhosts.New(files...)
	if err != nil {
		return nil, fmt.Errorf("SSH host-key verification: read known_hosts: %w", err)
	}
	return &sshKnownHosts{verify: verify, path: filepath.Join(sshDir, "known_hosts")}, nil
}

// check is the ssh.HostKeyCallback. It follows what OpenSSH does on a first
// connection: a key that is already recorded passes silently, a key that
// replaces a recorded one of the same type is refused outright, and anything
// else is put to the user.
func (kh *sshKnownHosts) check(hostname string, remote net.Addr, key ssh.PublicKey) error {
	err := kh.verify(hostname, remote, key)
	if err == nil {
		return nil
	}

	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) {
		// A revoked key, or an address knownhosts could not split. Neither
		// is a question, so neither gets a dialog.
		return err
	}

	// Want lists every key recorded for this host, of every type. Only a key
	// of the type the server just offered can have been replaced; entries of
	// other types mean the host is known but this key of it is not, which
	// OpenSSH also treats as a new key rather than as an attack. Without that
	// split, a host recorded years ago under ssh-rsa would raise a tampering
	// alarm the first time it answers with ed25519.
	address := knownhosts.Normalize(hostname)
	if replaced := recordedKeysOfType(keyErr.Want, key.Type()); len(replaced) > 0 {
		return changedHostKeyError{address: address, offered: key, recorded: replaced}
	}

	trusted := askHostKeyTrust(hostKeyQuestion{
		address:     address,
		keyType:     key.Type(),
		fingerprint: ssh.FingerprintSHA256(key),
		path:        kh.path,
	})
	if !trusted {
		return fmt.Errorf("SSH host-key verification: the host key of %s (%s %s) was not trusted",
			address, key.Type(), ssh.FingerprintSHA256(key))
	}

	if err := appendKnownHost(kh.path, address, key); err != nil {
		// The user answered the question, so the connection goes through;
		// only the memory of it is lost, and the same question comes back
		// next time. OpenSSH treats an unwritable known_hosts the same way.
		vtui.DebugLog("NET: cannot record the host key of %s in %s: %v", address, kh.path, err)
	}
	return nil
}

// recordedKeysOfType picks the entries that could have been superseded by a
// key of the given type.
func recordedKeysOfType(recorded []knownhosts.KnownKey, keyType string) []knownhosts.KnownKey {
	matching := make([]knownhosts.KnownKey, 0, len(recorded))
	for _, known := range recorded {
		if known.Key != nil && known.Key.Type() == keyType {
			matching = append(matching, known)
		}
	}
	return matching
}

// changedHostKeyError reports a host whose recorded key was replaced. It
// names the file and the line so the message is actionable: knownhosts'
// own "key mismatch" leaves the user hunting for which of several files
// carries the stale entry.
type changedHostKeyError struct {
	address  string
	offered  ssh.PublicKey
	recorded []knownhosts.KnownKey
}

func (e changedHostKeyError) Error() string {
	locations := make([]string, 0, len(e.recorded))
	for _, known := range e.recorded {
		locations = append(locations, fmt.Sprintf("%s:%d", known.Filename, known.Line))
	}
	return fmt.Sprintf("SSH host-key verification: the %s host key of %s has changed (server offers %s); "+
		"if this is not an attack, remove the recorded key from %s and reconnect",
		e.offered.Type(), e.address, ssh.FingerprintSHA256(e.offered), strings.Join(locations, ", "))
}

// appendKnownHost records address under key in an OpenSSH known_hosts file,
// creating ~/.ssh and the file itself when they do not exist yet.
func appendKnownHost(path, address string, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	// O_RDWR rather than O_WRONLY: appending blind to a file whose last line
	// has no newline -- one edited by hand, or truncated -- would splice the
	// new entry onto it and cost the user both lines.
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	line := knownhosts.Line([]string{address}, key) + "\n"
	if unterminated, err := lacksFinalNewline(file); err != nil {
		_ = file.Close() // Preserve the inspection failure.
		return err
	} else if unterminated {
		line = "\n" + line
	}

	if _, err := file.WriteString(line); err != nil {
		_ = file.Close() // Preserve the write failure.
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

func lacksFinalNewline(file *os.File) (bool, error) {
	info, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", file.Name(), err)
	}
	if info.Size() == 0 {
		return false, nil
	}
	last := make([]byte, 1)
	if _, err := file.ReadAt(last, info.Size()-1); err != nil {
		return false, fmt.Errorf("read %s: %w", file.Name(), err)
	}
	return last[0] != '\n', nil
}

// promptHostKeyTrust puts the question on screen and waits for the answer.
// Waiting for the answer is unbounded on purpose -- reading a fingerprint
// takes as long as it takes -- so the dial deadline is suspended around it
// (see withHostKeyPromptDeadline). Waiting for the question to *reach* the
// screen is not: that part depends on an event loop this code cannot see.
func promptHostKeyTrust(question hostKeyQuestion) bool {
	if vtui.FrameManager == nil || vtui.FrameManager.Screen() == nil {
		// A mount from the command line, or a test binary: there is no UI to
		// answer with. Refusing keeps the connection from hanging on a
		// dialog nobody will ever see, and the error says what to do.
		return false
	}

	answer := make(chan bool, 1)
	onScreen := make(chan struct{})
	var abandoned atomic.Bool
	vtui.FrameManager.PostTask(func() {
		if abandoned.Load() {
			// The connection stopped waiting; a dialog now would be a
			// question about something that already failed.
			return
		}
		close(onScreen)
		showHostKeyDialog(question, answer)
	})

	select {
	case <-onScreen:
	case <-time.After(hostKeyPromptGrace):
		// Posting succeeded but nothing ran it, so there is no interactive
		// UI behind this frame manager after all.
		abandoned.Store(true)
		return false
	}

	for {
		select {
		case trusted := <-answer:
			return trusted
		case <-time.After(hostKeyPromptPoll):
			// A UI that goes away while the question is on screen -- the
			// task queue is torn down on shutdown -- would otherwise leave
			// this goroutine, and the half-open connection under it,
			// waiting forever.
			if vtui.FrameManager.IsShutdown() {
				return false
			}
		}
	}
}

func showHostKeyDialog(question hostKeyQuestion, answer chan<- bool) {
	text := fmt.Sprintf(vtui.Msg("NetFox.HostKeyPrompt"),
		question.address, strings.ToUpper(question.keyType), question.fingerprint, question.path)
	dlg := vtui.ShowMessageEx(
		vtui.Msg("NetFox.HostKeyTitle"),
		text,
		[]string{vtui.Msg("NetFox.HostKeyTrust"), vtui.Msg("NetFox.HostKeyRefuse")},
		vtui.MessageWarn,
	)
	if dlg == nil {
		answer <- false
		return
	}
	// Every way out of the dialog lands here, including Esc, which arrives
	// as a negative code and means the same as the second button.
	dlg.OnResult = func(code int) {
		select {
		case answer <- code == 0:
		default:
		}
	}
}
