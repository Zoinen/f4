//go:build !noffi && !android && (windows || ((linux || darwin || freebsd) && (amd64 || arm64)))

package media

import (
	"strings"
	"testing"
)

// Regression test for issue #380 part 4: reopening the audio panel (e.g.
// after switching which side it is on) built a brand new AudioEngine, which
// tried to call oto.NewContext a second time in the process. oto allows that
// call to succeed only once per process, so the second AudioEngine always
// failed with "oto: context is already created" even though the first
// context was still perfectly usable. ensureContext now shares one
// process-wide context between every AudioEngine instead of creating its own.
func TestAudioEngineSecondEngineReusesContext(t *testing.T) {
	first := NewAudioEngine()
	firstErr := first.ensureContext(44100)

	second := NewAudioEngine()
	secondErr := second.ensureContext(44100)

	if (firstErr == nil) != (secondErr == nil) {
		t.Fatalf("first ensureContext err=%v, second ensureContext err=%v: a second AudioEngine must succeed or fail exactly like the first", firstErr, secondErr)
	}

	if firstErr == nil {
		if first.ctx != second.ctx {
			t.Fatal("second AudioEngine created its own oto context instead of reusing the first engine's context")
		}
		return
	}

	if strings.Contains(secondErr.Error(), "already created") && !strings.Contains(firstErr.Error(), "already created") {
		t.Fatalf("second AudioEngine hit oto's one-shot NewContext guard even though the first call was the one that ran it: first=%v second=%v", firstErr, secondErr)
	}
}
