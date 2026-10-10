//go:build !noffi && !lite && !android && (windows || ((linux || darwin || freebsd) && (amd64 || arm64)))

package media

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAMRFileInfoReadsNativeMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.amr")
	var data bytes.Buffer
	data.WriteString(amrNBMagic)
	data.WriteByte(7 << 3)
	data.Write(make([]byte, amrNBFrameSizes[7]-1))
	data.WriteByte(0)
	data.Write(make([]byte, amrNBFrameSizes[0]-1))
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	info, ok := amrFileInfo(path)
	if !ok || info.Codec != "AMR-NB" || info.Rate != 8000 || info.Frames != 2 || info.Duration != 40*time.Millisecond {
		t.Fatalf("AMR metadata = %+v, ok=%t", info, ok)
	}

	bad := filepath.Join(t.TempDir(), "not-amr.bin")
	if err := os.WriteFile(bad, []byte("not an audio recording"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := amrFileInfo(bad); ok {
		t.Fatal("non-AMR file was recognized")
	}
}

func TestParseFFprobeOutputKeepsValidFieldsAndIgnoresNoise(t *testing.T) {
	got := parseFFprobeOutput(" codec_name=opus\nchannels=2\nduration=1.25\nunknown=value\nmalformed\nchannels=bad\nduration=-2\n")
	if got.Codec != "OPUS" || got.Channels != 0 || got.Duration != 1250*time.Millisecond {
		t.Fatalf("parsed ffprobe output = %+v", got)
	}

	if _, ok := parseAMR(bufio.NewReader(bytes.NewBufferString("#!AMR-WB\n"))); !ok {
		t.Fatal("empty AMR-WB stream should still be recognized")
	}
}
