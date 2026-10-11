//go:build !noffi && !lite && !android && (linux || darwin || freebsd) && (amd64 || arm64)

package media

// openExternalAudio and ffprobeAudio spawn real processes, so these tests use
// the same seam the rest of the package relies on for that (tools.go's
// toolPaths cache) plus /bin/true and /bin/false — programs every POSIX
// system has, that ignore their arguments and only differ in exit status.
// That is enough: openExternalAudio never inspects what the process prints
// before returning, only whether it started, so a real decoder is not
// needed to exercise its bookkeeping.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// fakeToolPath makes tool.Find() report path (or "not found" for an empty
// path) without touching the real PATH, and restores whatever was cached
// before the test ran.
func fakeToolPath(t *testing.T, tool ExternalTool, path string) {
	t.Helper()
	key := tool.Names[0]
	for _, n := range tool.Names[1:] {
		key += "," + n
	}

	toolPathMu.Lock()
	old, hadOld := toolPaths[key]
	toolPaths[key] = path
	toolPathMu.Unlock()

	t.Cleanup(func() {
		toolPathMu.Lock()
		if hadOld {
			toolPaths[key] = old
		} else {
			delete(toolPaths, key)
		}
		toolPathMu.Unlock()
	})
}

// posixTrue resolves the real absolute path to /bin/true (or wherever the
// system keeps it), skipping the test if this POSIX system somehow lacks it.
func posixTrue(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no `true` binary on PATH")
	}
	return p
}

func TestOpenExternalAudioWithoutFFmpegIsReported(t *testing.T) {
	fakeToolPath(t, ToolFFmpeg, "")

	dir := t.TempDir()
	Path := filepath.Join(dir, "voice.aac")
	if err := os.WriteFile(Path, []byte("not really aac"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := openAudioSource(Path, 0)
	if err != ErrNeedFFmpeg {
		t.Errorf("err = %v, want ErrNeedFFmpeg", err)
	}
}

func TestOpenExternalAudioMissingFileIsReportedBeforeSpawning(t *testing.T) {
	fakeToolPath(t, ToolFFmpeg, posixTrue(t))

	// Never written: openExternalAudio must notice with os.Stat before it
	// ever starts ffmpeg, so ffmpeg's own silent success cannot mask it.
	Path := filepath.Join(t.TempDir(), "gone.aac")

	if _, err := openAudioSource(Path, 0); err == nil {
		t.Error("a missing file must be reported, not handed to ffmpeg")
	}
}

func TestOpenExternalAudioSpawnsAndProbesMetadata(t *testing.T) {
	fakeToolPath(t, ToolFFmpeg, posixTrue(t))
	fakeToolPath(t, toolFFprobe, posixTrue(t))

	dir := t.TempDir()
	Path := filepath.Join(dir, "voice.aac")
	if err := os.WriteFile(Path, []byte("not really aac"), 0o600); err != nil {
		t.Fatal(err)
	}

	src, err := openAudioSource(Path, 0)
	if err != nil {
		t.Fatalf("openAudioSource: %v", err)
	}
	if src.Codec != "AAC" {
		t.Errorf("codec = %q, want AAC (ffprobe found nothing to override it with)", src.Codec)
	}
	if src.Mono {
		t.Error("an empty ffprobe answer must not claim mono")
	}
	if src.Rate != 48000 {
		t.Errorf("rate = %d, want the 48000 default when none was requested", src.Rate)
	}
	if src.closer == nil {
		t.Fatal("an opened external source must know how to close its process")
	}
	// Close must reap the (already-exited) ffmpeg stand-in without hanging
	// or panicking.
	done := make(chan struct{})
	go func() { src.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return; the ffmpeg process was not reaped")
	}
}

func TestOpenExternalAudioHonoursThePreferredRate(t *testing.T) {
	fakeToolPath(t, ToolFFmpeg, posixTrue(t))
	fakeToolPath(t, toolFFprobe, "")

	dir := t.TempDir()
	Path := filepath.Join(dir, "voice.opus")
	if err := os.WriteFile(Path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	src, err := openAudioSource(Path, 44100)
	if err != nil {
		t.Fatalf("openAudioSource: %v", err)
	}
	defer src.Close()
	if src.Rate != 44100 {
		t.Errorf("rate = %d, want the requested 44100", src.Rate)
	}
}

func TestOpenExternalAudioPrefersAMRMetadataOverFFprobe(t *testing.T) {
	fakeToolPath(t, ToolFFmpeg, posixTrue(t))
	// If ffprobe were consulted it would win the race trivially since it is
	// never even asked: amrFileInfo answers first and openExternalAudio
	// must stop there.
	fakeToolPath(t, toolFFprobe, posixTrue(t))

	dir := t.TempDir()
	Path := filepath.Join(dir, "memo.amr")
	var b bytes.Buffer
	b.WriteString(amrNBMagic)
	for i := 0; i < 5; i++ { // five MR122 (type 7, 32 byte) frames = 100 ms
		b.WriteByte(7 << 3)
		b.Write(make([]byte, 31))
	}
	if err := os.WriteFile(Path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	src, err := openAudioSource(Path, 0)
	if err != nil {
		t.Fatalf("openAudioSource: %v", err)
	}
	defer src.Close()
	if src.Codec != "AMR-NB" {
		t.Errorf("codec = %q, want AMR-NB from the file itself", src.Codec)
	}
	if !src.Mono {
		t.Error("a dictaphone recording must be reported mono")
	}
	if src.Duration != 100*time.Millisecond {
		t.Errorf("duration = %s, want 100ms (5 frames at 20ms)", src.Duration)
	}
	wantLength := int64(src.Duration.Seconds() * float64(48000*AudioBytesPerFrame))
	if src.Length != wantLength {
		t.Errorf("length = %d, want %d derived from the AMR duration", src.Length, wantLength)
	}
}

func TestFFprobeAudioWithoutFFprobeIsUnknown(t *testing.T) {
	fakeToolPath(t, toolFFprobe, "")
	if _, ok := ffprobeAudio(filepath.Join(t.TempDir(), "whatever.aac")); ok {
		t.Error("with no ffprobe on PATH there is nothing to report")
	}
}

func TestFFprobeAudioWhenTheProcessFails(t *testing.T) {
	falseBin, err := exec.LookPath("false")
	if err != nil {
		t.Skip("no `false` binary on PATH")
	}
	fakeToolPath(t, toolFFprobe, falseBin)
	if _, ok := ffprobeAudio(filepath.Join(t.TempDir(), "whatever.aac")); ok {
		t.Error("a non-zero exit must be treated as no metadata, not a crash")
	}
}
