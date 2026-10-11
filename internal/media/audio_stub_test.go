//go:build noffi || lite || android || !(windows || ((linux || darwin || freebsd) && (amd64 || arm64)))

package media

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAudioStubLifecycle(t *testing.T) {
	a := NewAudioEngine()
	if err := a.Load("unused.wav"); !errors.Is(err, errAudioUnavailable) {
		t.Fatalf("Load error = %v, want audio unavailable", err)
	}
	a.Play()
	a.Pause()
	if a.TogglePause() || a.Seek(time.Second) || a.IsPlaying() || a.IsLoaded() || a.Finished() {
		t.Fatal("unavailable audio engine acquired playback state")
	}
	if a.Position() != 0 || a.Duration() != 0 {
		t.Fatal("unavailable audio engine advanced the clock")
	}
	if !reflect.DeepEqual(a.Info(), audioTrackInfo{}) || !reflect.DeepEqual(a.Spectrum(4), make([]float64, 4)) {
		t.Fatal("unavailable audio engine returned track data")
	}
	if a.Volume() != 0.8 {
		t.Fatalf("initial volume = %v, want 0.8", a.Volume())
	}
	for _, value := range []float64{-1, 2} {
		a.SetVolume(value)
		if a.Volume() != max(0, min(1, value)) {
			t.Fatalf("volume was not clamped: %v", a.Volume())
		}
	}
	a.Stop()
	a.Close()
	a.Close()
}
