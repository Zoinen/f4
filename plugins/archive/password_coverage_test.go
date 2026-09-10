package archive

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/klauspost/compress/flate"
	"github.com/unxed/archives"
	"github.com/unxed/sevenzip"
	"github.com/unxed/vtui"
	"github.com/unxed/zip"
)

func TestIsArchivePasswordRetryErrorRecognizesLazySevenZipErrors(t *testing.T) {
	plain := errors.New("payload failed")
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "value", err: sevenzip.ReadError{Err: plain}},
		{name: "pointer", err: &sevenzip.ReadError{Err: plain}},
		{name: "wrapped value", err: errors.Join(errors.New("context"), sevenzip.ReadError{Err: plain})},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !isArchivePasswordRetryError(test.err) {
				t.Fatalf("isArchivePasswordRetryError(%T) = false", test.err)
			}
		})
	}
	if isArchivePasswordRetryError(nil) {
		t.Fatal("nil must not be a password retry error")
	}
}

func TestPromptArchivePasswordUntilProvidedRetriesEmptyAnswers(t *testing.T) {
	previous := archivePasswordPrompt
	t.Cleanup(func() { archivePasswordPrompt = previous })

	answers := []string{"", "", "secret"}
	calls := 0
	archivePasswordPrompt = func(context.Context, string) (string, error) {
		answer := answers[calls]
		calls++
		return answer, nil
	}

	got, err := promptArchivePasswordUntilProvided(context.Background(), "archive.zip")
	if err != nil || got != "secret" {
		t.Fatalf("promptArchivePasswordUntilProvided() = %q, %v; want secret", got, err)
	}
	if calls != len(answers) {
		t.Fatalf("prompt calls = %d, want %d", calls, len(answers))
	}
}

func TestPromptArchivePasswordUntilProvidedStopsOnContextAndPromptError(t *testing.T) {
	previous := archivePasswordPrompt
	t.Cleanup(func() { archivePasswordPrompt = previous })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	archivePasswordPrompt = func(context.Context, string) (string, error) {
		calls++
		return "never", nil
	}
	if _, err := promptArchivePasswordUntilProvided(ctx, "archive.zip"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Fatalf("prompt calls for cancelled context = %d, want 0", calls)
	}

	wantErr := errors.New("dialog failed")
	archivePasswordPrompt = func(context.Context, string) (string, error) { return "", wantErr }
	if _, err := promptArchivePasswordUntilProvided(context.Background(), "archive.zip"); !errors.Is(err, wantErr) {
		t.Fatalf("prompt error = %v, want %v", err, wantErr)
	}
}

func TestPromptArchivePasswordWithoutUI(t *testing.T) {
	previous := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = previous })

	if _, err := promptArchivePassword(context.Background(), "archive.zip"); err == nil {
		t.Fatal("promptArchivePassword without a frame manager returned nil error")
	}
}

func TestZipCryptoPayloadError(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", want: false},
		{name: "checksum", err: zip.ErrChecksum, want: true},
		{name: "corrupt deflate", err: flate.CorruptInputError(7), want: true},
		{name: "unrelated", err: errors.New("not encrypted"), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := zipCryptoPayloadError(test.err); got != test.want {
				t.Fatalf("zipCryptoPayloadError(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}

func TestArchivePasswordFormat(t *testing.T) {
	for _, test := range []struct {
		name      string
		format    archives.Format
		want      archives.Format
		installed bool
	}{
		{
			name:   "empty password leaves format unchanged",
			format: archives.Rar{Password: "old"},
			want:   archives.Rar{Password: "old"},
		},
		{
			name:      "rar value",
			format:    archives.Rar{},
			want:      archives.Rar{Password: "secret"},
			installed: true,
		},
		{
			name:      "rar pointer",
			format:    &archives.Rar{},
			want:      &archives.Rar{Password: "secret"},
			installed: true,
		},
		{
			name:      "sevenzip value",
			format:    archives.SevenZip{},
			want:      archives.SevenZip{Password: "secret"},
			installed: true,
		},
		{
			name:      "sevenzip pointer",
			format:    &archives.SevenZip{},
			want:      &archives.SevenZip{Password: "secret"},
			installed: true,
		},
		{
			name:   "unsupported format",
			format: archives.Zip{},
			want:   archives.Zip{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			password := "secret"
			if test.name == "empty password leaves format unchanged" {
				password = ""
			}
			got, installed := archivePasswordFormat(test.format, password)
			if installed != test.installed {
				t.Fatalf("installed = %v, want %v", installed, test.installed)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("format = %#v, want %#v", got, test.want)
			}
		})
	}
}
