package netfox

import (
	"errors"
	"testing"
)

func TestQuoteSFTPCommandArgumentEmptyCoverageBatch30(t *testing.T) {
	if got := quoteSFTPCommandArgument(""); got != "''" {
		t.Fatalf("empty argument = %q, want ''", got)
	}
}

func TestQuoteSFTPCommandArgumentPlainCoverageBatch30(t *testing.T) {
	if got := quoteSFTPCommandArgument("/tmp/work"); got != "'/tmp/work'" {
		t.Fatalf("plain argument = %q", got)
	}
}

func TestQuoteSFTPCommandArgumentEscapesQuotesCoverageBatch30(t *testing.T) {
	if got := quoteSFTPCommandArgument("it's ready"); got != "'it'\"'\"'s ready'" {
		t.Fatalf("quoted argument = %q", got)
	}
}

func TestSFTPCommandExitStatusNilCoverageBatch30(t *testing.T) {
	if code, err := sftpCommandExitStatus(nil); code != 0 || err != nil {
		t.Fatalf("nil status = (%d, %v), want (0, nil)", code, err)
	}
}

type batch30ExitStatusError struct{ code int }

func (e batch30ExitStatusError) Error() string   { return "remote command failed" }
func (e batch30ExitStatusError) ExitStatus() int { return e.code }

func TestSFTPCommandExitStatusExtractsRemoteCodeCoverageBatch30(t *testing.T) {
	if code, err := sftpCommandExitStatus(batch30ExitStatusError{code: 23}); code != 23 || err != nil {
		t.Fatalf("remote status = (%d, %v), want (23, nil)", code, err)
	}
}

func TestSFTPCommandExitStatusPreservesUnknownErrorCoverageBatch30(t *testing.T) {
	want := errors.New("transport failed")
	if code, err := sftpCommandExitStatus(want); code != 0 || !errors.Is(err, want) {
		t.Fatalf("unknown status = (%d, %v), want original error", code, err)
	}
}

func TestSFTPCommandOutputChunkEndShortInputCoverageBatch30(t *testing.T) {
	data := []byte("abc")
	if got := sftpCommandOutputChunkEnd(data, 8); got != len(data) {
		t.Fatalf("short chunk end = %d, want %d", got, len(data))
	}
}

func TestSFTPCommandOutputChunkEndPreservesUTF8CoverageBatch30(t *testing.T) {
	data := []byte("aéz")
	if got := sftpCommandOutputChunkEnd(data, 2); got != 1 {
		t.Fatalf("UTF-8 chunk end = %d, want 1", got)
	}
}

func TestSFTPCommandLineWriterNilCallbackCoverageBatch30(t *testing.T) {
	w := newSFTPCommandLineWriter(nil)
	if n, err := w.Write([]byte("ignored")); n != 7 || err != nil {
		t.Fatalf("nil callback write = (%d, %v), want (7, nil)", n, err)
	}
	w.Flush()
}

func TestSFTPCommandLineWriterFlushesCRCoverageBatch30(t *testing.T) {
	var got []string
	w := newSFTPCommandLineWriter(func(line string) { got = append(got, line) })
	if _, err := w.Write([]byte("ready\r")); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if len(got) != 1 || got[0] != "ready" {
		t.Fatalf("flushed lines = %#v, want [ready]", got)
	}
}
