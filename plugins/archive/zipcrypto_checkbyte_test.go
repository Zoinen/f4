package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/zip"
)

// zipCryptoCollisionFixture builds a ZipCrypto archive (Info-ZIP `zip -P`)
// whose one-byte password check happens to accept the wrong password too, so
// that reading the member fails on the CRC instead of at the check. Info-ZIP
// stores the check byte as the high byte of the DOS modification time, so
// the fixture is regenerated over the hour/minute combinations until the
// collision appears; without a collision the test is skipped.
func zipCryptoCollisionFixture(t *testing.T, correct, wrong string) string {
	t.Helper()
	if _, err := exec.LookPath("zip"); err != nil {
		t.Skip("zip command is not installed")
	}
	tmp := t.TempDir()
	member := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(member, []byte("secret data"), 0600); err != nil {
		t.Fatal(err)
	}
	for hour := 0; hour < 24; hour++ {
		for minute := 0; minute < 60; minute += 8 {
			p := filepath.Join(tmp, fmt.Sprintf("s-%02d%02d.zip", hour, minute))
			touch := exec.Command("touch", "-t", fmt.Sprintf("20200101%02d%02d", hour, minute), member)
			if out, err := touch.CombinedOutput(); err != nil {
				t.Fatalf("touch: %v: %s", err, out)
			}
			cmd := exec.Command("zip", "-q", "-P", correct, p, "secret.txt")
			cmd.Dir = tmp
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Skipf("create zip fixture: %v: %s", err, out)
			}
			if zipCryptoCheckByteAccepts(t, p, wrong) {
				return p
			}
			_ = os.Remove(p)
		}
	}
	t.Skipf("no ZipCrypto check-byte collision for %q within DOS time range", wrong)
	return ""
}

func zipCryptoCheckByteAccepts(t *testing.T, p, password string) bool {
	t.Helper()
	rc, err := zip.OpenReaderWithPassword(p, password)
	if err != nil {
		t.Fatalf("open %s: %v", p, err)
	}
	defer rc.Close()
	f, err := rc.File[0].Open()
	if err != nil {
		return false // The one-byte check rejected the password.
	}
	defer f.Close()
	_, err = io.Copy(io.Discard, f)
	return errors.Is(err, zip.ErrChecksum)
}

// TestZipCrypto_WrongPasswordPassingCheckByteReprompts is the deterministic
// form of the 1-in-256 failure TestIssue816_WrongPasswordRepromptsUntilCorrect
// hit in CI: a wrong password that passes the ZipCrypto check byte must still
// bring the password dialog back rather than surface "zip: checksum error".
func TestZipCrypto_WrongPasswordPassingCheckByteReprompts(t *testing.T) {
	p := zipCryptoCollisionFixture(t, "Correct", "Wrong")
	for _, withProgress := range []bool{false, true} {
		t.Run(fmt.Sprintf("progress=%v", withProgress), func(t *testing.T) {
			res, prompts := runIssue816Scenario(t, p, []any{"Wrong", "Correct"}, withProgress)
			if res != `data="secret data"` {
				t.Fatalf("wrong then correct: got %s", res)
			}
			if prompts != 2 {
				t.Fatalf("prompts = %d, want 2", prompts)
			}
			res, _ = runIssue816Scenario(t, p, []any{"Wrong", context.Canceled}, withProgress)
			if !strings.Contains(res, context.Canceled.Error()) {
				t.Fatalf("closing the dialog after a wrong password: got %s", res)
			}
		})
	}
}

func TestMemberReadError_ReclassifiesOnlyWithPassword(t *testing.T) {
	v := &ArchiveVFS{}
	if got := v.memberReadError(zip.ErrChecksum); !errors.Is(got, zip.ErrChecksum) || isArchivePasswordRetryError(got) {
		t.Errorf("without a password a checksum error must stay a checksum error: %v", got)
	}
	v.password = "x"
	got := v.memberReadError(zip.ErrChecksum)
	if !isArchivePasswordRetryError(got) || !errors.Is(got, zip.ErrChecksum) {
		t.Errorf("with a password a checksum error must become a retry error that still wraps the cause: %v", got)
	}
	if got := v.memberReadError(io.ErrUnexpectedEOF); got != io.ErrUnexpectedEOF {
		t.Errorf("unrelated errors must pass through: %v", got)
	}
	if got := v.memberReadError(nil); got != nil {
		t.Errorf("nil must stay nil: %v", got)
	}
	var nilVFS *ArchiveVFS
	if got := nilVFS.memberReadError(zip.ErrChecksum); got != zip.ErrChecksum {
		t.Errorf("nil receiver must pass through: %v", got)
	}
}
