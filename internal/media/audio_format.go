package media

// What names the player recognizes as audio, and how each one is played:
// natively or through ffmpeg. This has no decoder dependencies of its own,
// so it stays available in every build (including lite, which drops the
// audio player and its decoders) — the tree and the file panel still need
// to know a recording when they see one to show the right icon and menu
// entries, even where nothing can actually play it.

import (
	"path/filepath"
	"strings"
)

// audioFormat says how a file name is played: natively, or through ffmpeg.
type audioFormat struct {
	Codec    string
	External bool
}

// audioFormats maps extension (lower case, with the dot) to how it is
// played. Anything not here is not audio as far as the player is concerned:
// the extension is what says the file is a recording, and sniffing every
// file a user presses Enter on would promise something quite different.
var audioFormats = map[string]audioFormat{
	".mp3":  {Codec: "MP3"},
	".wav":  {Codec: "WAV"},
	".wave": {Codec: "WAV"},
	".flac": {Codec: "FLAC"},
	".ogg":  {Codec: "Vorbis"},
	".oga":  {Codec: "Vorbis"},

	// Dictaphones and phones: AMR narrow band and wide band.
	".amr": {Codec: "AMR", External: true},
	".awb": {Codec: "AMR-WB", External: true},
	".3ga": {Codec: "AMR", External: true},

	".aac":  {Codec: "AAC", External: true},
	".m4a":  {Codec: "AAC", External: true},
	".m4b":  {Codec: "AAC", External: true},
	".opus": {Codec: "Opus", External: true},
	".wma":  {Codec: "WMA", External: true},
	".ape":  {Codec: "APE", External: true},
	".wv":   {Codec: "WavPack", External: true},
	".mka":  {Codec: "Matroska", External: true},
	".aif":  {Codec: "AIFF", External: true},
	".aiff": {Codec: "AIFF", External: true},
	".mpc":  {Codec: "Musepack", External: true},
	".ac3":  {Codec: "AC-3", External: true},
	".au":   {Codec: "AU", External: true},
	".mp2":  {Codec: "MP2", External: true},
	".tta":  {Codec: "TTA", External: true},
	".spx":  {Codec: "Speex", External: true},
	".dss":  {Codec: "DSS", External: true},
	".gsm":  {Codec: "GSM", External: true},
}

func audioFormatFor(Path string) (audioFormat, bool) {
	f, ok := audioFormats[strings.ToLower(filepath.Ext(Path))]
	return f, ok
}

// IsAudioFile reports whether the player knows what to do with the name.
func IsAudioFile(Path string) bool {
	_, ok := audioFormatFor(Path)
	return ok
}
