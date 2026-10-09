//go:build !noffi && !lite && !android && (windows || ((linux || darwin || freebsd) && (amd64 || arm64)))

package media

// A file whose extension promises a codec but whose content is not that
// codec at all: openAudioSource must route to the matching Go decoder (the
// switch in openAudioSource, not the "not an audio file" or ffmpeg paths)
// and come back with that decoder's own error. WAV and FLAC already have
// dedicated malformed-header tests elsewhere in the package; MP3 and Vorbis
// have none yet, so those are the two this covers.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAudioSourceRejectsGarbageForEveryNativeCodec(t *testing.T) {
	cases := []struct {
		name string
		ext  string
	}{
		{"mp3", ".mp3"},
		{"vorbis", ".ogg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			Path := filepath.Join(dir, "junk"+c.ext)
			if err := os.WriteFile(Path, []byte("this is not an audio stream, just filler text padded out"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := openAudioSource(Path, 0); err == nil {
				t.Errorf("%s: garbage bytes must not decode as %s", c.name, c.ext)
			}
		})
	}
}

func TestOpenAudioSourceRejectsAnUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	Path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(Path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openAudioSource(Path, 0); err == nil {
		t.Error("a file with no known audio extension must be reported as such")
	}
}

func TestOpenAudioSourceReportsAMissingFile(t *testing.T) {
	Path := filepath.Join(t.TempDir(), "gone.mp3")
	if _, err := openAudioSource(Path, 0); err == nil {
		t.Error("a native-decoder path with no file to open must fail")
	}
}
