package unpack

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestUnpackRejectsMalformedArchives makes sure Zip and TarGz fail loudly on
// input that is not a valid archive of their format, instead of silently
// extracting nothing (which would hide a corrupt download from the caller).
func TestUnpackRejectsMalformedArchives(t *testing.T) {
	var corruptTarGz bytes.Buffer
	gw := gzip.NewWriter(&corruptTarGz)
	if _, err := gw.Write([]byte("not a tar header, just padding that is not 512 bytes long")); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		extract func([]byte, string) error
		data    []byte
	}{
		{"Zip/not a zip file", Zip, []byte("this is not a zip archive")},
		{"Zip/empty input", Zip, []byte{}},
		{"TarGz/not gzip data", TarGz, []byte("this is not gzip data at all")},
		{"TarGz/gzip wraps a corrupt tar stream", TarGz, corruptTarGz.Bytes()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.extract(tt.data, t.TempDir()); err == nil {
				t.Fatalf("%s: expected an error for malformed input, got nil", tt.name)
			}
		})
	}
}

// makeBlockedDest returns a path that exists as a regular file. Using it as
// destDir makes every extraction attempt underneath it fail with a genuine
// filesystem error (ENOTDIR) that has nothing to do with permissions, so the
// sudo-elevation branches in writeFileSafe stay out of the way and the test
// exercises only the plain error-propagation path.
func makeBlockedDest(t *testing.T) string {
	t.Helper()
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	return blocked
}

// TestTarGzPropagatesExtractEntryError checks that a write failure for one
// archive member aborts TarGz with that error, rather than being swallowed
// so the caller believes the release/plugin was installed successfully.
func TestTarGzPropagatesExtractEntryError(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: "member", Size: 1, Mode: 0644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := TarGz(buf.Bytes(), makeBlockedDest(t)); err == nil {
		t.Fatal("TarGz swallowed a write failure instead of propagating it")
	}
}

// TestZipParallelPropagatesWorkerErrorAndStopsProducer drives ZipParallel
// with more entries than workers, all of which fail extraction the same way.
// It checks two things at once: the aggregate error surfaces to the caller
// (not silently dropped once one worker already reported one), and the job
// producer goroutine actually stops sending once every worker has quit
// instead of leaking a goroutine blocked forever on a full channel.
func TestZipParallelPropagatesWorkerErrorAndStopsProducer(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	// 5 entries, 2 workers, every entry fails the same way: both workers
	// consume one job each, report an error and quit, leaving the producer
	// stuck trying to hand out the remaining 3 jobs to nobody until the
	// done channel releases it.
	blockedDest := makeBlockedDest(t)
	done := make(chan error, 1)
	go func() {
		done <- ZipParallel(buf.Bytes(), blockedDest, 2)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ZipParallel with failing entries returned nil error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ZipParallel deadlocked instead of returning once every worker failed")
	}
}
