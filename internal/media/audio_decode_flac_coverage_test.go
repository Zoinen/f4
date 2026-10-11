//go:build !noffi && !lite && !android && (windows || ((linux || darwin || freebsd) && (amd64 || arm64)))

package media

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// flacStreamInfoBytes builds a minimal, valid FLAC stream: the "fLaC" magic
// followed by a single, last STREAMINFO metadata block and nothing else.
// mewkiz/flac's Stream.New (flac.go) reads only the StreamInfo block up
// front and stops as soon as a block's IsLast bit is set, so this is enough
// for decodeFLAC to open the stream without any actual audio frame present.
//
// Most StreamInfo fields are not byte aligned (the sample rate alone is 20
// bits), so this packs them by hand, most significant bit first, the same
// order mewkiz/flac/internal/bits.Reader unpacks them in.
func flacStreamInfoBytes(sampleRate uint32, channels, bitsPerSample uint8, nSamples uint64) []byte {
	var packed []byte
	var acc uint64
	var nbits uint
	push := func(v uint64, width uint) {
		acc = (acc << width) | (v & ((1 << width) - 1))
		nbits += width
		for nbits >= 8 {
			nbits -= 8
			packed = append(packed, byte(acc>>nbits))
		}
	}
	push(4096, 16) // BlockSizeMin, must be >= 16
	push(4096, 16) // BlockSizeMax, must be >= 16
	push(0, 24)    // FrameSizeMin, 0 = unknown
	push(0, 24)    // FrameSizeMax, 0 = unknown
	push(uint64(sampleRate), 20)
	push(uint64(channels-1), 3)      // stored as (channels - 1)
	push(uint64(bitsPerSample-1), 5) // stored as (bits-per-sample - 1)
	push(nSamples, 36)
	if nbits != 0 {
		panic("flacStreamInfoBytes: fields do not end on a byte boundary")
	}

	body := append(packed, make([]byte, 16)...) // MD5sum, not checked by decodeFLAC
	if len(body) != 34 {
		panic("flacStreamInfoBytes: a STREAMINFO body must be 34 bytes")
	}
	header := []byte{0x80, 0x00, 0x00, byte(len(body))} // last block, type 0 (StreamInfo)
	out := append([]byte("fLaC"), header...)
	return append(out, body...)
}

// A FLAC file that ends the instant its last (and only) metadata block does
// — no audio frames at all — is not malformed. decodeFLAC only reads the
// StreamInfo block up front, so opening it must succeed, and the first read
// attempted afterwards must see the graceful end of stream
// flac.Stream.ParseNext documents (io.EOF) rather than an error: frame
// parsing treats a sync-code read that hits EOF with nothing buffered as
// "no more frames", not "corrupt frame".
func TestDecodeFLACHeaderOnlyStreamOpensAndReadsAsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.flac")
	if err := os.WriteFile(path, flacStreamInfoBytes(44100, 2, 16, 0), 0o600); err != nil {
		t.Fatal(err)
	}

	src, err := openAudioSource(path, 0)
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	defer src.Close()

	if src.Rate != 44100 {
		t.Errorf("rate = %d, want 44100", src.Rate)
	}
	if src.Mono {
		t.Error("a two channel stream must not be reported as mono")
	}
	if src.Codec != "FLAC" {
		t.Errorf("codec = %q, want FLAC", src.Codec)
	}
	if src.Length != 0 {
		t.Errorf("length = %d, want 0 for an unknown sample count", src.Length)
	}
	if src.Duration != 0 {
		t.Errorf("duration = %s, want 0 when the length is unknown", src.Duration)
	}

	out, err := io.ReadAll(src)
	if err != nil {
		t.Fatalf("reading the frame-less stream: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("read %d bytes from a stream with no frames", len(out))
	}
}

// A mono file with a known sample count both sets Mono and lets
// openAudioSource compute Duration from the Length and Rate decodeFLAC
// reports — decodeFLAC itself only fills in Length.
func TestDecodeFLACMonoKnownLengthComputesDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mono.flac")
	// One second of 8 kHz mono: 8000 samples, no frames.
	if err := os.WriteFile(path, flacStreamInfoBytes(8000, 1, 16, 8000), 0o600); err != nil {
		t.Fatal(err)
	}

	src, err := openAudioSource(path, 0)
	if err != nil {
		t.Fatalf("decoding failed: %v", err)
	}
	defer src.Close()

	if !src.Mono {
		t.Error("a one channel stream must be reported as mono")
	}
	if want := int64(8000) * AudioBytesPerFrame; src.Length != want {
		t.Errorf("length = %d, want %d", src.Length, want)
	}
	if src.Duration != time.Second {
		t.Errorf("duration = %s, want 1s", src.Duration)
	}

	if _, err := io.ReadAll(src); err != nil {
		t.Fatalf("reading the frame-less stream: %v", err)
	}
}

// A file that never gets past the FLAC signature check is reported as such,
// not as a stream with zero frames.
func TestDecodeFLACRejectsBadSignature(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.flac")
	if err := os.WriteFile(path, []byte("not a flac file at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openAudioSource(path, 0); err == nil {
		t.Error("a file without the fLaC signature must be reported as such")
	}
}
