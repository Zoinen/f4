package media

import "testing"

func TestAudioFormatsByExtension(t *testing.T) {
	for _, name := range []string{"a.mp3", "b.WAV", "c.flac", "d.ogg", "REC001.AMR", "e.m4a", "f.opus"} {
		if !IsAudioFile(name) {
			t.Errorf("%s should be audio", name)
		}
	}
	for _, name := range []string{"a.txt", "b.jpg", "c.mp4", "noext"} {
		if IsAudioFile(name) {
			t.Errorf("%s should not be audio", name)
		}
	}
	if f, _ := audioFormatFor("x.amr"); !f.External {
		t.Errorf("AMR must go through ffmpeg")
	}
	if f, _ := audioFormatFor("x.flac"); f.External {
		t.Errorf("FLAC is decoded natively")
	}
}
