package vfs

import (
	"errors"
	"io"
	"testing"
)

type resetFailingSeeker struct {
	resetErr error
}

func (s resetFailingSeeker) Seek(_ int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return 4096, nil
	}
	return 0, s.resetErr
}

func TestProbeSeekSizePropagatesResetFailure(t *testing.T) {
	wantErr := errors.New("reset failed")
	size, found, err := probeSeekSize(resetFailingSeeker{resetErr: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("probeSeekSize error = %v, want %v", err, wantErr)
	}
	if size != 0 || found {
		t.Fatalf("probeSeekSize = (%d, %v), want (0, false) on reset failure", size, found)
	}
}
