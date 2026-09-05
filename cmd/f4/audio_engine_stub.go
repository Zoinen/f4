//go:build noffi || android || !(windows || ((linux || darwin || freebsd) && (amd64 || arm64)))

package main

import "time"

// audioEngine on platforms where oto (and therefore purego) is not
// available: every Load fails with errAudioUnavailable and the panel shows
// that in its title row. The playlist still works, so the tree is usable
// as a plain playlist editor there.
type audioEngine struct {
	volume float64
}

func newAudioEngine() *audioEngine             { return &audioEngine{volume: 0.8} }
func (a *audioEngine) Load(string) error       { return errAudioUnavailable }
func (a *audioEngine) Close()                  {}
func (a *audioEngine) Play()                   {}
func (a *audioEngine) Pause()                  {}
func (a *audioEngine) TogglePause() bool       { return false }
func (a *audioEngine) Stop()                   {}
func (a *audioEngine) IsPlaying() bool         { return false }
func (a *audioEngine) IsLoaded() bool          { return false }
func (a *audioEngine) Finished() bool          { return false }
func (a *audioEngine) Volume() float64         { return a.volume }
func (a *audioEngine) SetVolume(v float64)     { a.volume = max(0, min(1, v)) }
func (a *audioEngine) Position() time.Duration { return 0 }
func (a *audioEngine) Duration() time.Duration { return 0 }
func (a *audioEngine) Info() audioTrackInfo    { return audioTrackInfo{} }
func (a *audioEngine) Spectrum(bands int) []float64 {
	return make([]float64, bands)
}
